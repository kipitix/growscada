package library

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// TestSavedStateLabels_AssignAndUnassign covers the labels of a library's saved
// states in the history (task 85_3; ADR 0025): the labels are read when History
// opens, a mark is placed and taken off, and the server's refusal is a toast.
func TestSavedStateLabels_AssignAndUnassign(t *testing.T) {
	url, api := newServer(t)
	createWidgetType(t, api, "Gauge")
	f := mount(t, url, api)
	f.run()
	f.l.saveAuthor = "Ada"
	f.l.saveTitle = "Widgets before the demo"
	f.l.saveCommit()
	f.run()
	f.e.TakeToasts() // the Save's own confirmation

	ctx := context.Background()
	label, err := f.api.CreateLabel(ctx, "test-"+uuid.NewString()[:8], "#3b82f6")
	if err != nil {
		t.Fatalf("CreateLabel: %v", err)
	}
	f.l.openHistory()
	f.run()
	if len(f.l.historyLabels) == 0 {
		t.Fatalf("the history read no labels")
	}

	f.l.assignLabel(f.l.compoCtx, 1, label.ID)
	f.run()
	f.noToasts()
	if len(f.l.commits) != 1 || len(f.l.commits[0].Labels) != 1 || f.l.commits[0].Labels[0].ID != label.ID {
		t.Fatalf("the saved state's labels: %+v", f.l.commits)
	}
	html := app.HTMLString(f.l.Render())
	for _, want := range []string{`data-saved-state-labels="1"`, `data-label-id="` + label.ID.String() + `"`, label.Name} {
		if !strings.Contains(html, want) {
			t.Errorf("the history does not hold %q: %s", want, html)
		}
	}

	f.l.unassignLabel(f.l.compoCtx, 1, label.ID)
	f.run()
	f.noToasts()
	if len(f.l.commits) != 1 || len(f.l.commits[0].Labels) != 0 {
		t.Errorf("the saved state's labels after taking one off: %+v", f.l.commits)
	}

	// Another tab deletes the label: the server refuses the mark and the UI
	// shows its problem as a toast.
	if err := f.api.DeleteLabel(ctx, label.ID); err != nil {
		t.Fatalf("DeleteLabel: %v", err)
	}
	f.l.assignLabel(f.l.compoCtx, 1, label.ID)
	f.run()
	if got := f.oneToast(); got.Status != http.StatusNotFound {
		t.Errorf("a mark of a deleted label: toast %+v, want a 404", got)
	}

	// "Manage labels" of an empty menu opens the tab's Labels window.
	f.l.manageLabels(f.l.compoCtx)
	f.run()
}
