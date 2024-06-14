package main

import (
	"encoding/gob"
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/jdkato/gnols/internal/stdlib"
)

var buildOutput = "internal/stdlib/stdlib"

func main() {
	hd, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	rootDir := flag.String(
		"root-dir", filepath.Join(hd, "gno"),
		"Root of the gno repository, to fetch examples and standard libraries.",
	)

	storageFormat := flag.String(
		"format", "gob",
		"Format to save the symbols in; 'gob' or 'json'.",
	)

	flag.Parse()

	dirs := [...]string{
		filepath.Join(*rootDir, "examples"),
		filepath.Join(*rootDir, "gnovm/stdlibs"),
	}

	var pkgs []stdlib.Package

	for _, dir := range dirs {
		for _, lib := range walkLib(dir) {
			symbols := []stdlib.Symbol{}
			for _, file := range walkPkg(lib) {
				symbols = append(symbols, getSymbols(file)...)
			}

			// convert to import path:
			// get path relative to dir, and convert separators to slashes.
			ip := strings.ReplaceAll(
				strings.TrimPrefix(lib, dir+string(filepath.Separator)),
				string(filepath.Separator), "/",
			)

			pkgs = append(pkgs, stdlib.Package{
				Name:       filepath.Base(lib),
				ImportPath: ip,
				Symbols:    symbols,
			})
		}
	}

	saveSymbols(pkgs, *storageFormat)
}

func walkLib(path string) []string {
	var libs []string

	err := filepath.WalkDir(path, func(lib string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		} else if d.IsDir() && lib != path {
			libs = append(libs, lib)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	return libs
}

func walkPkg(path string) []string {
	var files []string

	err := filepath.WalkDir(path, func(file string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if !strings.Contains(file, "_test") {
			ext := filepath.Ext(file)
			if ext != ".gno" {
				return nil
			} else if filepath.Dir(file) != path {
				return nil
			}
			files = append(files, file)
		}

		return nil
	})
	if err != nil {
		panic(err)
	}

	return files
}

func getSymbols(source string) []stdlib.Symbol {
	var symbols []stdlib.Symbol

	// Create a FileSet to work with.
	fset := token.NewFileSet()

	// Parse the file and create an AST.
	file, err := parser.ParseFile(fset, source, nil, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	bsrc, err := os.ReadFile(source)
	if err != nil {
		panic(err)
	}
	text := string(bsrc)

	// Trim AST to exported declarations only.
	ast.FileExports(file)

	ast.Inspect(file, func(n ast.Node) bool {
		var found []stdlib.Symbol

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

	return symbols
}

func saveSymbols(pkgs []stdlib.Package, format string) {
	switch format {
	case "gob":
		toGob(pkgs)
	case "json":
		toJSON(pkgs)
	}
}

func toJSON(pkgs []stdlib.Package) {
	found, err := json.MarshalIndent(pkgs, "", " ")
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(buildOutput+".json", found, os.ModePerm)
	if err != nil {
		panic(err)
	}
}

func toGob(pkgs []stdlib.Package) {
	dataFile, err := os.Create(buildOutput + ".gob")
	if err != nil {
		panic(err)
	}
	dataEncoder := gob.NewEncoder(dataFile)

	err = dataEncoder.Encode(pkgs)
	if err != nil {
		panic(err)
	}

	dataFile.Close()
}

func declaration(n *ast.GenDecl, source string) []stdlib.Symbol {
	for _, spec := range n.Specs {
		switch t := spec.(type) { //nolint:gocritic
		case *ast.TypeSpec:
			return []stdlib.Symbol{{
				Name:      t.Name.Name,
				Doc:       n.Doc.Text(),
				Signature: strings.Split(source[t.Pos()-1:t.End()-1], " {")[0],
				Kind:      typeName(*t),
			}}
		}
	}

	return nil
}

func function(n *ast.FuncDecl, source string) []stdlib.Symbol {
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
	return []stdlib.Symbol{{
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
