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
	ss := strings.FieldsFunc(text, func(r rune) bool { return r == '.' })
	symbolName := ss[0]
	var selectors []string
	if len(ss) > 1 {
		selectors = ss[1:]
	}

	slog.Info("completion", "text", text, "symbol", symbolName, "selectors", selectors)

	items := []protocol.CompletionItem{}
	syms := h.lookupSymbols(symbolName, selectors)
	for _, f := range syms {
		items = append(items, protocol.CompletionItem{
			Label:         f.Name,
			InsertText:    f.Name,
			Kind:          symbolToKind(f.Kind),
			Detail:        f.Signature,
			Documentation: f.Doc,
		})
	}
	// TODO stop of len(items)>0?

	if pkg := lookupPkg(stdlib.Packages, symbolName); pkg != nil {
		for _, s := range pkg.Symbols {
			if s.Recv != "" {
				// skip symbols with receiver (methods)
				continue
			}
			if len(selectors) > 0 && !strings.HasPrefix(s.Name, selectors[0]) {
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
	return reply(ctx, items, nil)
}

// TODO merge name and selectors?
func (h handler) lookupSymbols(name string, selectors []string) []gno.Symbol {
	return lookupSymbols(h.symbols, name, selectors)
}

func lookupSymbols(symbols []gno.Symbol, name string, selectors []string) []gno.Symbol {
	for _, sym := range symbols {
		switch {

		case sym.Name == name:
			slog.Info("found symbol", "name", name, "kind", sym.Kind, "selectors", selectors)
			// we found a symbol matching name
			switch sym.Kind {
			case "var":
				if sym.Type == "" {
					// sym is an inline struct, returns fields
					return sym.Fields
				}
				// sym is a variable, lookup for symbols matching type
				return lookupSymbols(symbols, sym.Type, selectors)

			case "struct":
				// sym is a struct, lookup for matching fields
				if len(selectors) == 0 {
					// no other selectors, return all symbol fields
					// TODO handle remaining selectors if any
					return sym.Fields
				}
				var syms []gno.Symbol
				for _, f := range sym.Fields {
					if f.Name == selectors[0] {
						if f.Type == "" {
							// sym is an inline struct, returns fields
							return f.Fields
						}
						// field matches selector exactly, lookup in field type fields.
						return lookupSymbols(symbols, f.Type, selectors[1:])
					}
					if strings.HasPrefix(f.Name, selectors[0]) {
						// field match partially selector, append
						syms = append(syms, f)
					}
				}
				return syms
			}

		case sym.PkgPath == name:
			slog.Info("found package", "name", name, "selectors", selectors)
			// match a sub package, lookup symbols from that package
			var syms []gno.Symbol
			for _, sym := range symbols {
				if sym.PkgPath == name {
					syms = append(syms, sym)
				}
			}
			if len(selectors) == 0 {
				return syms
			}
			return lookupSymbols(syms, selectors[0], selectors[1:])
		}
	}
	return nil
}
