package label_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/label"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

func mustName(t *testing.T, raw string) label.Name {
	t.Helper()
	name, err := label.NewName(raw)
	if err != nil {
		t.Fatalf("name %q: %v", raw, err)
	}
	return name
}

func mustColor(t *testing.T, raw string) label.Color {
	t.Helper()
	color, err := label.NewColor(raw)
	if err != nil {
		t.Fatalf("color %q: %v", raw, err)
	}
	return color
}

func TestNewName_TrimsAndLimits(t *testing.T) {
	name, err := label.NewName("  Production  ")
	if err != nil {
		t.Fatalf("a valid name: %v", err)
	}
	if name.String() != "Production" {
		t.Errorf("the trimmed name: %q, want %q", name, "Production")
	}
	for _, raw := range []string{"", "   ", "\n", "two\nlines", strings.Repeat("x", label.NameMaxLength+1)} {
		if _, err := label.NewName(raw); !errors.Is(err, label.ErrInvalidLabelName) {
			t.Errorf("name %q: %v, want ErrInvalidLabelName", raw, err)
		}
	}
	if _, err := label.NewName(strings.Repeat("x", label.NameMaxLength)); err != nil {
		t.Errorf("a name of %d characters: %v", label.NameMaxLength, err)
	}
}

func TestNewColor_AcceptsHexOrEmpty(t *testing.T) {
	if color, err := label.NewColor(""); err != nil || !color.IsEmpty() || color.String() != "" {
		t.Errorf("an empty color: %q, %v", color, err)
	}
	color, err := label.NewColor(" #a1B2c3 ")
	if err != nil {
		t.Fatalf("a valid color: %v", err)
	}
	if color.String() != "#a1b2c3" {
		t.Errorf("the stored color: %q, want %q", color, "#a1b2c3")
	}
	for _, raw := range []string{"#12345", "#GGGGGG", "a1b2c3", "#a1b2c3ff", "red"} {
		if _, err := label.NewColor(raw); !errors.Is(err, label.ErrInvalidLabelColor) {
			t.Errorf("color %q: %v, want ErrInvalidLabelColor", raw, err)
		}
	}
}

func TestCreateLabel_RecordsCreated(t *testing.T) {
	orgID := id.NewID[organization.Organization]()
	at := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	created := label.CreateLabel(id.NewID[label.Label](), orgID, mustName(t, "Ready"), mustColor(t, "#22c55e"), at)

	if created.OrganizationID() != orgID || created.Name().String() != "Ready" || created.Color().String() != "#22c55e" {
		t.Errorf("the created label: %s", created)
	}
	if !created.CreatedAt().Equal(at) {
		t.Errorf("created at %s, want %s", created.CreatedAt(), at)
	}
	pending := created.PendingEvents()
	if len(pending) != 1 || pending[0].Type() != event.EventTypeLabelCreated {
		t.Fatalf("pending events: %v, want one label_created", pending)
	}
}

func TestLabel_RenameRecolorDelete_RecordUpdates(t *testing.T) {
	created := label.ReconstituteLabel(id.NewID[label.Label](), id.NewID[organization.Organization](),
		mustName(t, "Ready"), label.Color{}, time.Now())

	renamed, err := created.Rename(mustName(t, "Deployable"))
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if renamed.Name().String() != "Deployable" || created.Name().String() != "Ready" {
		t.Errorf("the rename changed the aggregate: %s / %s", renamed.Name(), created.Name())
	}
	if got := renamed.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeLabelUpdated {
		t.Errorf("the rename's events: %v", got)
	}
	if _, err := renamed.Rename(label.Name{}); !errors.Is(err, label.ErrInvalidLabelName) {
		t.Errorf("an empty rename: %v, want ErrInvalidLabelName", err)
	}

	recolored, err := created.Recolor(mustColor(t, "#3b82f6"))
	if err != nil {
		t.Fatalf("recolor: %v", err)
	}
	if recolored.Color().String() != "#3b82f6" || created.Color().String() != "" {
		t.Errorf("the recolor changed the aggregate: %q / %q", recolored.Color(), created.Color())
	}
	if got := recolored.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeLabelUpdated {
		t.Errorf("the recolor's events: %v", got)
	}
	cleared, err := recolored.Recolor(label.Color{})
	if err != nil || !cleared.Color().IsEmpty() {
		t.Errorf("clearing the color: %q, %v", cleared.Color(), err)
	}

	deleted := created.Delete()
	if got := deleted.PendingEvents(); len(got) != 1 || got[0].Type() != event.EventTypeLabelDeleted {
		t.Errorf("the delete's events: %v", got)
	}
}

func TestReconstituteLabel_RecordsNothing(t *testing.T) {
	createdAt := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	stored := label.ReconstituteLabel(id.NewID[label.Label](), id.NewID[organization.Organization](),
		mustName(t, "Ready"), label.FromStorage("#AABBCC"), createdAt)
	if len(stored.PendingEvents()) != 0 {
		t.Errorf("a reconstituted label records %v", stored.PendingEvents())
	}
	if stored.Color().String() != "#aabbcc" {
		t.Errorf("the stored color: %q, want lower case", stored.Color())
	}
}
