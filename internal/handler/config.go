package handler

import (
	"context"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"github.com/jdkato/gnols/internal/gno"
)

func (h *handler) handleDidChangeConfiguration(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidChangeConfigurationParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}

	settings, ok := params.Settings.(map[string]interface{})
	if !ok {
		return replyErr(ctx, reply, ErrBadSettings)
	}
	slog.Info("configuration changed", "settings", settings)

	gnoBin, _ := settings["gno"].(string)
	gnokeyBin, _ := settings["gnokey"].(string)
	goplsBin, _ := settings["gopls"].(string)

	transpile, _ := settings["transpileOnSave"].(bool)
	build, _ := settings["buildOnSave"].(bool)
	root, _ := settings["root"].(string)

	var err error
	h.binManager, err = gno.NewBinManager(h.workspaceFolder, gnoBin, gnokeyBin, goplsBin, root, transpile, build)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	slog.Info("binManager created", "workspaceFolder", h.workspaceFolder)
	close(h.configLoaded)
	return reply(ctx, nil, nil)
}
