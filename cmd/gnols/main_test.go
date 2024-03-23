package main

import (
	"context"
	"errors"
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
	ctx, cancel := context.WithCancel(context.Background())
	// var output bytes.Buffer

	testscript.Run(t, testscript.Params{
		Setup: func(env *testscript.Env) error {
			return nil
		},
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"runLspServer": func(ts *testscript.TestScript, neg bool, args []string) {
				go func() {
					ctx := context.WithValue(ctx, "name", "server")
					if err := handlerSrv.ServeStream(ctx, serverConn); !errors.Is(err, io.ErrClosedPipe) {
						ts.Fatalf("Server error: %v", err)
					}
				}()
				clientConn.Go(context.WithValue(ctx, "name", "client"), nil)
				// go func() {
				// r := bufio.NewScanner(clientBuf.PipeReader)
				// for r.Scan() {
				// fmt.Println("RCV", r.Text())
				// output.WriteString(r.Text())
				// }
				// fmt.Println("END OF READ")
				// }()
			},

			"lsp": func(ts *testscript.TestScript, neg bool, args []string) {
				fmt.Println("CALL")
				params := &protocol.InitializeParams{
					RootURI: uri.File(ts.Getenv("WORK")),
				}
				var res protocol.InitializeResult
				ctx := context.WithValue(ctx, "name", "client")
				id, err := clientConn.Call(ctx, protocol.MethodInitialize, params, &res)
				if err != nil {
					ts.Fatalf("Error call: %v", err)
				}
				fmt.Println("CALLID", id, res)
			},
			"assertRcv": func(ts *testscript.TestScript, neg bool, args []string) {
				// fmt.Println("RCV", buf)
			},
			"stopLspServer": func(ts *testscript.TestScript, neg bool, args []string) {
				fmt.Println("STOP")
				clientConn.Close()
				serverConn.Close()
				fmt.Println("STOPPED")
				cancel()
			},
		},
	})
}
