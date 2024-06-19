package handler

import (
	"context"
	"log/slog"
	"slices"
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
	// TODO handle multiple dots like Struct.A.B
	var (
		symbolName = text
		prefixes   []string // TODO rename
	)
	// TODO ensure textDocument/completion is only triggered after a dot '.' and
	// if yes why ? Is it a client limitation or configuration, or a LSP spec?
	ss := strings.Split(text, ".")
	symbolName = ss[0]
	if len(ss) > 1 {
		prefixes = ss[1:]
	}

	slog.Info("completion", "text", text, "symbol", symbolName, "prefixes", prefixes)

	items := []protocol.CompletionItem{}
	matchName := func(name string) func(gno.Symbol) bool {
		return func(s gno.Symbol) bool { return s.Name == name }
	}
	if i := slices.IndexFunc(h.symbols, matchName(symbolName)); i != -1 {
		sym := h.symbols[i]
		switch sym.Kind {
		case "var":
			// find referrence type
			if j := slices.IndexFunc(h.symbols, matchName(sym.Ref)); j != -1 {
				var prefix string
				if len(prefixes) > 0 {
					prefix = prefixes[0]
					prefixes = prefixes[1:]
				}
				for _, f := range h.symbols[j].Fields {
					if prefix != "" && !strings.HasPrefix(f.Name, prefix) {
						// skip symbols that doesn't match the prefix
						continue
					}
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
	}

	if pkg := lookupPkg(stdlib.Packages, symbolName); pkg != nil {
		for _, s := range pkg.Symbols {
			if s.Recv != "" {
				// skip symbols with receiver (methods)
				continue
			}
			if prefix != "" && !strings.HasPrefix(s.Name, prefix) {
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
