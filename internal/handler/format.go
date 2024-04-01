package handler

import (
	"context"
	"fmt"
	"log/slog"
	"unicode/utf8"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentFormatting(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DocumentFormattingParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	doc, ok := h.documents.Get(params.TextDocument.URI)
	if !ok {
		return replyNoDocFound(ctx, reply, params.TextDocument.URI)
	}

	slog.Info("formatting", "pre", doc.Content)

	formatted, err := h.getBinManager().Format(doc.Content)
	if err != nil {
		return replyErr(ctx, reply, fmt.Errorf("formatting: %w", err))
	}

	slog.Info("formatting", "post", formatted)

	lastLine := len(doc.Lines) - 1
	lastChar := utf8.RuneCountInString(doc.Lines[lastLine])

	slog.Info("formatting", "lastLine", lastLine, "lastChar", lastChar)
	return reply(ctx, []protocol.TextEdit{
		{
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End: protocol.Position{
					Line:      uint32(lastLine),
					Character: uint32(lastChar),
				},
			},
			NewText: string(formatted),
		},
	}, nil)
}
