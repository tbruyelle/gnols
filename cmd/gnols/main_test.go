package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jdkato/gnols/internal/handler"
	"github.com/rogpeppe/go-internal/testscript"
	"go.lsp.dev/jsonrpc2"
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
			ctx := context.Background()
			go func() {
				if err := serverHandler.ServeStream(ctx, serverConn); !errors.Is(err, io.ErrClosedPipe) {
					env.T().Fatal("Server error", err)
				}
			}()
			// Listen to server notifications
			var notifyNum atomic.Uint32
			clientConn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
				// write server notifications into $WORK/output/notify{++notifyNum}.json
				filename := fmt.Sprintf("notify%d.json", notifyNum.Add(1))
				return writeJSON(env, filename, req)
			})

			// Stop LSP server at the end of test
			env.Defer(func() {
				clientConn.Close()
				serverConn.Close()
				<-clientConn.Done()
				<-serverConn.Done()
			})

			env.Setenv("GOBIN", filepath.Join(os.Getenv("HOME"), "go", "bin"))
			/*
				NOTE disabled until a txtar requires to have access to the gno repo
				gnoPath := os.Getenv("GNOPATH")
				if gnoPath == "" {
					t.Fatal("must GNOPATH env to gno repo directory")
				}
				env.Setenv("GNOPATH", gnoPath)
			*/

			return nil
		},
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			// "lsp" sends a lsp command to the server with the following arguments:
			// - the method name
			// - the path to the file that contains the method parameters
			// The server's response is encoded into the $WORK/output directory, with
			// filename equals to the parameter filename.
			"lsp": func(ts *testscript.TestScript, neg bool, args []string) { //nolint:unparam
				if len(args) != 2 {
					ts.Fatalf("usage: lsp <method> <param_file>")
				}
				var (
					method     = args[0]
					paramsFile = args[1]
				)
				call(ts, method, paramsFile)
			},
		},
	})
}

// call decodes paramFile and send it to the server using method.
func call(ts *testscript.TestScript, method string, paramFile string) {
	paramStr := ts.ReadFile(paramFile)
	// Replace $WORK with real path
	paramStr = os.Expand(paramStr, func(key string) string {
		if strings.HasPrefix(key, "FILE_") {
			// replace with the corresponding file
			fileContent := ts.ReadFile(key[5:])
			// Escape fileContent for JSON format
			bz, err := json.Marshal(fileContent)
			if err != nil {
				ts.Fatalf("encode key %s %q: %v", key, fileContent, err)
			}
			return string(bz[1 : len(bz)-1]) // remove quote wrapping
		}
		return ts.Getenv(key)
	})
	var params any
	if err := json.Unmarshal([]byte(paramStr), &params); err != nil {
		ts.Fatalf("decode param file %s: %v", paramFile, err)
	}
	var (
		conn     = ts.Value("conn").(jsonrpc2.Conn) //nolint:errcheck
		response any
	)
	_, err := conn.Call(context.Background(), method, params, &response)
	if err != nil {
		if jerr, ok := err.(*jsonrpc2.Error); ok { //nolint:errorlint
			// output jsonrpc2.Error in json
			response = map[string]any{"error": jerr}
		} else {
			ts.Fatalf("Error Call: %v", err)
		}
	}
	if err := writeJSON(ts, filepath.Base(paramFile), response); err != nil {
		ts.Fatalf("writeJSON: %v", err)
	}
}

// writeJSON writes x to $WORK/output/filename
func writeJSON(ts interface{ Getenv(string) string }, filename string, x any) error {
	workDir := ts.Getenv("WORK")
	filename = filepath.Join(workDir, "output", filename)
	err := os.MkdirAll(filepath.Dir(filename), os.ModePerm)
	if err != nil {
		return err
	}
	bz, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return err
	}
	bz = append(bz, '\n') // txtar files always have a final newline
	return os.WriteFile(filename, bz, os.ModePerm)
}
