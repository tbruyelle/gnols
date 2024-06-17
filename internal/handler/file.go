package handler

import (
	"context"
	"fmt"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentDidOpen(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidOpenTextDocumentParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	doc, err := h.documents.Save(params.TextDocument.URI, params.TextDocument.Text)
	if err != nil {
		return replyErr(ctx, reply, fmt.Errorf("documents.Save uri=%s: %w", params.TextDocument.URI, err))
	}
	if err := h.updateSymbols(); err != nil {
		return replyErr(ctx, reply, err)
	}
	h.publishDianostics(ctx, doc)
	return reply(ctx, nil, nil)
}

func (h *handler) handleTextDocumentDidClose(ctx context.Context, reply jsonrpc2.Replier, _ jsonrpc2.Request) error {
	return reply(
		ctx,
		h.connPool.Notify(
			ctx,
			protocol.MethodTextDocumentDidClose,
			nil,
		),
		nil,
	)
}

func (h *handler) handleTextDocumentDidSave(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidSaveTextDocumentParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	doc, ok := h.documents.Get(params.TextDocument.URI)
	if !ok {
		// File is saved for the first time
		newDoc, err := h.documents.Save(params.TextDocument.URI, params.Text)
		if err != nil {
			return replyErr(ctx, reply, fmt.Errorf("documents.Save: %w", err))
		}
		slog.Info("new doc saved", "path", newDoc.Path)
		doc = newDoc
	}
	h.publishDianostics(ctx, doc)
	return reply(ctx, nil, nil)
}

func (h *handler) handleTextDocumentDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidChangeTextDocumentParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	doc, ok := h.documents.Get(params.TextDocument.URI)
	if !ok {
		return replyNoDocFound(ctx, reply, params.TextDocument.URI)
	}
	doc.ApplyChanges(params.ContentChanges)

	return reply(ctx, nil, nil)
}
