package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
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
			var (
				clientRead, serverWrite = io.Pipe()
				serverRead, clientWrite = io.Pipe()
				clisrv                  = clisrv{
					serverBuf: buffer{
						PipeWriter: serverWrite,
						PipeReader: serverRead,
					},
					clientBuf: buffer{
						PipeWriter: clientWrite,
						PipeReader: clientRead,
					},
				}
			)
			clisrv.serverConn = jsonrpc2.NewConn(jsonrpc2.NewStream(clisrv.serverBuf))
			clisrv.serverHandler = jsonrpc2.HandlerServer(handler.NewHandler(clisrv.serverConn))
			clisrv.clientConn = jsonrpc2.NewConn(jsonrpc2.NewStream(clisrv.clientBuf))
			env.Values["conn"] = clisrv.clientConn

			// Start LSP server
			var (
				ctx     = context.Background()
				workDir = env.Getenv("WORK")
				n       atomic.Uint32
			)
			go func() {
				if err := clisrv.serverHandler.ServeStream(ctx, clisrv.serverConn); !errors.Is(err, io.ErrClosedPipe) {
					env.T().Fatal("Server error", err)
				}
			}()
			clisrv.clientConn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
				// write server notification into $WORK/notify
				filename := filepath.Join(workDir, fmt.Sprintf("notify%d.json", n.Add(1)))
				writeJson(filename, req)
				return nil
			})

			// Stop LSP server at the end of test
			env.Defer(func() {
				clisrv.clientConn.Close()
				clisrv.serverConn.Close()
				<-clisrv.clientConn.Done()
				<-clisrv.serverConn.Done()
			})

			return nil
		},
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"lsp": func(ts *testscript.TestScript, neg bool, args []string) { //nolint:unparam
				if len(args) == 0 {
					ts.Fatalf("usage: lsp <command>")
				}
				var (
					workDir = ts.Getenv("WORK")
					method  = args[0]
				)
				switch method {

				case "initialize":
					params := &protocol.InitializeParams{
						RootURI: uri.File(workDir),
					}
					clientCall(ts, protocol.MethodInitialize, params)

				case "initialized":
					clientCall(ts, protocol.MethodInitialized, &protocol.InitializeParams{})

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
					clientCall(ts, protocol.MethodWorkspaceDidChangeConfiguration, params)

				case "textDocument-didOpen":
					if len(args) != 2 {
						ts.Fatalf("usage: lsp didOpenDocument <path>")
					}
					params := &protocol.DidOpenTextDocumentParams{
						TextDocument: protocol.TextDocumentItem{
							URI: uri.File(filepath.Join(workDir, args[1])),
						},
					}
					clientCall(ts, protocol.MethodTextDocumentDidOpen, params)

				default:
					ts.Fatalf("lsp method '%s' unknown", method)
				}
			},
		},
	})
}

func clientCall(ts *testscript.TestScript, method string, params any) {
	var (
		conn     = ts.Value("conn").(jsonrpc2.Conn)
		response any
	)
	id, err := conn.Call(context.Background(), method, params, &response)
	if err != nil {
		ts.Fatalf("Error Call: %v", err)
	}
	if response != nil {
		// write response into $WORK/output
		workDir := ts.Getenv("WORK")
		filename := filepath.Join(workDir, "output", fmt.Sprintf("%s%d.json", method, id))
		writeJson(filename, response)
	}
}

func writeJson(filename string, x any) {
	err := os.MkdirAll(filepath.Dir(filename), os.ModePerm)
	if err != nil {
		panic(err)
	}
	bz, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		panic(err)
	}
	bz = append(bz, '\n') // txtar files always have a final newline
	err = os.WriteFile(filename, bz, os.ModePerm)
	if err != nil {
		panic(err)
	}
}
