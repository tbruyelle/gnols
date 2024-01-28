package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentDidOpen(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidOpenTextDocumentParams

	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return replyBadJSON(ctx, reply, err)
	}

	doc, err := h.documents.Save(params.TextDocument.URI, params.TextDocument.Text)
	if err != nil {
		return replyErr(ctx, reply, fmt.Errorf("documents.Save: %w", err))
	}

	if err := h.publishDianostics(ctx, h.connPool, doc); err != nil {
		slog.Error("publishDianostics", "doc", doc.Path, "err", err)
	}
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

	if req.Params() == nil {
		return &jsonrpc2.Error{Code: jsonrpc2.InvalidParams}
	}
	if err := json.Unmarshal(req.Params(), &params); err != nil {
		return replyBadJSON(ctx, reply, err)
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

	if err := h.publishDianostics(ctx, h.connPool, doc); err != nil {
		slog.Error("publishDianostics", "doc", doc.Path, "err", err)
	}
	return reply(ctx, nil, nil)
}

func (h *handler) handleTextDocumentDidChange(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidChangeTextDocumentParams

	if req.Params() == nil {
		return &jsonrpc2.Error{Code: jsonrpc2.InvalidParams}
	} else if err := json.Unmarshal(req.Params(), &params); err != nil {
		return replyBadJSON(ctx, reply, err)
	}

	doc, ok := h.documents.Get(params.TextDocument.URI)
	if !ok {
		return replyNoDocFound(ctx, reply, params.TextDocument.URI)
	}
	doc.ApplyChanges(params.ContentChanges)

	return reply(ctx, nil, nil)
}
