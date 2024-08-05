package gno

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/sourcegraph/go-diff/diff"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

const (
	genGoExt       = ".gen.go"
	genGoLineShift = 5
)

var ErrNoGno = errors.New("no gno binary found")

// BinManager is a wrapper for the gno binary and related tooling.
//
// TODO: Should we install / update our own copy of gno?
type BinManager struct {
	workspaceFolder string // path to project
	gno             string // path to gno binary
	gnokey          string // path to gnokey binary
	gopls           string // path to gopls binary
	root            string // path to gno repository
	shouldTranspile bool   // whether to transpile on save
	shouldBuild     bool   // whether to build on save
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
// `transpile`: Whether to transpile Gno files on save.
// `build`: Whether to build Gno files on save.
//
// NOTE: Unlike `gnoBin`, `gnokeyBin` is optional.
func NewBinManager(workspaceFolder, gnoBin, gnokeyBin, goplsBin, root string, transpile, build bool) (*BinManager, error) {
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
		workspaceFolder: workspaceFolder,
		gno:             gnoBin,
		gnokey:          gnokeyBin,
		gopls:           goplsBin,
		root:            root,
		shouldTranspile: transpile,
		shouldBuild:     build,
	}, nil
}

// GnoBin returns the path to the `gno` binary.
//
// This is either user-provided or found on the user's PATH.
func (m *BinManager) GnoBin() string {
	return m.gno
}

func (m *BinManager) RunGopls(ctx context.Context, args ...string) ([]byte, error) {
	// Prepare call to gopls
	cmd := exec.CommandContext(ctx, m.gopls, args...) //nolint:gosec
	// *.gen.go files have the gno build tag.
	// Must append to os.Environ() or else gopls doesn't find the go binary.
	const goFlags = "GOFLAGS=-tags=gno"
	cmd.Env = append(os.Environ(), goFlags)
	var buf, bufErr bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &bufErr
	cmd.Dir = m.workspaceFolder
	// Run command
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("'gopls %s' error :%s:%w", strings.Join(args, " "),
			bufErr.String(), err)
	}
	return buf.Bytes(), nil
}

// Format a Gno file using gno fmt.
func (m *BinManager) Format(gnoFile string) ([]byte, error) {
	args := []string{"fmt", gnoFile}
	bz, err := exec.Command(m.gno, args...).CombinedOutput() //nolint:gosec
	if err != nil {
		return bz, fmt.Errorf("running '%s %s': %w: %s", m.gno, strings.Join(args, " "), err, string(bz))
	}
	return bz, nil
}

// Transpile a Gno package: gno transpile <m.workspaceFolder>.
func (m *BinManager) Transpile() ([]byte, error) {
	args := []string{"transpile", m.workspaceFolder}
	if m.shouldBuild {
		args = append(args, "-gobuild")
	}
	cmd := exec.Command(m.gno, args...)
	// FIXME(tb): See https://github.com/gnolang/gno/pull/1695/files#r1697255524
	const disableGoMod = "GO111MODULE=off"
	cmd.Env = append(os.Environ(), disableGoMod)
	bz, err := cmd.CombinedOutput()
	if err != nil {
		return bz, fmt.Errorf("running '%s %s': %w: %s", m.gno, strings.Join(args, " "), err, string(bz))
	}
	return bz, nil
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

// Lint transpiles and builds a Gno package and returns any errors.
//
// In practice, this means:
//
// 1. Transpile
// 2. Parse the errors
func (m *BinManager) Lint() ([]BuildError, error) {
	if !m.shouldTranspile && !m.shouldBuild {
		return []BuildError{}, nil
	}

	preOut, errTranspile := m.Transpile()
	// parse errors even if errTranspile!=nil bc that's always the case if
	// there's errors.
	errors, errParse := m.parseErrors(string(preOut), "transpile")
	if errParse != nil {
		return nil, errParse
	}
	if len(errors) == 0 && errTranspile != nil {
		// no errors but errTranspile!=nil, this isn't the expected error
		return nil, errTranspile
	}
	return errors, nil
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

type DocumentEdit struct {
	URI   uri.URI
	Edits []Edit
}

type Edit struct {
	Location
	NewText string
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
	if s.IsGenGo() {
		panic(fmt.Sprintf("span %v is not a .gno referrence", s))
	}
	// Remove .gen.go extention, we want to target the gno file
	s.URI = uri.New(string(s.URI) + genGoExt)
	// Shift lines
	s.Start.Line += genGoLineShift
	s.End.Line += genGoLineShift
	return s
}

func (s Span) IsGenGo() bool {
	return strings.Contains(string(s.URI), genGoExt)
}

// GenGo2Gno shifts the .gen.go Span s into a .gno Span.
func (s Span) GenGo2Gno() Span {
	if !s.IsGenGo() {
		panic(fmt.Sprintf("span %v is not a .gen.go referrence", s))
	}
	// Remove .gen.go extention, we want to target the gno file
	s.URI = uri.New(strings.ReplaceAll(string(s.URI), genGoExt, ""))
	// Shift lines
	s.Start.Line -= genGoLineShift
	s.End.Line -= genGoLineShift
	return s
}

// ToLocation converts s to a protocol.Location.
// NOTE: In LSP, a position inside a document is expressed as a zero-based line
// and character offset, thus we need to decrement by one the span Start and
// End position.
func (s Span) ToLocation() protocol.Location {
	return protocol.Location{
		URI: s.URI,
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      s.Start.Line - 1,
				Character: s.Start.Column - 1,
			},
			End: protocol.Position{
				Line:      s.End.Line - 1,
				Character: s.End.Column - 1,
			},
		},
	}
}

func NewSpan(path string, line, col1, col2 int) Span {
	return Span{
		URI: uri.File(path),
		Start: Location{
			Line:   uint32(line),
			Column: uint32(col1),
		},
		End: Location{
			Line:   uint32(line),
			Column: uint32(col2),
		},
	}
}

// SpanFromLSPLocation converts a protocol.Location to a Span.
// NOTE: In LSP, a position inside a document is expressed as a zero-based line
// and character offset, thus we need to decrement by one the span Start and
// End position.
func SpanFromLSPLocation(uri uri.URI, line, col uint32) Span {
	return Span{
		URI: uri,
		Start: Location{
			Line:   line + 1,
			Column: col + 1,
		},
	}
}

var rePosition = regexp.MustCompile(`(?m)^(\S+):(\d+):(\d+)-(\d+)$`)

func SpansFromPositions(positions string) ([]Span, error) {
	var spans []Span
	matches := rePosition.FindAllStringSubmatch(positions, -1)
	for _, match := range matches {
		filename := match[1]
		line, err := strconv.Atoi(match[2])
		if err != nil {
			return nil, fmt.Errorf("SpanFromPosition '%s': %w", match, err)
		}

		col1, err := strconv.Atoi(match[3])
		if err != nil {
			return nil, fmt.Errorf("SpanFromPosition '%s': %w", match, err)
		}
		col2, err := strconv.Atoi(match[4])
		if err != nil {
			return nil, fmt.Errorf("SpanFromPosition '%s': %w", match, err)
		}
		spans = append(spans, NewSpan(filename, line, col1, col2))
	}
	return spans, nil
}

// Definition returns the definition of the symbol at the given position
// using the `gopls` tool.
//
// TODO:
// * move gnols stuff in an other package
func (m *BinManager) Definition(ctx context.Context, uri uri.URI, line, col uint32) (GoplsDefinition, error) {
	target := SpanFromLSPLocation(uri, line, col).Gno2GenGo().Position()
	slog.Info("fetching definition", "uri", uri, "line", line, "col", col, "target", target)

	bz, err := m.RunGopls(ctx, "definition", "-json", target)
	if err != nil {
		return GoplsDefinition{}, err
	}

	// Parse output
	var def GoplsDefinition
	if err := json.Unmarshal(bz, &def); err != nil {
		return GoplsDefinition{}, fmt.Errorf("unexpected gopls definition output: %w", err)
	}
	// Turn back span to .gno file.
	def.Span = def.Span.GenGo2Gno()
	slog.Info("definition found", "position", def.Span.Position())
	return def, nil
}

// References returns the references of the symbol at the given position using
// the `gopls` tool.
func (m *BinManager) References(ctx context.Context, uri uri.URI, line, col uint32) ([]Span, error) {
	target := SpanFromLSPLocation(uri, line, col).Gno2GenGo().Position()
	slog.Info("fetching references", "uri", uri, "line", line, "col", col, "target", target)

	bz, err := m.RunGopls(ctx, "references", "-d", target)
	if err != nil {
		return nil, err
	}
	spans, err := SpansFromPositions(string(bz))
	if err != nil {
		return nil, err
	}
	// Turn back span to .gno file.
	for i := 0; i < len(spans); i++ {
		spans[i] = spans[i].GenGo2Gno()
	}
	slog.Info("found references", "spans", spans)
	return spans, nil
}

// Implementation returns the implementations of the symbol at the given
// position using the `gopls` tool.
func (m *BinManager) Implementation(ctx context.Context, uri uri.URI, line, col uint32) ([]Span, error) {
	target := SpanFromLSPLocation(uri, line, col).Gno2GenGo().Position()
	slog.Info("fetching implementation", "uri", uri, "line", line, "col", col, "target", target)

	bz, err := m.RunGopls(ctx, "implementation", target)
	if err != nil {
		return nil, err
	}
	spans, err := SpansFromPositions(string(bz))
	if err != nil {
		return nil, err
	}
	// Turn back span to .gno file.
	for i := 0; i < len(spans); i++ {
		spans[i] = spans[i].GenGo2Gno()
	}
	slog.Info("found implementation", "spans", spans)
	return spans, nil
}

func (m *BinManager) PrepareRename(ctx context.Context, file uri.URI, line, col uint32) error {
	target := SpanFromLSPLocation(file, line, col).Gno2GenGo().Position()
	slog.Info("prepare_rename", "uri", file, "line", line, "col", col, "target", target)
	_, err := m.RunGopls(ctx, "prepare_rename", target)
	return err
}

func (m *BinManager) Rename(ctx context.Context, file uri.URI, line, col uint32, newName string) ([]DocumentEdit, error) {
	target := SpanFromLSPLocation(file, line, col).Gno2GenGo().Position()
	slog.Info("rename", "uri", file, "line", line, "col", col, "target", target)

	bz, err := m.RunGopls(ctx, "rename", "-d", target, newName)
	if err != nil {
		return nil, err
	}
	// gopls rename returns a diff, parse it
	diffs, err := diff.ParseMultiFileDiff(bz)
	if err != nil {
		return nil, err
	}
	var docEdits []DocumentEdit
	for _, f := range diffs {
		var docEdit DocumentEdit
		for _, h := range f.Hunks {
			var lineCount uint32
			for _, line := range strings.Split(string(h.Body), "\n") {
				if len(line) > 0 {
					switch line[0] {
					case ' ': // regular line, increment lineCount
						lineCount++
					case '-': // deletion line, ignore
						break
					case '+': // add line, found the newText
						newText := line[1:] + "\n"
						span := Span{
							URI: uri.File(f.NewName),
							Start: Location{
								Line:   uint32(h.NewStartLine) + lineCount,
								Column: 1,
							},
						}.GenGo2Gno()
						docEdit.URI = span.URI
						docEdit.Edits = append(docEdit.Edits, Edit{
							Location: span.Start,
							NewText:  newText,
						})
						lineCount++
					}
				}
			}
		}
		docEdits = append(docEdits, docEdit)
	}
	return docEdits, nil
}
