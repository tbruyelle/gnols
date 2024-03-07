package gno_test

import (
	"testing"

	"github.com/jdkato/gnols/internal/gno"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lsp.dev/uri"
)

func TestNewManager(t *testing.T) {
	mgr, _ := gno.NewBinManager("", "", "", "", "", false, false)
	if mgr != nil {
		t.Logf("gno bin: %s", mgr.GnoBin())
	} else {
		t.Log("gno bin: not found")
	}
}

func TestSpansFromPositions(t *testing.T) {
	tests := []struct {
		name          string
		positions     string
		expectedSpans []gno.Span
		expectedError string
	}{
		{
			name:          "empty",
			positions:     "",
			expectedError: "EOF",
		},
		{
			name:          "not a position",
			positions:     "foo",
			expectedError: "unexpected EOF",
		},
		{
			name:      "one valid position",
			positions: "dir/file.gno.gen.go:1:2-3",
			expectedSpans: []gno.Span{
				{
					URI:   uri.File("dir/file.gno.gen.go"),
					Start: gno.Location{Line: 1, Column: 2},
					End:   gno.Location{Line: 1, Column: 3},
				},
			},
		},
		{
			name: "multiple valid positions",
			positions: `
dir/file.gno.gen.go:1:2-3
dir/file2.gno.gen.go:4:5-6
`,
			expectedSpans: []gno.Span{
				{
					URI:   uri.File("dir/file.gno.gen.go"),
					Start: gno.Location{Line: 1, Column: 2},
					End:   gno.Location{Line: 1, Column: 3},
				},
				{
					URI:   uri.File("dir/file2.gno.gen.go"),
					Start: gno.Location{Line: 4, Column: 5},
					End:   gno.Location{Line: 4, Column: 6},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)

			spans, err := gno.SpansFromPositions(tt.positions)

			require.NoError(err)
			assert.Equal(tt.expectedSpans, spans)
		})
	}
}
