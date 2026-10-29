package root

import (
	"testing"

	"github.com/google/uuid"
)

// TestSelectProject_BarFollowsTheModeShown: each mode keeps the project its
// switcher selects; the Bar follows that of the mode shown, and Library,
// which shows none, leaves it as it is.
func TestSelectProject_BarFollowsTheModeShown(t *testing.T) {
	r := NewRoot("http://127.0.0.1:0")
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	follows := func(want uuid.UUID) {
		t.Helper()
		if got := r.bar.Project(); got != want {
			t.Errorf("Bar follows %v, want %v", got, want)
		}
	}

	r.SetMode(ModeProject)
	r.selectProject(ModeProject, a)
	follows(a)
	r.selectProject(ModeOperation, b) // hidden: the Bar stays
	follows(a)

	r.SetMode(ModeOperation)
	follows(b)
	r.SetMode(ModeLibrary)
	follows(b)
	r.SetMode(ModeHistory) // its switcher has not announced yet
	follows(b)
	r.selectProject(ModeHistory, c)
	follows(c)
	r.SetMode(ModeProject)
	follows(a)
}
