package handler

import (
	"context"
	"encoding/json"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentReferrences(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.ReferenceParams

	if req.Params() == nil {
		return &jsonrpc2.Error{Code: jsonrpc2.InvalidParams}
	} else if err := json.Unmarshal(req.Params(), &params); err != nil {
		return replyBadJSON(ctx, reply, err)
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
