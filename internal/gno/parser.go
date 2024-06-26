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
}

// ParsePackages parses gno files in rootDir and sub-directories, and returns
// Packages with all the symbols.
// If wd is provided and is different from the file being parsed, then only
// public symbols are returned.
func ParsePackages(wd, rootDir string) ([]Package, error) {
	dirs, err := getDirs(rootDir)
	if err != nil {
		return nil, err
	}
	var pkgs []Package
	for _, dir := range dirs {
		files, err := getFiles(dir)
		if err != nil {
			return nil, err
		}
		// convert to import path:
		// get path relative to dir, and convert separators to slashes.
		rel, err := filepath.Rel(rootDir, dir)
		if err != nil {
			return nil, err
		}
		ip := strings.ReplaceAll(rel, string(filepath.Separator), "/")
		pkg := Package{
			Name:       filepath.Base(dir),
			ImportPath: ip,
		}
		for _, file := range files {
			symbols, err := getSymbols(wd, file)
			if err != nil {
				return nil, err
			}
			pkg.Symbols = append(pkg.Symbols, symbols...)
		}
		if len(pkg.Symbols) > 0 {
			pkgs = append(pkgs, pkg)
		}
	}

	return pkgs, nil
}

// getDirs returns all directories inside path, ignoring hidden directories.
func getDirs(path string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(path, func(dir string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && filepath.Base(dir)[0] != '.' { // avoid hidden dir
			dirs = append(dirs, dir)
		}
		return nil
	})
	return dirs, err
}

// getFiles returns all the gno non-tests files inside path.
//
// TODO parse test files inside wd so completion can work with symbols created
// in test files.
func getFiles(path string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(path, func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Dir(file) != path {
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

// getSymbols returns all symbols found in filename. If wd is different than
// filname path, only public symbols are returned.
func getSymbols(wd string, filename string) ([]Symbol, error) {
	var symbols []Symbol

	// Create a FileSet to work with.
	fset := token.NewFileSet()

	// Parse the file and create an AST.
	file, _ := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	// fmt.Println("PARSEFILE", err)

	bsrc, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	text := string(bsrc)

	isCurrentDir := wd == filepath.Dir(filename)
	if !isCurrentDir {
		// Trim AST to exported declarations only if not in working dir
		ast.FileExports(file)
	}

	var (
		nestedCount int
		// A node is exported if ast.IsExported() returns true AND if its
		// declaration is at top level. For this we rely on the nestedCount
		// variable, which is incremented when ast.Inspect goes down inside the
		// nodes, and decremeted when it goes up.
		// This is required for filter out the variables and types that have an
		// uppercase in the name but are declared inside a block. In that case they
		// are not visible outside of the package.
		isExported = func(name string) bool {
			return ast.IsExported(name) && nestedCount < 4
		}
	)
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			nestedCount--
		} else {
			nestedCount++
		}
		var found *Symbol

		// fmt.Println("NODE", filename, nestedCount, spew.Sdump(n))
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
			if isCurrentDir || isExported(found.Name) {
				// append only if current directory or node is exported
				symbols = append(symbols, *found)
			}
		}

		return true
	})

	return symbols, nil
}

func declaration(n *ast.GenDecl, source string) *Symbol {
	for _, spec := range n.Specs {
		switch t := spec.(type) { //nolint:gocritic
		case *ast.TypeSpec:
			typ, fields := typeFromExpr(t.Type, source)
			return &Symbol{
				Name:      t.Name.Name,
				Doc:       strings.TrimSpace(n.Doc.Text()),
				Signature: strings.Split(source[t.Pos()-1:t.End()-1], " {")[0],
				Kind:      typeName(*t),
				Type:      typ,
				Fields:    fields,
			}
		}
	}

	return nil
}

func function(n *ast.FuncDecl, source string) *Symbol {
	var recv string
	if n.Recv != nil {
		recv, _ = typeFromExpr(n.Recv.List[0].Type, source)
		if !ast.IsExported(recv) {
			return nil
		}
	}
	return &Symbol{
		Name:      n.Name.Name,
		Doc:       n.Doc.Text(),
		Signature: strings.Split(source[n.Pos()-1:n.End()-1], " {")[0],
		Kind:      "func",
		Recv:      recv,
	}
}

func assignment(n *ast.AssignStmt, source string) *Symbol {
	typ, fields := typeFromExpr(n.Rhs[0], source)
	return &Symbol{
		Name:      n.Lhs[0].(*ast.Ident).Name,
		Signature: source[n.Pos()-1 : n.End()-1],
		Kind:      "var",
		Type:      typ,
		Fields:    fields,
	}
}

func variable(n *ast.ValueSpec, source string) *Symbol {
	typ, fields := typeFromExpr(n.Type, source)
	return &Symbol{
		Name:      n.Names[0].Name,
		Doc:       strings.TrimSpace(n.Doc.Text()),
		Signature: source[n.Pos()-1 : n.End()-1],
		Kind:      "var",
		Type:      typ,
		Fields:    fields,
	}
}

func fieldsFromStruct(st *ast.StructType, source string) (fields []Symbol) {
	for _, f := range st.Fields.List {
		typ, subfields := typeFromExpr(f.Type, source)
		var name string
		if len(f.Names) > 0 {
			name = f.Names[0].Name
		} else {
			// f is an embedded struct, use type name as the name.
			name = typ
		}
		fields = append(fields, Symbol{
			Name:      name,
			Doc:       strings.TrimSpace(f.Doc.Text()),
			Signature: source[f.Pos()-1 : f.End()-1],
			Kind:      "field",
			Type:      typ,
			Fields:    subfields,
		})
	}
	return
}

func typeFromExpr(x ast.Expr, source string) (string, []Symbol) {
	switch x := x.(type) {
	case *ast.Ident:
		return x.Name, nil
	case *ast.CompositeLit:
		return typeFromExpr(x.Type, source)
	case *ast.UnaryExpr:
		return typeFromExpr(x.X, source)
	case *ast.StructType: // inline struct
		return "", fieldsFromStruct(x, source)
	}
	return "", nil
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
