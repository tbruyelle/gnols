package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
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

func TestScripts(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Setup: func(env *testscript.Env) error {
			var (
				clientRead, serverWrite = io.Pipe()
				serverRead, clientWrite = io.Pipe()
				serverBuf               = buffer{
					PipeWriter: serverWrite,
					PipeReader: serverRead,
				}
				clientBuf = buffer{
					PipeWriter: clientWrite,
					PipeReader: clientRead,
				}
				serverConn    = jsonrpc2.NewConn(jsonrpc2.NewStream(serverBuf))
				serverHandler = jsonrpc2.HandlerServer(handler.NewHandler(serverConn))
				clientConn    = jsonrpc2.NewConn(jsonrpc2.NewStream(clientBuf))
			)
			env.Values["conn"] = clientConn

			// Start LSP server
			var (
				ctx       = context.Background()
				workDir   = env.Getenv("WORK")
				notifyNum atomic.Uint32
			)
			go func() {
				if err := serverHandler.ServeStream(ctx, serverConn); !errors.Is(err, io.ErrClosedPipe) {
					env.T().Fatal("Server error", err)
				}
			}()
			clientConn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
				// write server notification into $WORK/notify
				filename := filepath.Join(workDir, fmt.Sprintf("notify%d.json", notifyNum.Add(1)))
				writeJSON(filename, req)
				return nil
			})

			// Stop LSP server at the end of test
			env.Defer(func() {
				clientConn.Close()
				serverConn.Close()
				<-clientConn.Done()
				<-serverConn.Done()
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
				// TODO use `case protocol.MethodInitialize:` or even better avoid
				// `swicth method` and rely only on txtar content to create the params
				// Problem: that makes txtar harder to read and interpret.
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
						ts.Fatalf("usage: lsp textDocument-didOpen <path>")
					}
					params := &protocol.DidOpenTextDocumentParams{
						TextDocument: protocol.TextDocumentItem{
							URI: uri.File(filepath.Join(workDir, args[1])),
						},
					}
					clientCall(ts, protocol.MethodTextDocumentDidOpen, params)

				case "textDocument-definition":
					if len(args) != 4 {
						ts.Fatalf("usage: lsp textDocument-definition <path> <line> <col>")
					}
					file := args[1]
					line, err := strconv.Atoi(args[2])
					if err != nil {
						ts.Fatalf("usage: lsp textDocument-definition <path> <line> <col>")
					}
					col, err := strconv.Atoi(args[3])
					if err != nil {
						ts.Fatalf("usage: lsp textDocument-definition <path> <line> <col>")
					}
					params := &protocol.DefinitionParams{
						TextDocumentPositionParams: protocol.TextDocumentPositionParams{
							TextDocument: protocol.TextDocumentIdentifier{
								URI: uri.File(filepath.Join(workDir, file)),
							},
							Position: protocol.Position{
								Line:      uint32(line),
								Character: uint32(col),
							},
						},
					}
					clientCall(ts, protocol.MethodTextDocumentDefinition, params)

				default:
					ts.Fatalf("lsp method '%s' unknown", method)
				}
			},
		},
	})
}

func clientCall(ts *testscript.TestScript, method string, params any) {
	var (
		conn     = ts.Value("conn").(jsonrpc2.Conn) //nolint:errcheck
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
		writeJSON(filename, response)
	}
}

func writeJSON(filename string, x any) {
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
