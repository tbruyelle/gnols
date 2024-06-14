package handler

import (
	"testing"

	"github.com/jdkato/gnols/internal/stdlib"
)

func TestLookupPkg(t *testing.T) {
	pkg := lookupPkg(stdlib.Packages, "fmt")
	if pkg != nil {
		t.Errorf("Expected nil, got %v", pkg)
	}

	pkg = lookupPkg(stdlib.Packages, "ufmt")
	if pkg == nil {
		t.Errorf("Expected non-nil, got %v", pkg)
	}

	if pkg.ImportPath != "gno.land/p/demo/ufmt" {
		t.Errorf("Expected gno.land/p/demo/ufmt, got %v", pkg.ImportPath)
	}

	if len(pkg.Symbols) < 1 {
		t.Errorf("Expected symbols, got %v", len(pkg.Symbols))
	}
}
