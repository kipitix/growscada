package growctl_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

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
