package handler

import (
	"context"
	"encoding/json"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"github.com/jdkato/gnols/internal/gno"
)

func (h *handler) handleDidChangeConfiguration(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.DidChangeConfigurationParams

	err := json.Unmarshal(req.Params(), &params)
	if err != nil {
		return replyBadJSON(ctx, reply, err)
	}

	settings, ok := params.Settings.(map[string]interface{})
	if !ok {
		return replyErr(ctx, reply, ErrBadSettings)
	}
	slog.Info("configuration changed", "settings", settings)

	gnoBin, _ := settings["gno"].(string)
	gnokeyBin, _ := settings["gnokey"].(string)
	goplsBin, _ := settings["gopls"].(string)

	precompile, _ := settings["precompileOnSave"].(bool)
	build, _ := settings["buildOnSave"].(bool)
	root, _ := settings["root"].(string)

	h.binManager, err = gno.NewBinManager(gnoBin, gnokeyBin, goplsBin, root, precompile, build)
	if err != nil {
		return replyErr(ctx, reply, err)
	}
	close(h.configLoaded)
	return reply(ctx, nil, nil)
}
