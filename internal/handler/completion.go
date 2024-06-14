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
	// TODO handle multiple dots like Struct.A.B
	var (
		symbolName = text
		prefix     string
	)
	if i := strings.IndexByte(text, '.'); i > 0 {
		symbolName = text[:i]
		if i != len(text)-1 {
			prefix = text[i+1:]
		}
	}
	slog.Info("completion", "text", text, "symbol", symbolName, "prefix", prefix)

	items := []protocol.CompletionItem{}
	for _, pkgs := range [][]gno.Package{
		h.packages,      // Check in workspace's packages
		stdlib.Packages, // Check in stdlib and examples packages
	} {
		pkg := lookupPkg(pkgs, symbolName)
		if pkg == nil {
			continue
		}
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
