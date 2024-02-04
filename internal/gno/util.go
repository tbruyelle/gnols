package gno

import (
	"log/slog"
	"path/filepath"
	"regexp"
	"strconv"

	"go.lsp.dev/uri"
)

// This is used to extract information from the `gno build` command
// (see `parseError` below).
//
// TODO: Maybe there's a way to get this in a structured format?
var errorRe = regexp.MustCompile(`(?m)^([^#]+?):(\d+):(\d+):(.+)$`)

// parseErrors parses the output of the `gno precompile -gobuild` command for
// errors.
//
// They look something like this:
//
// ```
// command-line-arguments
// # command-line-arguments
// <file>:20:9: undefined: strin
//
// <pkg_path>: build pkg: std go compiler: exit status 1
//
// 1 go build errors
// ```
func (m *BinManager) parseErrors(output, cmd string) ([]BuildError, error) {
	errors := []BuildError{}

	matches := errorRe.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return errors, nil
	}

	for _, match := range matches {
		path := match[1]
		line, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, err
		}

		column, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, err
		}
		msg := match[4]
		slog.Info("parsing", "path", path, "line", line, "column", column, "msg", msg)
		span := Span{
			URI: uri.File(filepath.Join(m.workspaceFolder, path)),
			Start: Location{
				Line:   uint32(line),
				Column: uint32(column),
			},
			End: Location{
				Line:   uint32(line),
				Column: uint32(column),
			},
		}
		// Shift from .gen.go to .gno
		span = span.GenGo2Gno()
		errors = append(errors, BuildError{
			Span: span,
			Msg:  msg,
			Tool: cmd,
		})
	}

	return errors, nil
}
