package store

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"log/slog"
	"strings"

	"github.com/jdkato/gnols/internal/gno"
)

// A ParsedGnoFile contains the results of parsing a Gno file.
type ParsedGnoFile struct {
	File    *ast.File
	FileSet *token.FileSet
	Errors  scanner.ErrorList
}

// NewParsedGnoFile parses the Gno file with the standard parser, including
// comments.
func NewParsedGnoFile(path, content string) *ParsedGnoFile {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	var parseErr scanner.ErrorList
	if err != nil {
		parseErr = err.(scanner.ErrorList) //nolint:errcheck,errorlint
	}
	return &ParsedGnoFile{File: file, FileSet: fset, Errors: parseErr}
}

// ApplyChangesToAst applies the changes in the Document to the AST.
func (d *Document) ApplyChangesToAst(path, content string) {
	d.Pgf = NewParsedGnoFile(path, content)
}

func (d *Document) LookupSymbol(name string, offset int) *gno.Symbol {
	conf := types.Config{Importer: importer.Default(), Error: func(err error) { slog.Info(err.Error()) }}
	info := &types.Info{
		Defs:  make(map[*ast.Ident]types.Object),
		Uses:  make(map[*ast.Ident]types.Object),
		Types: make(map[ast.Expr]types.TypeAndValue),
	}

	pkg, _ := conf.Check(d.Path, d.Pgf.FileSet, []*ast.File{d.Pgf.File}, info)
	if pkg == nil || pkg.Scope() == nil {
		return nil
	}
	pos := d.Pgf.FileSet.File(d.Pgf.File.Pos()).Pos(offset)

	inner := pkg.Scope().Innermost(pos)
	if inner == nil {
		return nil
	}

	if obj := inner.Lookup(name); obj != nil {
		typeName := obj.Type().String()
		if !strings.Contains(typeName, "invalid") {
			// FIXME: `std` types are "invalid" since Go doesn't know about
			// them.
			return &gno.Symbol{
				Name:      obj.Name(),
				Signature: obj.String(),
				Kind:      obj.Type().String(),
			}
		}
	}

	return nil
}
