package gno

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
)

// This is used to extract information from the `go build` command
// (see `parseError` below).
//
// TODO: Maybe there's a way to get this in a structured format?
// Yes, when this change is released: https://go-review.googlesource.com/c/go/+/536397
// related issue: https://github.com/golang/go/issues/62067
var reGoBuildError = regexp.MustCompile(`(?m)^([^#]+?):(\d+):(\d+):(.+)$`)

// parseErrors parses the output of the `gno transpile -gobuild` command for
// errors.
//
// The format is:
// ```
// <file.gno>:<line>:<col>: <error>
// ```
func (m *BinManager) parseErrors(output, cmd string) ([]BuildError, error) {
	errors := []BuildError{}

	matches := reGoBuildError.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return errors, nil
	}

	for _, match := range matches {
		path := match[1]
		line, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, fmt.Errorf("parseErrors '%s': %w", match, err)
		}

		column, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, fmt.Errorf("parseErrors '%s': %w", match, err)
		}
		msg := match[4]
		span := NewSpan(filepath.Join(m.workspaceFolder, path), line, column, column)
		errors = append(errors, BuildError{
			Span: span,
			Msg:  msg,
			Tool: cmd,
		})
	}

	return errors, nil
}
