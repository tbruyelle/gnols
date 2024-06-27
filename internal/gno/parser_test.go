package gno_test

import (
	"encoding/json"
	"testing"

	"github.com/jdkato/gnols/internal/gno"
	"github.com/rogpeppe/go-internal/testscript"
)

func TestParsePackage(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"printSymbols": func(ts *testscript.TestScript, neg bool, arg []string) { //nolint:unparam
				pkg, err := gno.ParsePackage(ts.Getenv("WORK"), "")
				if err != nil {
					ts.Fatalf("gno.ParsePackage: %v", err)
				}
				bz, _ := json.MarshalIndent(pkg.Symbols, "", "  ")
				ts.Stdout().Write(bz)           //nolint:errcheck
				ts.Stdout().Write([]byte{'\n'}) //nolint:errcheck
			},
		},
	})
}
