package handler

import (
	"context"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"github.com/jdkato/gnols/internal/store"
)

func (h *handler) publishDianostics(ctx context.Context, conn jsonrpc2.Conn, doc *store.Document) error {
	diagnostics, err := h.getDiagnostics(doc)
	if err != nil {
		return err
	}
	return conn.Notify(
		ctx,
		protocol.MethodTextDocumentPublishDiagnostics,
		&protocol.PublishDiagnosticsParams{
			URI:         doc.URI,
			Diagnostics: diagnostics,
		},
	)
}

func (h *handler) getDiagnostics(doc *store.Document) ([]protocol.Diagnostic, error) {
	diagnostics := []protocol.Diagnostic{}

	slog.Info("Lint", "path", doc.Path)

	buildErrs, err := h.getBinManager().Lint()
	if err != nil {
		return diagnostics, err
	}

	for _, buildErr := range buildErrs {
		if doc.URI != buildErr.Span.URI {
			// Ignore buildErr that doesn't target doc
			continue
		}
		diagnostics = append(diagnostics, protocol.Diagnostic{
			Range:    buildErr.Span.ToLocation().Range,
			Severity: protocol.DiagnosticSeverityError,
			Source:   "gnols",
			Message:  buildErr.Msg,
			Code:     buildErr.Tool,
		})
	}

	slog.Info("diagnostics", "count", len(diagnostics), "parsed", diagnostics)
	return diagnostics, nil
}
