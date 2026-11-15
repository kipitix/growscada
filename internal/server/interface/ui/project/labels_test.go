package project

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// TestSavedStateLabels_AssignAndUnassign covers the labels of a project's saved
// states in the history (task 85_3; ADR 0025): the menu of the Organization's
// labels, placing marks, taking them off, and the server's refusal of a label
// another tab deleted.
func TestSavedStateLabels_AssignAndUnassign(t *testing.T) {
	f := startProject(t)
	f.run()
	f.p.saveAuthor = "Ada"
	f.p.saveTitle = "First save"
	f.p.saveCommit(f.p.compoCtx)
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	ctx := context.Background()
	first, err := f.api.CreateLabel(ctx, "test-"+uuid.NewString()[:8], "#22c55e")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	second, err := f.api.CreateLabel(ctx, "test-"+uuid.NewString()[:8], "")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}

	f.p.openHistory(f.p.compoCtx)
	f.run()
	if len(f.p.historyLabels) < 2 {
		t.Fatalf("the history read %d labels, want the Organization's", len(f.p.historyLabels))
	}

	f.p.assignLabel(f.p.compoCtx, 1, first.ID)
	f.run()
	f.noToasts()
	if len(f.p.commits) != 1 || len(f.p.commits[0].Labels) != 1 || f.p.commits[0].Labels[0].ID != first.ID {
		t.Fatalf("the saved state's labels: %+v", f.p.commits)
	}
	html := app.HTMLString(f.p.Render())
	for _, want := range []string{`data-saved-state-labels="1"`, `data-label-id="` + first.ID.String() + `"`, first.Name} {
		if !strings.Contains(html, want) {
			t.Errorf("the history does not hold %q: %s", want, html)
		}
	}

	// A saved state may carry any number of labels.
	f.p.assignLabel(f.p.compoCtx, 1, second.ID)
	f.run()
	f.noToasts()
	if len(f.p.commits) != 1 || len(f.p.commits[0].Labels) != 2 {
		t.Errorf("the saved state's labels after a second mark: %+v", f.p.commits)
	}

	// Another tab takes the first mark off: its label_unassigned event reloads
	// this mode's history (task 85_3).
	if _, err := f.api.UnassignLabel(ctx, 1, first.ID); err != nil {
		t.Fatalf("another tab's unassignment: %v", err)
	}
	f.event(apiv0.Event{Type: apiv0.EventLabelUnassigned, ID: servertest.MainProject.String(), SavedState: 1, Label: first.ID.String()})
	if len(f.p.commits) != 1 || len(f.p.commits[0].Labels) != 1 || f.p.commits[0].Labels[0].ID != second.ID {
		t.Errorf("the history after another tab's unassignment: %+v", f.p.commits)
	}

	// The person takes the remaining mark off here.
	f.p.unassignLabel(f.p.compoCtx, 1, second.ID)
	f.run()
	f.noToasts()
	if len(f.p.commits) != 1 || len(f.p.commits[0].Labels) != 0 {
		t.Errorf("the saved state's labels after taking the mark off: %+v", f.p.commits)
	}
	if _, err := f.api.AssignLabel(ctx, 1, second.ID); err != nil {
		t.Fatalf("another tab's mark: %v", err)
	}
	f.p.loadCommits(f.p.compoCtx)
	f.run()

	// Another tab deletes the label: the server refuses the mark and the UI
	// shows its problem as a toast.
	if err := f.api.DeleteLabel(ctx, second.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	f.p.unassignLabel(f.p.compoCtx, 1, second.ID)
	f.run()
	if got := f.oneToast(); got.Status != http.StatusNotFound {
		t.Errorf("a mark of a deleted label: toast %+v, want a 404", got)
	}

	// "Manage labels" of an empty menu opens the tab's Labels window.
	f.p.manageLabels(f.p.compoCtx)
	f.run()
}
