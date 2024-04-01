package handler

import (
	"context"
	"log/slog"
	"strings"

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
	items := []protocol.CompletionItem{}

	token, err := doc.TokenAt(params.Position)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	text := strings.TrimSuffix(strings.TrimSpace(token.Text), ".")
	slog.Info("completion", "text", text)

	pkg := lookupPkg(text)
	if pkg != nil {
		for _, s := range pkg.Symbols {
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
