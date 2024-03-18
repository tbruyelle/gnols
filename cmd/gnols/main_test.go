package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jdkato/gnols/internal/handler"
	"github.com/rogpeppe/go-internal/testscript"
	"go.lsp.dev/jsonrpc2"
)

func TestScripts(t *testing.T) {
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(stdrwc{}))

	testscript.Run(t, testscript.Params{
		Setup: func(env *testscript.Env) error {
			handler := handler.NewHandler(conn)
			handlerSrv := jsonrpc2.HandlerServer(handler)

			go func() {
				if err := handlerSrv.ServeStream(context.Background(), conn); err != nil {
					fmt.Printf("Server error: %v\n", err)
					env.T().Fatal("Server error: %v", err)
				}
			}()
			time.Sleep(time.Second)
			return nil
		},
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"lspcli": func(ts *testscript.TestScript, neg bool, args []string) {
				fmt.Println("NOTIFY")
				err := conn.Notify(context.Background(), "xxx", nil)
				if err != nil {
					ts.Fatalf("Error notify: %v", err)
				}
			},
		},
	})
}
