package handler

import (
	"context"
	"encoding/json"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentDefinition(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DefinitionParams

	if req.Params() == nil {
		return &jsonrpc2.Error{Code: jsonrpc2.InvalidParams}
	} else if err := json.Unmarshal(req.Params(), &params); err != nil {
		return replyBadJSON(ctx, reply, err)
	}

	def, err := h.getBinManager().Definition(ctx,
		params.TextDocument.URI, params.Position.Line, params.Position.Character,
	)
	if err != nil {
		return replyErr(ctx, reply, err)
	}

	return reply(ctx, protocol.Location{
		URI: def.Span.URI,
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      def.Span.Start.Line,
				Character: def.Span.Start.Column,
			},
			End: protocol.Position{
				Line:      def.Span.End.Line,
				Character: def.Span.End.Column,
			},
		},
	}, nil)
}
