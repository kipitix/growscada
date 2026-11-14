package growctl_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// TestLabels_CRUD drives the minimal label commands against the real API:
// list, create with a color, rename, recolor (and clear the color), the
// refusal of a taken name, and delete (task 85_2; ADR 0025).
func TestLabels_CRUD(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	name := "e2e-" + uuid.NewString()[:8]

	out, _, err := execGrowctlBare(t, srv, "", "labels", "create", name, "--color", "#22c55e")
	if err != nil {
		t.Fatalf("labels create: %v", err)
	}
	assertOutput(t, out, "label/"+name+" created")

	out, _, err = execGrowctlBare(t, srv, "", "labels")
	if err != nil {
		t.Fatalf("labels: %v", err)
	}
	if !strings.Contains(out, name) || !strings.Contains(out, "#22c55e") {
		t.Errorf("the list misses the created label: %q", out)
	}

	if _, _, err := execGrowctlBare(t, srv, "", "labels", "create", strings.ToUpper(name)); err == nil {
		t.Errorf("a taken name was created")
	}

	out, _, err = execGrowctlBare(t, srv, "", "labels", "rename", name, name+"-renamed")
	if err != nil {
		t.Fatalf("labels rename: %v", err)
	}
	assertOutput(t, out, "label/"+name+" renamed to "+name+"-renamed")

	out, _, err = execGrowctlBare(t, srv, "", "labels", "color", name+"-renamed", "none")
	if err != nil {
		t.Fatalf("labels color none: %v", err)
	}
	assertOutput(t, out, "label/"+name+"-renamed colored -")

	out, _, err = execGrowctlBare(t, srv, "", "labels", "delete", name+"-renamed")
	if err != nil {
		t.Fatalf("labels delete: %v", err)
	}
	assertOutput(t, out, "label/"+name+"-renamed deleted")

	out, _, err = execGrowctlBare(t, srv, "", "labels")
	if err != nil {
		t.Fatalf("labels after the delete: %v", err)
	}
	if strings.Contains(out, name) {
		t.Errorf("the deleted label is still listed: %q", out)
	}
}

// TestLabelAssign_ProjectAndLibrary drives the assignment commands against the
// real API (task 85_3; ADR 0025): a mark placed by name, shown by `commits`,
// and taken off again, for a Project's and a Library's saved state.
func TestLabelAssign_ProjectAndLibrary(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	if _, err := runGrowctl(t, srv, twoTags, "apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, _, err := execGrowctl(t, srv, "", "save", "Main", "--title", "Before the demo", "--author", "Ada"); err != nil {
		t.Fatalf("save: %v", err)
	}
	name := "e2e-" + uuid.NewString()[:8]
	if out, _, err := execGrowctlBare(t, srv, "", "labels", "create", name, "--color", "#22c55e"); err != nil {
		t.Fatalf("labels create: %v\n%s", err, out)
	}

	// A name is enough to name the label, whatever its case.
	out, _, err := execGrowctl(t, srv, "", "label", "assign", "Main", "--commit", "1", "--label", strings.ToUpper(name))
	if err != nil {
		t.Fatalf("label assign: %v", err)
	}
	assertOutput(t, out, "project/Main saved state 1: assigned "+name, "labels: "+name+" (#22c55e)")
	out, _, err = execGrowctl(t, srv, "", "commits", "Main")
	if err != nil || !strings.Contains(out, name) || !strings.Contains(out, "#22c55e") {
		t.Fatalf("commits after the assignment: %q, %v", out, err)
	}

	out, _, err = execGrowctl(t, srv, "", "label", "unassign", "Main", "--commit", "1", "--label", name)
	if err != nil {
		t.Fatalf("label unassign: %v", err)
	}
	assertOutput(t, out, "project/Main saved state 1: unassigned "+name)
	if out, _, err := execGrowctl(t, srv, "", "commits", "Main"); err != nil || strings.Contains(out, name) {
		t.Fatalf("commits after the unassignment: %q, %v", out, err)
	}
	// The commands refuse a missing saved state and a missing label.
	if _, _, err := execGrowctl(t, srv, "", "label", "assign", "Main", "--commit", "9", "--label", name); err == nil {
		t.Errorf("a mark of a missing saved state succeeded")
	}
	if _, _, err := execGrowctl(t, srv, "", "label", "assign", "Main", "--commit", "1", "--label", "no-such-label"); err == nil {
		t.Errorf("a mark of a missing label succeeded")
	}

	// The same under a Library.
	ctx := context.Background()
	api := apiclient.New(srv.URL, srv.Client())
	libraryName := "LabelGauges-" + uuid.NewString()[:8]
	library, err := api.CreateLibrary(ctx, apiv0.CreateLibraryRequest{Name: libraryName})
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}
	libraryClient := api.Library(library.ID)
	if _, err := libraryClient.CreateWidgetType(ctx, apiv0.CreateWidgetTypeRequest{
		Name: "gauge", HtmlTemplate: "<div></div>", Script: "function render(inputs) {}",
		ScriptLanguage: "javascript", DefaultWidth: 100, DefaultHeight: 100,
	}); err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	state, err := libraryClient.DraftState(ctx)
	if err != nil {
		t.Fatalf("DraftState: %v", err)
	}
	if _, err := libraryClient.SaveCommit(ctx, apiv0.CreateCommitRequest{
		Version: state.Version, Title: "First", Author: "Ada",
	}); err != nil {
		t.Fatalf("SaveCommit: %v", err)
	}

	out, _, err = execGrowctl(t, srv, "", "label", "assign", libraryName, "--commit", "1", "--label", name, "--library")
	if err != nil {
		t.Fatalf("library label assign: %v", err)
	}
	assertOutput(t, out, "library/"+libraryName+" saved state 1: assigned "+name, "labels: "+name+" (#22c55e)")
	out, _, err = execGrowctl(t, srv, "", "commits", libraryName, "--library")
	if err != nil || !strings.Contains(out, name) {
		t.Fatalf("library commits after the assignment: %q, %v", out, err)
	}
	out, _, err = execGrowctl(t, srv, "", "label", "unassign", libraryName, "--commit", "1", "--label", name, "--library")
	if err != nil {
		t.Fatalf("library label unassign: %v", err)
	}
	assertOutput(t, out, "library/"+libraryName+" saved state 1: unassigned "+name)
}
