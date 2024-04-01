package handler

import (
	"context"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentReferences(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.ReferenceParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	spans, err := h.getBinManager().References(ctx,
		params.TextDocument.URI, params.Position.Line, params.Position.Character,
	)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	locs := make([]protocol.Location, len(spans))
	for i := 0; i < len(spans); i++ {
		locs[i] = spans[i].ToLocation()
	}
	return reply(ctx, locs, nil)
}

func (h *handler) handleTextDocumentDefinition(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DefinitionParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	def, err := h.getBinManager().Definition(ctx,
		params.TextDocument.URI, params.Position.Line, params.Position.Character,
	)
	if err != nil {
		return replyErr(ctx, reply, err)
	}

	return reply(ctx, def.Span.ToLocation(), nil)
}

func (h *handler) handleTextDocumentImplementation(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.ImplementationParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	spans, err := h.getBinManager().Implementation(ctx,
		params.TextDocument.URI, params.Position.Line, params.Position.Character,
	)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	locs := make([]protocol.Location, len(spans))
	for i := 0; i < len(spans); i++ {
		locs[i] = spans[i].ToLocation()
	}
	return reply(ctx, locs, nil)
}
