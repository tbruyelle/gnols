package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"github.com/jdkato/gnols/internal/stdlib"
)

func (h *handler) handleHover(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.HoverParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	doc, ok := h.documents.Get(params.TextDocument.URI)
	if !ok {
		return replyNoDocFound(ctx, reply, params.TextDocument.URI)
	}
	pgf := doc.Pgf

	offset := doc.PositionToOffset(params.Position)
	// tokedf := pgf.FileSet.AddFile(doc.Path, -1, len(doc.Content))
	// target := tokedf.Pos(offset)

	slog.Info("hover", "offset", offset)
	for _, spec := range pgf.File.Imports {
		slog.Info("hover", "spec", spec.Path.Value, "pos", spec.Path.Pos(), "end", spec.Path.End())
		if int(spec.Path.Pos()) <= offset && offset <= int(spec.Path.End()) {
			// TODO: handle hover for imports
			slog.Info("hover", "import", spec.Path.Value)
			return reply(ctx, nil, nil)
		}
	}

	token, err := doc.TokenAt(params.Position)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	text := strings.TrimSpace(token.Text)

	// FIXME: Use the AST package to do this + get type of token.
	//
	// This is just a quick PoC to get something working.

	// strings.Split(p.Body,
	text = strings.Split(text, "(")[0]

	text = strings.TrimSuffix(text, ",")
	text = strings.TrimSuffix(text, ")")

	// *mux.Request
	text = strings.TrimPrefix(text, "*")

	slog.Info("hover", "pkg", len(stdlib.Packages))

	parts := strings.Split(text, ".")
	if len(parts) == 2 {
		pkg := parts[0]
		sym := parts[1]

		slog.Info("hover", "pkg", pkg, "sym", sym)
		found := lookupSymbol(pkg, sym)
		if found == nil && pgf.File != nil {
			found = lookupSymbolByImports(sym, pgf.File.Imports)
		}

		if found != nil {
			return reply(ctx, protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.Markdown,
					Value: fmt.Sprintf("```go\n%s\n```\n\n%s", found.Signature, found.Doc),
				},
				Range: posToRange(
					int(params.Position.Line),
					[]int{token.Start, token.End},
				),
			}, nil)
		}
	}

	return reply(ctx, nil, nil)
}
