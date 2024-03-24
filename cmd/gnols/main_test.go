package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jdkato/gnols/internal/handler"
	"github.com/rogpeppe/go-internal/testscript"
	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

type buffer struct {
	*io.PipeWriter
	*io.PipeReader
}

func (b buffer) Close() error {
	b.PipeReader.Close()
	b.PipeWriter.Close()
	return nil
}

type clisrv struct {
	serverBuf     buffer
	clientBuf     buffer
	serverHandler jsonrpc2.StreamServer
	clientConn    jsonrpc2.Conn
	serverConn    jsonrpc2.Conn
}

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Setup: func(env *testscript.Env) error {
			clientRead, serverWrite := io.Pipe()
			serverRead, clientWrite := io.Pipe()
			clisrv := clisrv{
				serverBuf: buffer{
					PipeWriter: serverWrite,
					PipeReader: serverRead,
				},
				clientBuf: buffer{
					PipeWriter: clientWrite,
					PipeReader: clientRead,
				},
			}
			clisrv.serverConn = jsonrpc2.NewConn(jsonrpc2.NewStream(clisrv.serverBuf))
			clisrv.serverHandler = jsonrpc2.HandlerServer(handler.NewHandler(clisrv.serverConn))
			clisrv.clientConn = jsonrpc2.NewConn(jsonrpc2.NewStream(clisrv.clientBuf))
			env.Values["clisrv"] = clisrv

			return nil
		},
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"server": func(ts *testscript.TestScript, neg bool, args []string) { //nolint:unparam
				if len(args) == 0 {
					ts.Fatalf("lsp command expects at least one argument")
				}
				var (
					workDir = ts.Getenv("WORK")
					clisrv  = getCliSrv(ts)
					ctx     = context.Background()
				)
				switch args[0] {
				case "start":
					go func() {
						if err := clisrv.serverHandler.ServeStream(ctx, clisrv.serverConn); !errors.Is(err, io.ErrClosedPipe) {
							ts.Fatalf("Server error: %v", err)
						}
					}()
					clisrv.clientConn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
						// write server notification into $WORK/notify
						filename := filepath.Join(workDir, fmt.Sprintf("notify.json"))
						writeJson(ts, filename, req)
						return nil
					})

				case "stop":
					clisrv.clientConn.Close()
					clisrv.serverConn.Close()
					<-clisrv.clientConn.Done()
					<-clisrv.serverConn.Done()
				}
			},

			"lsp": func(ts *testscript.TestScript, neg bool, args []string) { //nolint:unparam
				if len(args) == 0 {
					ts.Fatalf("usage: lsp <command>")
				}
				var (
					workDir  = ts.Getenv("WORK")
					method   = args[0]
					id       jsonrpc2.ID
					response any
					clisrv   = getCliSrv(ts)
					ctx      = context.Background()
				)
				switch method {

				case "initialize":
					params := &protocol.InitializeParams{
						RootURI: uri.File(workDir),
					}
					var err error
					id, err = clisrv.clientConn.Call(ctx, protocol.MethodInitialize, params, &response)
					if err != nil {
						ts.Fatalf("Error Call: %v", err)
					}

				case "initialized":
					params := &protocol.InitializeParams{}
					var err error
					id, err = clisrv.clientConn.Call(ctx, protocol.MethodInitialized, params, &response)
					if err != nil {
						ts.Fatalf("Error Call: %v", err)
					}

				case "didChangeConfiguration":
					params := &protocol.DidChangeConfigurationParams{
						Settings: map[string]any{
							"gno":              "/home/tom/go/bin/gno",
							"gopls":            "/home/tom/go/bin/gopls",
							"root":             "/home/tom/src/gno",
							"precompileOnSave": true,
							"buildOnSave":      true,
						},
					}
					var err error
					id, err = clisrv.clientConn.Call(ctx, protocol.MethodWorkspaceDidChangeConfiguration, params, &response)
					if err != nil {
						ts.Fatalf("Error Call: %v", err)
					}

				case "textDocument-didOpen":
					if len(args) != 2 {
						ts.Fatalf("usage: lsp didOpenDocument <path>")
					}
					params := &protocol.DidOpenTextDocumentParams{
						TextDocument: protocol.TextDocumentItem{
							URI: uri.File(filepath.Join(workDir, args[1])),
						},
					}
					var err error
					id, err = clisrv.clientConn.Call(ctx, protocol.MethodTextDocumentDidOpen, params, &response)
					if err != nil {
						ts.Fatalf("Error Call: %v", err)
					}

				default:
					ts.Fatalf("lsp method '%s' unknown", method)
				}

				if response != nil {
					// write response into $WORK/output
					filename := filepath.Join(workDir, "output", fmt.Sprintf("%s%d.json", method, id))
					writeJson(ts, filename, response)
				}
			},
		},
	})
}

func getCliSrv(ts *testscript.TestScript) clisrv {
	return ts.Value("clisrv").(clisrv)
}

func writeJson(ts *testscript.TestScript, filename string, x any) {
	err := os.MkdirAll(filepath.Dir(filename), os.ModePerm)
	if err != nil {
		ts.Fatalf("Error MkdirAll: %v", err)
	}
	bz, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		ts.Fatalf("Error MarshalIndent: %v", err)
	}
	bz = append(bz, '\n') // txtar files always have a final newline
	err = os.WriteFile(filename, bz, os.ModePerm)
	if err != nil {
		ts.Fatalf("Error WriteFile: %v", err)
	}
}
