package gno

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jdkato/gnols/internal/store"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

var ErrNoGno = errors.New("no gno binary found")

// BinManager is a wrapper for the gno binary and related tooling.
//
// TODO: Should we install / update our own copy of gno?
type BinManager struct {
	gno              string // path to gno binary
	gnokey           string // path to gnokey binary
	gopls            string // path to gopls binary
	root             string // path to gno repository
	shouldPrecompile bool   // whether to precompile on save
	shouldBuild      bool   // whether to build on save
}

// BuildError is an error returned by the `gno build` command.
type BuildError struct {
	Path string
	Line int
	Span []int
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
func NewBinManager(gnoBin, gnokeyBin, goplsBin, root string, precompile, build bool) (*BinManager, error) {
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
	slog.Info("gopls init", "path", goplsBin)
	if goplsBin == "" {
		goplsBin, _ = exec.LookPath("gopls")
		slog.Info("gopls lookup", "path", goplsBin)
	}
	return &BinManager{
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

// Precompile a Gno package: gno precompile <dir>.
func (m *BinManager) Precompile(gnoDir string) ([]byte, error) {
	args := []string{"precompile", gnoDir}
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
// 1. Precompile the file;
// 2. build the file (using -gobuild precompile flag);
// 3. parse the errors; and
// 4. recompute the offsets (.go -> .gno).
//
// TODO: is this the best way?
func (m *BinManager) Lint(doc *store.Document) ([]BuildError, error) {
	pkg := pkgFromFile(doc.Path)

	if !m.shouldPrecompile && !m.shouldBuild {
		return []BuildError{}, nil
	}
	preOut, _ := m.Precompile(pkg)
	return parseError(doc, string(preOut), "precompile")
}

var goplsDefRe = regexp.MustCompile(`(.+):(\d+):(\d+)-(\d+): defined here as (.+) (.+)`)

// Definition returns the definition of the symbol at the given position
// using the `gopls` tool.
//
// TODO:
//
// * invalid types
// * add -json flag (better than regexp)
// * add handy BinManager.RunGoPls
func (m *BinManager) Definition(ctx context.Context, path string, line, col uint32) (protocol.Location, error) {
	// Location is 1-based and shifted down 4 lines.
	target := fmt.Sprintf("%s.gen.go:%d:%d", path, line+5, col+1)

	cmd := exec.CommandContext(ctx, m.gopls, "definition", target) //nolint:gosec
	// *.gen.go files have the gno build tag.
	// Must append to os.Environ or else gopls doesn't find the go binary.
	cmd.Env = append(os.Environ(), "GOFLAGS=-tags=gno")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	var bufErr bytes.Buffer
	cmd.Stderr = &bufErr

	err := cmd.Run()
	if err != nil {
		return protocol.Location{}, fmt.Errorf("'gopls definition %s' error :%s:%w", target, bufErr.String(), err)
	}

	matches := goplsDefRe.FindStringSubmatch(buf.String())
	if len(matches) != 7 {
		return protocol.Location{}, fmt.Errorf("invalid gopls output: %s", buf.String())
	}
	spath := strings.TrimSuffix(matches[1], ".gen.go")
	sline, err := strconv.Atoi(matches[2])
	if err != nil {
		return protocol.Location{}, fmt.Errorf("invalid gopls line output: %s", matches[2])
	}
	scharStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return protocol.Location{}, fmt.Errorf("invalid gopls char start output: %s", matches[3])
	}
	scharEnd, err := strconv.Atoi(matches[4])
	if err != nil {
		return protocol.Location{}, fmt.Errorf("invalid gopls char end output: %s", matches[4])
	}
	// Location is 1-based and shifted down 4 lines.
	sline -= 5
	scharStart--
	scharEnd--
	slog.Info("Definition matches", "path", spath, "line", sline, "char_start", scharStart, "char_end", scharEnd)

	return protocol.Location{
		URI: uri.File(spath),
		Range: protocol.Range{
			Start: protocol.Position{
				Line:      uint32(sline),
				Character: uint32(scharStart),
			},
			End: protocol.Position{
				Line:      uint32(sline),
				Character: uint32(scharEnd),
			},
		},
	}, nil
}
