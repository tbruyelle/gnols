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

func TestScripts(t *testing.T) {
	clientRead, serverWrite := io.Pipe()
	serverRead, clientWrite := io.Pipe()
	serverBuf := buffer{
		PipeWriter: serverWrite,
		PipeReader: serverRead,
	}
	clientBuf := buffer{
		PipeWriter: clientWrite,
		PipeReader: clientRead,
	}
	serverConn := jsonrpc2.NewConn(jsonrpc2.NewStream(serverBuf))
	handlerSrv := jsonrpc2.HandlerServer(handler.NewHandler(serverConn))
	clientConn := jsonrpc2.NewConn(jsonrpc2.NewStream(clientBuf))
	// ctx, cancel := context.WithCancel(context.Background())
	// _ = cancel
	ctx := context.Background()

	testscript.Run(t, testscript.Params{
		Setup: func(env *testscript.Env) error {
			return nil
		},
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"server": func(ts *testscript.TestScript, neg bool, args []string) {
				if len(args) == 0 {
					ts.Fatalf("lsp command expects at least one argument")
				}
				switch args[0] {
				case "start":
					go func() {
						ctx := context.WithValue(ctx, "name", "server")
						if err := handlerSrv.ServeStream(ctx, serverConn); !errors.Is(err, io.ErrClosedPipe) {
							ts.Fatalf("Server error: %v", err)
						}
					}()
					clientConn.Go(context.WithValue(ctx, "name", "client"), nil)

				case "stop":
					clientConn.Close()
					serverConn.Close()
					<-clientConn.Done()
					<-serverConn.Done()
				}
			},

			"lsp": func(ts *testscript.TestScript, neg bool, args []string) {
				if len(args) == 0 {
					ts.Fatalf("lsp command expects at least one argument")
				}
				var (
					workDir  = ts.Getenv("WORK")
					id       jsonrpc2.ID
					response any
				)
				switch args[0] {
				case "initialize":
					params := &protocol.InitializeParams{
						RootURI: uri.File(workDir),
					}
					var err error
					id, err = clientConn.Call(ctx, protocol.MethodInitialize, params, &response)
					if err != nil {
						ts.Fatalf("Error Call: %v", err)
					}
				default:
					ts.Fatalf("lsp command '%s' unknown", args[0])
				}

				// write response into $WORK/output
				bz, err := json.MarshalIndent(response, "", " ")
				if err != nil {
					ts.Fatalf("Error MarshalIndent: %v", err)
				}
				bz = append(bz, '\n') // txtar files always have a final newline
				filename := filepath.Join(workDir, "output", fmt.Sprintf("init%d.json", id))
				err = os.WriteFile(filename, bz, os.ModePerm)
				if err != nil {
					ts.Fatalf("Error WriteFile: %v", err)
				}
			},
		},
	})
}
