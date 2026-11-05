package growctl_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// mainPins declares the pin of release n of Gauges in the project Main.
func mainPins(n string) string {
	return "---\napiVersion: growscada/v0.4\nkind: Project\nmetadata:\n  name: Main\nspec:\n  libraryPins:\n    Gauges: " + n + "\n"
}

// TestLibrary_HeldLock_IsAnErrorNamingTheHolder: a run that writes a library's
// Draft does not write it for another client; --take-over takes it
// (ADR 0015).
func TestLibrary_HeldLock_IsAnErrorNamingTheHolder(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	if _, err := runGrowctl(t, srv, libraryManifest, "apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	found, ok, err := apiclient.New(srv.URL, srv.Client()).FindLibraryByName(context.Background(), "Gauges")
	if err != nil || !ok {
		t.Fatalf("find library Gauges: %t, %v", ok, err)
	}
	servertest.EnsureLibraryEditLock(t, srv, found.ID)

	edited := strings.Replace(libraryManifest, "width: 40", "width: 60", 1)
	if _, err := runGrowctl(t, srv, edited, "apply"); err == nil {
		t.Fatal("apply of a held Draft succeeded")
	} else if !strings.Contains(err.Error(), servertest.ClientName) || !strings.Contains(err.Error(), "--take-over") {
		t.Errorf("the refusal does not name the holder and the way out: %v", err)
	}
	if _, err := runGrowctl(t, srv, edited, "apply", "--take-over"); err != nil {
		t.Errorf("apply --take-over: %v", err)
	}
}

// TestLibraryRelease_HeldLock_IsAnErrorNamingTheHolder: a Release is an act
// over the whole Draft and needs its EditLock (ADR 0015).
func TestLibraryRelease_HeldLock_IsAnErrorNamingTheHolder(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	if _, err := runGrowctl(t, srv, libraryManifest, "apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	found, ok, err := apiclient.New(srv.URL, srv.Client()).FindLibraryByName(context.Background(), "Gauges")
	if err != nil || !ok {
		t.Fatalf("find library Gauges: %t, %v", ok, err)
	}
	servertest.EnsureLibraryEditLock(t, srv, found.ID)

	if _, _, err := execGrowctl(t, srv, "", "release", "Gauges"); err == nil ||
		!strings.Contains(err.Error(), servertest.ClientName) || !strings.Contains(err.Error(), "--take-over") {
		t.Errorf("release of a held Draft: %v, want the holder named", err)
	}
	if _, _, err := execGrowctl(t, srv, "", "release", "Gauges", "--take-over"); err != nil {
		t.Errorf("release --take-over: %v", err)
	}
}

// TestLibrary_ApplyReleasePinDiscardDelete walks a library through growctl:
// apply creates it with its Draft, release makes release 1, apply pins it in
// Main, an edited WidgetType is updated and released again, the pin switched
// to release 2, prune removes a WidgetType left out, discard resets the
// Draft, delete removes the WidgetType and, once unpinned, the library.
func TestLibrary_ApplyReleasePinDiscardDelete(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))

	out, err := runGrowctl(t, srv, libraryManifest, "apply")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertOutput(t, out, "library/Gauges created", "widgettype/Gauges/Lamp created")

	if _, _, err := execGrowctl(t, srv, "", "release", "Gauges"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, stderr, err := execGrowctl(t, srv, "", "release", "Gauges"); err == nil {
		t.Errorf("a release of an unchanged Draft must fail, stderr %q", stderr)
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
	out, _, err = execGrowctl(t, srv, "", "release", "Gauges", "--note", "wider")
	if err != nil || out != "library/Gauges released 2\n" {
		t.Fatalf("release 2: %q, %v", out, err)
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
