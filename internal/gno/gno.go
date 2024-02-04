package gno

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

var ErrNoGno = errors.New("no gno binary found")

// BinManager is a wrapper for the gno binary and related tooling.
//
// TODO: Should we install / update our own copy of gno?
type BinManager struct {
	workspaceFolder  string // path to project
	gno              string // path to gno binary
	gnokey           string // path to gnokey binary
	gopls            string // path to gopls binary
	root             string // path to gno repository
	shouldPrecompile bool   // whether to precompile on save
	shouldBuild      bool   // whether to build on save
}

// BuildError is an error returned by the `gno build` command.
type BuildError struct {
	Span Span
	Msg  string
	Tool string
}

// NewBinManager returns a new GnoManager.
//
// If the user does not provide a path to the required binaries, we search the
// user's PATH for them.
//
// `gnoBin`: The path to the `gno` binary.
// `gnokeyBin`: The path to the `gnokey` binary.
// `goplsBin`: The path to the `gopls` binary.
// `root`: The path to the `gno` repository
// `precompile`: Whether to precompile Gno files on save.
// `build`: Whether to build Gno files on save.
//
// NOTE: Unlike `gnoBin`, `gnokeyBin` is optional.
func NewBinManager(workspaceFolder, gnoBin, gnokeyBin, goplsBin, root string, precompile, build bool) (*BinManager, error) {
	if gnoBin == "" {
		var err error
		gnoBin, err = exec.LookPath("gno")
		if err != nil {
			return nil, ErrNoGno
		}
	}
	if gnokeyBin == "" {
		gnokeyBin, _ = exec.LookPath("gnokey")
	}
	if goplsBin == "" {
		goplsBin, _ = exec.LookPath("gopls")
	}
	return &BinManager{
		workspaceFolder:  workspaceFolder,
		gno:              gnoBin,
		gnokey:           gnokeyBin,
		gopls:            goplsBin,
		root:             root,
		shouldPrecompile: precompile,
		shouldBuild:      build,
	}, nil
}

// GnoBin returns the path to the `gno` binary.
//
// This is either user-provided or found on the user's PATH.
func (m *BinManager) GnoBin() string {
	return m.gno
}

// Gopls returns the path to the `gopls` binary.
func (m *BinManager) Gopls() string {
	return m.gopls
}

// Format a Gno file using std formatter.
//
// TODO: support other tools?
func (m *BinManager) Format(gnoFile string) ([]byte, error) {
	return format.Source([]byte(gnoFile))
}

// Precompile a Gno package: gno precompile <m.workspaceFolder>.
func (m *BinManager) Precompile() ([]byte, error) {
	args := []string{"precompile", m.workspaceFolder}
	if m.shouldBuild {
		args = append(args, "-gobuild")
	}
	return exec.Command(m.gno, args...).CombinedOutput() //nolint:gosec
}

// RunTest runs a Gno test:
//
// gno test -timeout 30s -run ^TestName$ <pkg_path>
func (m *BinManager) RunTest(pkg, name string) ([]byte, error) {
	cmd := exec.Command( //nolint:gosec
		m.gno,
		"test",
		"-root-dir",
		m.root,
		"-verbose",
		"-timeout",
		"30s",
		"-run",
		fmt.Sprintf("^%s$", name),
		pkg,
	)
	cmd.Dir = pkg
	return cmd.CombinedOutput()
}

// Lint precompiles and builds a Gno package and returns any errors.
//
// In practice, this means:
//
// 1. Precompile
// 2. parse the errors
// 3. recompute the offsets (.gen.go -> .gno).
func (m *BinManager) Lint() ([]BuildError, error) {
	if !m.shouldPrecompile && !m.shouldBuild {
		return []BuildError{}, nil
	}
	preOut, _ := m.Precompile() // TODO handle error?
	return m.parseErrors(string(preOut), "precompile")
}

type GoplsDefinition struct {
	Span        Span
	Description string
}

type Span struct {
	URI   uri.URI
	Start Location
	End   Location
}

type Location struct {
	Line   uint32
	Column uint32
	Offset uint32
}

// Position returns s position in format filename:line:column.
func (s Span) Position() string {
	return fmt.Sprintf("%s:%d:%d", s.URI.Filename(), s.Start.Line, s.Start.Column)
}

// GenGo2Gno shifts the .gno Span s into a .gen.go Span.
func (s Span) Gno2GenGo() Span {
	if strings.Contains(string(s.URI), ".gen.go") {
		panic(fmt.Sprintf("span %v is not a .gno referrence", s))
	}
	// Remove .gen.go extention, we want to target the gno file
	s.URI = uri.New(string(s.URI) + ".gen.go")
	// Shift lines & columns
	s.Start.Line += 5
	s.Start.Column++
	s.End.Line += 5
	s.End.Column++
	return s
}

// GenGo2Gno shifts the .gen.go Span s into a .gno Span.
func (s Span) GenGo2Gno() Span {
	if !strings.Contains(string(s.URI), ".gen.go") {
		panic(fmt.Sprintf("span %v is not a .gen.go referrence", s))
	}
	// Remove .gen.go extention, we want to target the gno file
	s.URI = uri.New(strings.ReplaceAll(string(s.URI), ".gen.go", ""))
	// Shift lines & columns
	s.Start.Line -= 5
	s.Start.Column--
	s.End.Line -= 5
	s.End.Column--
	return s
}

func (s Span) ToLocation() protocol.Location {
	return protocol.Location{
		URI: s.URI,
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      s.Start.Line,
				Character: s.Start.Column,
			},
			End: protocol.Position{
				Line:      s.End.Line,
				Character: s.End.Column,
			},
		},
	}
}

// Definition returns the definition of the symbol at the given position
// using the `gopls` tool.
//
// TODO:
//
// * add handy BinManager.RunGoPls
// * move gnols stuff in an other packahe
func (m *BinManager) Definition(ctx context.Context, uri uri.URI, line, col uint32) (GoplsDefinition, error) {
	// Build a reference to the .gen.gno file position
	target := Span{
		URI: uri,
		Start: Location{
			Line:   line,
			Column: col,
		},
	}.Gno2GenGo().Position()
	slog.Info("fetching definition", "uri", uri, "line", line, "col", col, "target", target)

	// Prepare call to gopls
	cmd := exec.CommandContext(ctx, m.gopls, "definition", "-json", target) //nolint:gosec
	// *.gen.go files have the gno build tag.
	// Must append to os.Environ() or else gopls doesn't find the go binary.
	cmd.Env = append(os.Environ(), "GOFLAGS=-tags=gno")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	var bufErr bytes.Buffer
	cmd.Stderr = &bufErr

	// Run command
	err := cmd.Run()
	if err != nil {
		return GoplsDefinition{}, fmt.Errorf("'gopls definition %s' error :%s:%w", target, bufErr.String(), err)
	}

	// Parse output
	var def GoplsDefinition
	if err := json.Unmarshal(buf.Bytes(), &def); err != nil {
		return GoplsDefinition{}, fmt.Errorf("unexpected gopls definition output: %w", err)
	}
	// Turn back span to .gno file.
	def.Span = def.Span.GenGo2Gno()
	slog.Info("definition found", "position", def.Span.Position())
	return def, nil
}
