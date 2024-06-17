package handler

import (
	"context"
	"fmt"
	"log/slog"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"

	"github.com/jdkato/gnols/internal/gno"
	"github.com/jdkato/gnols/internal/store"
)

type handler struct {
	connPool  jsonrpc2.Conn
	documents *store.DocumentStore
	// symbols contains the workspace's symbols.
	symbols    []gno.Symbol
	binManager *gno.BinManager
	// initialized becomes true after `initialize` message is received.
	initialized bool
	// NOTE(tb): See why [here](https://github.com/tbruyelle/gnols/issues/11)
	configLoaded chan struct{}

	// workspaceFolder contains the path of the project.
	workspaceFolder string
}

func NewHandler(connPool jsonrpc2.Conn) jsonrpc2.Handler {
	handler := &handler{
		connPool:     connPool,
		documents:    store.NewDocumentStore(),
		binManager:   nil,
		configLoaded: make(chan struct{}),
	}
	slog.Info("connections opened")
	return jsonrpc2.ReplyHandler(handler.handle)
}

func (h *handler) getBinManager() *gno.BinManager {
	<-h.configLoaded
	return h.binManager
}

func (h *handler) handle(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	slog.Info("handle", "method", req.Method())
	if req.Method() == protocol.MethodInitialize {
		err := h.handleInitialize(ctx, reply, req)
		if err != nil {
			return err
		}
		h.initialized = true
		return nil
	}
	if !h.initialized {
		return replyErr(ctx, reply, jsonrpc2.NewError(jsonrpc2.ServerNotInitialized, "server not initialized"))
	}
	switch req.Method() {
	case protocol.MethodInitialized:
		return reply(ctx, nil, nil)
	case protocol.MethodWorkspaceDidChangeConfiguration:
		return h.handleDidChangeConfiguration(ctx, reply, req)
	case protocol.MethodShutdown:
		return h.handleShutdown(ctx, reply, req)
	case protocol.MethodTextDocumentDidOpen:
		return h.handleTextDocumentDidOpen(ctx, reply, req)
	case protocol.MethodTextDocumentDidClose:
		return h.handleTextDocumentDidClose(ctx, reply, req)
	case protocol.MethodTextDocumentDidChange:
		return h.handleTextDocumentDidChange(ctx, reply, req)
	case protocol.MethodTextDocumentDidSave:
		return h.handleTextDocumentDidSave(ctx, reply, req)
	case protocol.MethodTextDocumentDefinition:
		return h.handleTextDocumentDefinition(ctx, reply, req)
	case protocol.MethodTextDocumentReferences:
		return h.handleTextDocumentReferences(ctx, reply, req)
	case protocol.MethodTextDocumentImplementation:
		return h.handleTextDocumentImplementation(ctx, reply, req)
	case protocol.MethodTextDocumentPrepareRename:
		return h.handleTextDocumentPrepareRename(ctx, reply, req)
	case protocol.MethodTextDocumentRename:
		return h.handleTextDocumentRename(ctx, reply, req)
	case protocol.MethodTextDocumentCompletion:
		return h.handleTextDocumentCompletion(ctx, reply, req)
	case protocol.MethodTextDocumentHover:
		return h.handleHover(ctx, reply, req)
	case protocol.MethodTextDocumentCodeLens:
		return h.handleCodeLens(ctx, reply, req)
	case protocol.MethodWorkspaceExecuteCommand:
		return h.handleExecuteCommand(ctx, reply, req)
	case protocol.MethodTextDocumentFormatting:
		return h.handleTextDocumentFormatting(ctx, reply, req)
	default:
		return jsonrpc2.MethodNotFoundHandler(ctx, reply, req)
	}
}

func (h *handler) handleInitialize(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	var params protocol.InitializeParams
	if err := readParams(req, &params); err != nil {
		return replyErr(ctx, reply, err)
	}
	// NOTE(tb): params.RootURI is deprecated in favor of params.WorkspaceFolders,
	// but this one is not filled by vim-lsp. Maybe we should check it first and
	// fallback to params.RootURI.
	h.workspaceFolder = params.RootURI.Filename() //nolint:staticcheck
	slog.Info("Initialize", "params", params, "workspaceFolder", h.workspaceFolder)

	return reply(ctx, protocol.InitializeResult{
		Capabilities: protocol.ServerCapabilities{
			TextDocumentSync: protocol.TextDocumentSyncOptions{
				Change:    protocol.TextDocumentSyncKindFull,
				OpenClose: true,
				Save: &protocol.SaveOptions{
					IncludeText: true,
				},
			},
			DefinitionProvider: &protocol.DefinitionOptions{},
			ReferencesProvider: &protocol.ReferencesOptions{},
			RenameProvider: &protocol.RenameOptions{
				PrepareProvider: false,
			},
			ImplementationProvider: &protocol.ImplementationOptions{},
			CompletionProvider: &protocol.CompletionOptions{
				TriggerCharacters: []string{"."},
				ResolveProvider:   false,
			},
			HoverProvider: true,
			ExecuteCommandProvider: &protocol.ExecuteCommandOptions{
				Commands: []string{
					"gnols.gnofmt",
					"gnols.test",
				},
			},
			CodeLensProvider: &protocol.CodeLensOptions{
				ResolveProvider: true,
			},
			DocumentFormattingProvider: true,
		},
	}, nil)
}

func (h *handler) handleShutdown(ctx context.Context, reply jsonrpc2.Replier, _ jsonrpc2.Request) error {
	return reply(ctx, nil, h.connPool.Close())
}

func (h *handler) notify(ctx context.Context, method string, params any) {
	if !h.initialized && method != "exit" {
		// Skip notification if not initialized except for `exit` method.
		return
	}
	err := h.connPool.Notify(ctx, method, params)
	if err != nil {
		slog.Error("notify error", "err", err)
	}
}

func (h *handler) notifyErr(ctx context.Context, err error) {
	slog.Error(err.Error())
	h.notify(ctx, protocol.MethodWindowShowMessage, &protocol.ShowMessageParams{
		Message: err.Error(),
		Type:    protocol.MessageTypeError,
	})
}

func (h *handler) updateSymbols() error {
	pkg, err := gno.ParsePackage(h.workspaceFolder, "")
	if err != nil {
		return fmt.Errorf("updateSymbols: %w", err)
	}
	slog.Info("update workspace packages", "symbols", pkg.Symbols)
	h.symbols = pkg.Symbols
	return nil
}
