package jsonschema

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var (
	update    = flag.Bool("update", false, "rewrite the committed JSON schemas of the current versions (make schemas)")
	published = flag.String("published", "path", "git branch whose schemas count as published; -update never rewrites them")
)

// CheckCommitted fails the test when the schema generated for a contract's
// current version differs from the committed file — the Go types changed
// without the SchemaVersion being raised — or when an older, frozen schema
// of the contract changed.
//
// With -update it writes the file instead, unless that version is already
// published (on the -published branch) with other content, and freezes the
// older versions. While MAJOR is 0 a new MINOR may break old readers (ADR
// 0005); it then logs what would have required a MAJOR after 1.0.
func CheckCommitted(t *testing.T, path string, doc Document) {
	t.Helper()
	got, err := Generate(doc)
	if err != nil {
		t.Fatalf("generate %s: %v", path, err)
	}
	if *update {
		writeCommitted(t, path, got)
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
	if problems := checkFrozen(path); len(problems) > 0 {
		t.Fatal(strings.Join(problems, "\n"))
	}
}

func writeCommitted(t *testing.T, path string, got []byte) {
	t.Helper()
	pub, err := publishedSchema(*published, path)
	switch {
	case errors.Is(err, errUnchecked):
		t.Logf("WARNING: %s: not checked against branch %q: %v", path, *published, err)
	case err != nil:
		t.Fatal(err)
	case pub != nil && !bytes.Equal(pub, got):
		t.Fatalf("%s is already published on branch %q and the types no longer match it: "+
			"raise the contract's SchemaVersion (ADR 0005) instead of rewriting it", path, *published)
	}
	if problems := checkFrozen(path); len(problems) > 0 {
		for _, p := range problems {
			if !strings.Contains(p, "not frozen") {
				t.Fatal(strings.Join(problems, "\n"))
			}
		}
	}

	if major, _, _ := schemaVersionOf(filepath.Base(path)); major == 0 {
		problems, err := incompatibleWithPrevious(path, got)
		if err != nil {
			t.Fatal(err)
		}
		if len(problems) > 0 {
			t.Logf("NOTE: %s breaks readers of the previous MINOR. Fine while MAJOR is 0; after 1.0 it would need a MAJOR:\n%s",
				path, strings.Join(problems, "\n"))
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, got, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := freezeOlder(path); err != nil {
		t.Fatal(err)
	}
}
