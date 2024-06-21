package gno

import (
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
	Doc       string `json:",omitempty"`
	Signature string `json:",omitempty"`
	Kind      string
	Recv      string   `json:",omitempty"`
	Fields    []Symbol `json:",omitempty"`
	Type      string   `json:",omitempty"`
	PkgPath   string   `json:",omitempty"`
}

// func (s Symbol) String() string {
// return fmt.Sprintf("```go\n%s\n```\n\n%s", s.Signature, s.Doc)
// }

func ParsePackage(dir, importPath string) (*Package, error) {
	symbols := []Symbol{}
	files, err := getFiles(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		syms, err := getSymbols(dir, file)
		if err != nil {
			return nil, err
		}
		symbols = append(symbols, syms...)
	}
	if len(symbols) == 0 {
		// Ignore directories w/o symbols
		return nil, nil
	}

	return &Package{
		Name:       filepath.Base(dir),
		ImportPath: importPath,
		Symbols:    symbols,
	}, nil
}

func ParsePackages(dir string) ([]Package, error) {
	var pkgs []Package
	dirs, err := getDirs(dir)
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		// convert to import path:
		// get path relative to dir, and convert separators to slashes.
		ip := strings.ReplaceAll(
			strings.TrimPrefix(d, dir+string(filepath.Separator)),
			string(filepath.Separator), "/",
		)
		pkg, err := ParsePackage(d, ip)
		if err != nil {
			return nil, err
		}
		if pkg != nil {
			pkgs = append(pkgs, *pkg)
		}
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
		files = append(files, file)
		return nil
	})
	return files, err
}

func getSymbols(dir string, filename string) ([]Symbol, error) {
	var symbols []Symbol

	// Create a FileSet to work with.
	fset := token.NewFileSet()

	// Parse the file and create an AST.
	file, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		// fmt.Println("PARSEFILE", err)
	}

	bsrc, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	text := string(bsrc)

	// Trim AST to exported declarations only.
	ast.FileExports(file)

	ast.Inspect(file, func(n ast.Node) bool {
		var found []Symbol

		// fmt.Println("NODE", filename, spew.Sdump(n))
		switch n := n.(type) {
		case *ast.FuncDecl:
			found = function(n, text)
		case *ast.GenDecl:
			found = declaration(n, text)
		case *ast.AssignStmt:
			if n.Tok == token.DEFINE {
				// new variable declaration with :=
				found = assignment(n, text)
			}
		case *ast.ValueSpec:
			found = variable(n, text)
		}

		if found != nil {
			rel, err := filepath.Rel(dir, filename)
			if err == nil {
				pkgPath := filepath.Dir(rel)
				for i := range found {
					found[i].PkgPath = pkgPath
				}
			}
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
			var fields []Symbol
			if st, ok := t.Type.(*ast.StructType); ok {
				// Register struct's fields.
				for _, f := range st.Fields.List {
					var typ string
					if id, ok := f.Type.(*ast.Ident); ok {
						typ = id.Name
					}
					fields = append(fields, Symbol{
						Name:      f.Names[0].Name,
						Doc:       strings.TrimSpace(f.Doc.Text()),
						Signature: source[f.Pos()-1 : f.End()-1],
						Kind:      "field",
						Type:      typ,
					})
				}
			}
			return []Symbol{{
				Name:      t.Name.Name,
				Doc:       strings.TrimSpace(n.Doc.Text()),
				Signature: strings.Split(source[t.Pos()-1:t.End()-1], " {")[0],
				Kind:      typeName(*t),
				Fields:    fields,
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

func assignment(n *ast.AssignStmt, source string) []Symbol {
	var typ string
	if cl, ok := n.Rhs[0].(*ast.CompositeLit); ok {
		if id, ok := cl.Type.(*ast.Ident); ok {
			typ = id.Name
		}
	}
	return []Symbol{{
		Name:      n.Lhs[0].(*ast.Ident).Name,
		Signature: source[n.Pos()-1 : n.End()-1],
		Kind:      "var",
		Type:      typ,
	}}
}

func variable(n *ast.ValueSpec, source string) []Symbol {
	var typ string
	if id, ok := n.Type.(*ast.Ident); ok {
		typ = id.Name
	}
	return []Symbol{{
		Name:      n.Names[0].Name,
		Signature: source[n.Pos()-1 : n.End()-1],
		Kind:      "var",
		Type:      typ,
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
