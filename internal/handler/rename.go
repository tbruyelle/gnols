package handler

import (
	"context"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

func (h *handler) handleTextDocumentPrepareRename(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.PrepareRenameParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	err := h.getBinManager().PrepareRename(ctx,
		params.TextDocument.URI, params.Position.Line, params.Position.Character,
	)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	// FIXME: returns a Range parsed from prepare_rename reponse
	response := map[string]any{
		"defaultBehavior": true,
	}
	return reply(ctx, response, nil)
}

func (h *handler) handleTextDocumentRename(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.RenameParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	docEdits, err := h.getBinManager().Rename(ctx,
		params.TextDocument.URI, params.Position.Line, params.Position.Character,
		params.NewName,
	)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	response := protocol.WorkspaceEdit{}
	for _, de := range docEdits {
		tde := protocol.TextDocumentEdit{
			TextDocument: protocol.OptionalVersionedTextDocumentIdentifier{
				TextDocumentIdentifier: protocol.TextDocumentIdentifier{
					URI: de.URI,
				},
			},
		}
		for _, e := range de.Edits {
			slog.Info("edit", "line", e.Line, "char", e.Column)
			tde.Edits = append(tde.Edits, protocol.TextEdit{
				NewText: e.NewText,
				Range: protocol.Range{
					Start: protocol.Position{
						Line:      e.Line - 1,
						Character: e.Column - 1,
					},
					// To address the whole line: End should be set as follow:
					// endLine = startLine+1
					// endChar = 0
					// (see https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/#range)
					End: protocol.Position{
						Line:      e.Line,
						Character: 0,
					},
				},
			})
		}
		response.DocumentChanges = append(response.DocumentChanges, tde)
	}
	return reply(ctx, response, nil)
}
