package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/uri"
)

var ErrBadSettings = errors.New("bad settings")

func replyErr(ctx context.Context, reply jsonrpc2.Replier, err error) error {
	slog.Error(err.Error())
	return reply(ctx, nil, err)
}

func replyNoDocFound(ctx context.Context, reply jsonrpc2.Replier, uri uri.URI) error {
	return replyErr(ctx, reply, fmt.Errorf("couldn't find document %s", uri.Filename()))
}
