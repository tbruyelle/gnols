package gno

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type Package struct {
	Name       string
	ImportPath string
	Symbols    []Symbol
}

type Symbol struct {
	Name      string
	Doc       string
	Signature string
	Kind      string
	Recv      string
}

func (s Symbol) String() string {
	return fmt.Sprintf("```go\n%s\n```\n\n%s", s.Signature, s.Doc)
}

func ParsePackages(dir string) ([]Package, error) {
	var pkgs []Package
	dirs, err := getDirs(dir)
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		symbols := []Symbol{}
		files, err := getFiles(d)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			syms, err := getSymbols(file)
			if err != nil {
				return nil, err
			}
			symbols = append(symbols, syms...)
		}
		if len(symbols) == 0 {
			// Ignore directories w/o symbols
			continue
		}

		// convert to import path:
		// get path relative to dir, and convert separators to slashes.
		ip := strings.ReplaceAll(
			strings.TrimPrefix(d, dir+string(filepath.Separator)),
			string(filepath.Separator), "/",
		)

		pkgs = append(pkgs, Package{
			Name:       filepath.Base(d),
			ImportPath: ip,
			Symbols:    symbols,
		})
	}
	return pkgs, nil
}

func getDirs(path string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(path, func(dir string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			dirs = append(dirs, dir)
		}
		return nil
	})
	return dirs, err
}

func getFiles(path string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(path, func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.Contains(file, "_test") {
			return nil
		}
		ext := filepath.Ext(file)
		if ext != ".gno" {
			return nil
		}
		if filepath.Dir(file) != path {
			return nil
		}
		files = append(files, file)
		return nil
	})
	return files, err
}

func getSymbols(filename string) ([]Symbol, error) {
	var symbols []Symbol

	// Create a FileSet to work with.
	fset := token.NewFileSet()

	// Parse the file and create an AST.
	file, _ := parser.ParseFile(fset, filename, nil, parser.ParseComments)

	bsrc, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	text := string(bsrc)

	// Trim AST to exported declarations only.
	ast.FileExports(file)

	ast.Inspect(file, func(n ast.Node) bool {
		var found []Symbol

		switch n := n.(type) {
		case *ast.FuncDecl:
			found = function(n, text)
		case *ast.GenDecl:
			found = declaration(n, text)
		}

		if found != nil {
			symbols = append(symbols, found...)
		}

		return true
	})

	return symbols, nil
}

func declaration(n *ast.GenDecl, source string) []Symbol {
	for _, spec := range n.Specs {
		switch t := spec.(type) { //nolint:gocritic
		case *ast.TypeSpec:
			return []Symbol{{
				Name:      t.Name.Name,
				Doc:       n.Doc.Text(),
				Signature: strings.Split(source[t.Pos()-1:t.End()-1], " {")[0],
				Kind:      typeName(*t),
			}}
		}
	}

	return nil
}

func function(n *ast.FuncDecl, source string) []Symbol {
	var recv string
	if n.Recv != nil {
		switch x := n.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			recv = x.X.(*ast.Ident).Name
		case *ast.Ident:
			recv = x.Name
		}
		if !ast.IsExported(recv) {
			return nil
		}
	}
	return []Symbol{{
		Name:      n.Name.Name,
		Doc:       n.Doc.Text(),
		Signature: strings.Split(source[n.Pos()-1:n.End()-1], " {")[0],
		Kind:      "func",
		Recv:      recv,
	}}
}

func typeName(t ast.TypeSpec) string {
	switch t.Type.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	case *ast.ArrayType:
		return "array"
	case *ast.MapType:
		return "map"
	case *ast.ChanType:
		return "chan"
	default:
		return "type"
	}
}
