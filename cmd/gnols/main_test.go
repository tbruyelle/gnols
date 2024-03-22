package main

import (
	"context"
	"fmt"
	"io"
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
	// TODO declare 2 buffer, one for the client and one for the server
	pr, pw := io.Pipe()
	buf := buffer{
		PipeWriter: pw,
		PipeReader: pr,
	}
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(buf))
	handlerSrv := jsonrpc2.HandlerServer(handler.NewHandler(conn))

	testscript.Run(t, testscript.Params{
		Setup: func(env *testscript.Env) error {
			return nil
		},
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"runLspServer": func(ts *testscript.TestScript, neg bool, args []string) {
				go func() {
					if err := handlerSrv.ServeStream(context.Background(), conn); err != nil {
						// ts.Fatalf("Server error: %v", err)
						fmt.Println("ERROR", err)
					}
				}()
			},

			"lsp": func(ts *testscript.TestScript, neg bool, args []string) {
				fmt.Println("CALL")
				params := &protocol.InitializeParams{
					RootURI: uri.File(ts.Getenv("WORK")),
				}
				res := make(map[string]any)
				id, err := conn.Call(context.Background(), protocol.MethodInitialize, params, res)
				if err != nil {
					ts.Fatalf("Error call: %v", err)
				}
				fmt.Println("CALLID", id, res)
			},
			"assertRcv": func(ts *testscript.TestScript, neg bool, args []string) {
				fmt.Println("RCV", buf)
			},
		},
	})
}
