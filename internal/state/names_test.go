package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadNames_MissingIsEmpty(t *testing.T) {
	if names := ReadNames(t.TempDir()); len(names) != 0 {
		t.Errorf("missing overlay should be empty, got %v", names)
	}
}

func TestReadNames_UnparseableIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, namesFile), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if names := ReadNames(dir); len(names) != 0 {
		t.Errorf("unparseable overlay should be empty, got %v", names)
	}
}

func TestWriteName_RoundTripsAndPreservesOthers(t *testing.T) {
	dir := t.TempDir()
	if err := WriteName(dir, "id-1", "first"); err != nil {
		t.Fatal(err)
	}
	if err := WriteName(dir, "id-2", "second"); err != nil {
		t.Fatal(err)
	}
	// Overwriting one entry keeps the other.
	if err := WriteName(dir, "id-1", "renamed"); err != nil {
		t.Fatal(err)
	}

	names := ReadNames(dir)
	if names["id-1"] != "renamed" || names["id-2"] != "second" {
		t.Errorf("overlay round trip failed: %v", names)
	}
}

func TestWriteName_MarshalError(t *testing.T) {
	original := marshal
	marshal = func(any) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { marshal = original }()

	if err := WriteName(t.TempDir(), "id", "name"); err == nil {
		t.Fatal("expected a marshal error")
	}
}

func TestWriteName_MkdirError(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteName(filepath.Join(file, "sub"), "id", "name"); err == nil {
		t.Fatal("expected an error when the parent path is a file")
	}
}

func TestWriteName_WriteFileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, namesFile+".tmp"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteName(dir, "id", "name"); err == nil {
		t.Fatal("expected a WriteFile error")
	}
}

func TestWriteName_RenameError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, namesFile)
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "inner"), []byte("y"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteName(dir, "id", "name"); err == nil {
		t.Fatal("expected a Rename error")
	}
}
