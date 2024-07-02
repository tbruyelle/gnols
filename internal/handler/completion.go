package handler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/jdkato/gnols/internal/gno"
	"github.com/jdkato/gnols/internal/stdlib"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentCompletion(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.CompletionParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	doc, ok := h.documents.Get(params.TextDocument.URI)
	if !ok {
		return replyNoDocFound(ctx, reply, params.TextDocument.URI)
	}

	token, err := doc.TokenAt(params.Position)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	text := strings.TrimSpace(token.Text)

	// Extract symbol name and prefix from text
	// TODO ensure textDocument/completion is only triggered after a dot '.' and
	// if yes why ? Is it a client limitation or configuration, or a LSP spec?
	selectors := strings.FieldsFunc(text, func(r rune) bool { return r == '.' })

	slog.Info("completion", "text", text, "selectors", selectors)

	items := []protocol.CompletionItem{}

	//-----------------------------------------
	// Look up local symbols
	if syms := h.lookupSymbols(selectors); len(syms) > 0 {
		for _, f := range syms {
			items = append(items, protocol.CompletionItem{
				Label:         f.Name,
				InsertText:    f.Name,
				Kind:          symbolToKind(f.Kind),
				Detail:        f.Signature,
				Documentation: f.Doc,
			})
		}
	} else {
		//-----------------------------------------
		// Look up stdlib
		if pkg := lookupPkg(stdlib.Packages, selectors[0]); pkg != nil {
			for _, s := range pkg.Symbols {
				if s.Recv != "" {
					// skip symbols with receiver (methods)
					continue
				}
				if len(selectors) > 1 && !strings.HasPrefix(s.Name, selectors[1]) {
					// TODO handle multiple selectors? (possible if for example a global
					// var is defined in the pkg, and the user is referrencing it.)

					// skip symbols that doesn't match the prefix
					continue
				}
				items = append(items, protocol.CompletionItem{
					Label:         s.Name,
					InsertText:    s.Name,
					Kind:          symbolToKind(s.Kind),
					Detail:        s.Signature,
					Documentation: s.Doc,
				})
			}
		}
	}
	return reply(ctx, items, nil)
}

func (h handler) lookupSymbols(selectors []string) []gno.Symbol {
	// Check first in current pkg
	found := symbolFinder{h.currentPkg.Symbols}.find(selectors)
	if len(found) > 0 {
		return found
	}
	// Check in sub packages
	for _, pkg := range h.subPkgs {
		if pkg.Name == selectors[0] {
			return symbolFinder{pkg.Symbols}.find(selectors[1:])
		}
	}
	// TODO check in imported packages of the file
	return []gno.Symbol{}
}

type symbolFinder struct {
	baseSymbols []gno.Symbol
}

func (s symbolFinder) find(selectors []string) []gno.Symbol {
	return s.findIn(s.baseSymbols, selectors)
}

func (s symbolFinder) findIn(symbols []gno.Symbol, selectors []string) []gno.Symbol {
	slog.Info("lookup symbol", "symbols", symbols, "selectors", selectors)
	if len(selectors) == 0 {
		return symbols
	}
	name := selectors[0]
	var syms []gno.Symbol
	for _, sym := range symbols {
		switch {
		case sym.Name == name: // exact match
			slog.Info("found symbol", "name", name, "kind", sym.Kind, "selectors", selectors)
			// we found a symbol matching name
			switch sym.Kind {
			case "var", "field":
				if sym.Type == "" {
					// sym is an inline struct, returns fields
					return sym.Fields
				}
				// lookup for symbols matching type in baseSymbols
				return s.findIn(s.baseSymbols, append([]string{sym.Type}, selectors[1:]...))

			case "struct":
				// sym is a struct, lookup in fields
				return s.findIn(sym.Fields, selectors[1:])
			}

		case strings.HasPrefix(sym.Name, name): // partial match
			syms = append(syms, sym)
		}
	}
	return syms
}
