package handler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdkato/gnols/internal/store"
)

const testDir = "../../testdata/code_lens"

func TestCodeLensFind(t *testing.T) {
	filePath, err := filepath.Abs(filepath.Join(testDir, "t1_test.go"))
	if err != nil {
		t.Fatal(err)
	}

	dat, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	pgf := store.NewParsedGnoFile(filePath, string(dat))

	doc := &store.Document{
		Path: filePath,
		Pgf:  pgf,
	}
	found := testsAndBenchmarks(doc)

	if len(found.Tests) != 1 {
		t.Errorf("expected = %v, got = %v", 1, len(found.Tests))
	}

	if len(found.Benchmarks) != 2 {
		t.Errorf("expected = %v, got = %v", 2, len(found.Benchmarks))
	}
}
