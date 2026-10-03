package jsonschema

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the committed JSON schemas of the current versions (make schemas)")

// CheckCommitted fails the test when the schema generated for a contract's
// current version differs from the committed file — the Go types changed
// without the SchemaVersion being raised. With -update it writes the file
// instead.
func CheckCommitted(t *testing.T, path string, doc Document) {
	t.Helper()
	got, err := Generate(doc)
	if err != nil {
		t.Fatalf("generate %s: %v", path, err)
	}
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nNew contract version? Run make schemas to write its schema.", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the Go types of %s no longer match the committed schema.\n"+
			"Raise the contract's SchemaVersion (ADR 0005: MINOR if old readers can ignore the change, "+
			"MAJOR otherwise), then run make schemas.\nGenerated:\n%s", path, got)
	}
}
