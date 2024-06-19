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
	ss := strings.Split(text, ".")
	symbolName := ss[0]
	var selectors []string
	if len(ss) > 1 {
		selectors = ss[1:]
	}

	slog.Info("completion", "text", text, "symbol", symbolName, "selectors", selectors)

	items := []protocol.CompletionItem{}
	// TODO call lookupSymbols directly ?
	for _, sym := range h.symbols {
		if sym.Name != symbolName {
			continue
		}
		switch sym.Kind {
		case "var":
			syms := h.lookupSymbols(sym.Type, selectors)
			for _, f := range syms {
				items = append(items, protocol.CompletionItem{
					Label:         f.Name,
					InsertText:    f.Name,
					Kind:          symbolToKind(f.Kind),
					Detail:        f.Signature,
					Documentation: f.Doc,
				})
			}
		}
	}

	if pkg := lookupPkg(stdlib.Packages, symbolName); pkg != nil {
		for _, s := range pkg.Symbols {
			if s.Recv != "" {
				// skip symbols with receiver (methods)
				continue
			}
			if len(selectors) > 0 && !strings.HasPrefix(s.Name, selectors[0]) {
				// TODO handle multiple selectors (possible if for example a global var
				// is defined in the pkg, and the user is referrencing it.)
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

func (h handler) lookupSymbols(name string, selectors []string) []gno.Symbol {
	slog.Info("lookupSymbols", "name", name, "selectors", selectors)
	for _, sym := range h.symbols {
		if sym.Name != name {
			continue
		}
		// we found a symbol matching name
		if len(selectors) == 0 {
			// no other selectors, return symbol fields
			return sym.Fields
		}
		// Check if a field is matched
		var symbols []gno.Symbol
		for _, f := range sym.Fields {
			if f.Name == selectors[0] {
				// field matches selector exactly, lookup in field type fields.
				return h.lookupSymbols(f.Type, selectors[1:])
			}
			if strings.HasPrefix(f.Name, selectors[0]) {
				// field match partially selector, append
				symbols = append(symbols, f)
			}
		}
		return symbols
	}
	return nil
}
