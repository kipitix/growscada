package growctl_test

import (
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/server/servertest"
)

// mainPins declares the pin of release n of Gauges in the project Main.
func mainPins(n string) string {
	return "---\napiVersion: growscada/v0.4\nkind: Project\nmetadata:\n  name: Main\nspec:\n  libraryPins:\n    Gauges: " + n + "\n"
}

// TestLibrary_ApplyUpdatesTwoWidgetTypesInOneRun: one Version covers every
// write of a library's Draft (ADR 0016), so the second WidgetType a run
// updates states the Version the first response answered, not the one the
// plan read.
func TestLibrary_ApplyUpdatesTwoWidgetTypesInOneRun(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	const two = `apiVersion: growscada/v0.4
kind: Library
metadata:
  name: Gauges
---
apiVersion: growscada/v0.4
kind: WidgetType
metadata:
  name: Lamp
  library: Gauges
spec:
  html: <div/>
  script: ""
  width: 40
  height: 40
---
apiVersion: growscada/v0.4
kind: WidgetType
metadata:
  name: Dial
  library: Gauges
spec:
  html: <div/>
  script: ""
  width: 30
  height: 30
`
	if _, err := runGrowctl(t, srv, two, "apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	edited := strings.Replace(two, "width: 40", "width: 50", 1)
	edited = strings.Replace(edited, "width: 30", "width: 35", 1)
	if out, err := runGrowctl(t, srv, edited, "apply"); err != nil {
		t.Fatalf("apply two edits: %q, %v", out, err)
	}
}

// TestLibrary_ApplyReleasePinDiscardDelete walks a library through growctl:
// apply creates it with its Draft, save fixes Commit 1, release publishes it
// as release 1, apply pins it in Main, an edited WidgetType is updated, saved
// and released again, the pin switched to release 2, prune removes a
// WidgetType left out, discard resets the Draft, delete removes the
// WidgetType and, once unpinned, the library.
func TestLibrary_ApplyReleasePinDiscardDelete(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))

	out, err := runGrowctl(t, srv, libraryManifest, "apply")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertOutput(t, out, "library/Gauges created", "widgettype/Gauges/Lamp created")

	out, _, err = execGrowctl(t, srv, "", "save", "Gauges", "--title", "First", "--author", "tester")
	if err != nil || out != "library/Gauges saved commit 1\n" {
		t.Fatalf("save: %q, %v", out, err)
	}
	out, _, err = execGrowctl(t, srv, "", "release", "Gauges", "--commit", "1", "--author", "tester")
	if err != nil || out != "library/Gauges released 1\n" {
		t.Fatalf("release 1: %q, %v", out, err)
	}
	if _, stderr, err := execGrowctl(t, srv, "", "release", "Gauges", "--commit", "1", "--author", "tester"); err == nil {
		t.Errorf("a release of an already published Commit must fail, stderr %q", stderr)
	}

	out, err = runGrowctl(t, srv, libraryManifest+mainPins("1"), "apply")
	if err != nil {
		t.Fatalf("apply the pin: %v", err)
	}
	assertOutput(t, out, "project/Main unchanged", "library/Gauges unchanged", "widgettype/Gauges/Lamp unchanged",
		"project/Main pin library/Gauges release 1 added")

	edited := strings.Replace(libraryManifest, "width: 40", "width: 60", 1)
	out, _, err = execGrowctl(t, srv, edited+mainPins("1"), "diff", "-f", "-")
	if err == nil || out != "~ widgettype/Gauges/Lamp\n" {
		t.Errorf("diff: %q, %v; want the change and exit status 1", out, err)
	}
	if out, err = runGrowctl(t, srv, edited, "apply"); err != nil || !strings.Contains(out, "widgettype/Gauges/Lamp updated") {
		t.Fatalf("apply the edit: %q, %v", out, err)
	}
	if out, _, err = execGrowctl(t, srv, "", "save", "Gauges", "--title", "Wider", "--author", "tester"); err != nil ||
		out != "library/Gauges saved commit 2\n" {
		t.Fatalf("save 2: %q, %v", out, err)
	}
	out, _, err = execGrowctl(t, srv, "", "release", "Gauges", "--commit", "2", "--note", "wider", "--author", "tester")
	if err != nil || out != "library/Gauges released 2\n" {
		t.Fatalf("release 2: %q, %v", out, err)
	}
	out, _, err = execGrowctl(t, srv, "", "commits", "Gauges", "--library")
	if err != nil || !strings.Contains(out, "Wider") || !strings.Contains(out, "true") {
		t.Fatalf("commits: %q, %v", out, err)
	}
	out, _, err = execGrowctl(t, srv, "", "releases", "Gauges", "--library")
	if err != nil || !strings.Contains(out, "wider") || !strings.Contains(out, "commit") {
		t.Fatalf("releases: %q, %v", out, err)
	}
	if out, err = runGrowctl(t, srv, edited+mainPins("2"), "apply"); err != nil || !strings.Contains(out, "project/Main pin library/Gauges release 2 switched") {
		t.Fatalf("switch the pin: %q, %v", out, err)
	}

	withExtra := edited + "---\napiVersion: growscada/v0.4\nkind: WidgetType\nmetadata: {name: Extra, library: Gauges}\nspec: {html: '', script: '', width: 1, height: 1}\n"
	if _, err = runGrowctl(t, srv, withExtra, "apply"); err != nil {
		t.Fatalf("apply Extra: %v", err)
	}
	if out, err = runGrowctl(t, srv, edited, "apply", "--prune"); err != nil || !strings.Contains(out, "widgettype/Gauges/Extra pruned") {
		t.Fatalf("apply --prune: %q, %v", out, err)
	}

	if _, err = runGrowctl(t, srv, withExtra, "apply"); err != nil {
		t.Fatalf("apply Extra again: %v", err)
	}
	out, _, err = execGrowctl(t, srv, "", "discard", "Gauges")
	if err != nil || !strings.HasPrefix(out, "library/Gauges discarded: version ") {
		t.Fatalf("discard: %q, %v", out, err)
	}

	out, _, err = execGrowctl(t, srv, "", "get", "libraries")
	if err != nil || !strings.Contains(out, "Gauges") {
		t.Errorf("get libraries: %q, %v", out, err)
	}

	// The library is pinned: delete removes its WidgetType, the library
	// stays until no project pins it.
	out, _, err = execGrowctl(t, srv, "y\n", "delete", "-f", writeManifest(t, libraryManifest))
	if err == nil || !strings.Contains(out, "widgettype/Gauges/Lamp") {
		t.Errorf("delete of a pinned library: %q, %v; want the WidgetType done and the library refused", out, err)
	}
}
