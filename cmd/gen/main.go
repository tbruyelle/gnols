package main

import (
	"encoding/gob"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"

	"github.com/jdkato/gnols/internal/gno"
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

	var pkgs []gno.Package
	for _, dir := range dirs {
		dirPkgs, err := gno.ParsePackages("", dir)
		if err != nil {
			panic(err)
		}
		pkgs = append(pkgs, dirPkgs...)
	}

	saveSymbols(pkgs, *storageFormat)
}

func saveSymbols(pkgs []gno.Package, format string) {
	switch format {
	case "gob":
		toGob(pkgs)
	case "json":
		toJSON(pkgs)
	}
}

func toJSON(pkgs []gno.Package) {
	found, err := json.MarshalIndent(pkgs, "", " ")
	if err != nil {
		panic(err)
	}

	err = os.WriteFile(buildOutput+".json", found, os.ModePerm)
	if err != nil {
		panic(err)
	}
}

func toGob(pkgs []gno.Package) {
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
