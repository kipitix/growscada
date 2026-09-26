package growctl_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/growctl"
)

func manifest(name, tagType string) growctl.TagManifest {
	return growctl.TagManifest{Name: name, Type: tagType, InitialValue: "0", InitialQuality: "good"}
}

func serverTag(name, tagType string) growctl.ServerTag {
	return growctl.ServerTag{ID: uuid.New(), Name: name, Type: tagType}
}

// summary renders steps as "action:name" for compact comparison.
func summary(steps []growctl.Step) []string {
	names := map[growctl.Action]string{
		growctl.ActionCreate:    "create",
		growctl.ActionUnchanged: "unchanged",
		growctl.ActionPrune:     "prune",
		growctl.ActionDelete:    "delete",
		growctl.ActionNotFound:  "notfound",
	}
	out := make([]string, len(steps))
	for i, s := range steps {
		out[i] = names[s.Action] + ":" + s.Name()
	}
	return out
}

func TestPlanApply(t *testing.T) {
	tests := []struct {
		name      string
		manifests []growctl.TagManifest
		server    []growctl.ServerTag
		prune     bool
		want      []string
	}{
		{
			name:      "empty server creates all in manifest order",
			manifests: []growctl.TagManifest{manifest("b", "integer"), manifest("a", "boolean")},
			want:      []string{"create:b", "create:a"},
		},
		{
			name:      "existing tag with same type is unchanged",
			manifests: []growctl.TagManifest{manifest("a", "integer"), manifest("b", "integer")},
			server:    []growctl.ServerTag{serverTag("a", "integer")},
			want:      []string{"unchanged:a", "create:b"},
		},
		{
			name:      "extra server tags are kept without prune",
			manifests: []growctl.TagManifest{manifest("a", "integer")},
			server:    []growctl.ServerTag{serverTag("a", "integer"), serverTag("z", "string")},
			want:      []string{"unchanged:a"},
		},
		{
			name:      "prune deletes extra server tags last, sorted by name",
			manifests: []growctl.TagManifest{manifest("a", "integer")},
			server:    []growctl.ServerTag{serverTag("z", "string"), serverTag("a", "integer"), serverTag("m", "boolean")},
			prune:     true,
			want:      []string{"unchanged:a", "prune:m", "prune:z"},
		},
		{
			name:   "prune with empty manifest deletes everything",
			server: []growctl.ServerTag{serverTag("a", "integer")},
			prune:  true,
			want:   []string{"prune:a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps, err := growctl.PlanApply(tt.manifests, tt.server, tt.prune)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := summary(steps); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlanApply_TypeMismatch_FailsWholePlanListingAllMismatches(t *testing.T) {
	manifests := []growctl.TagManifest{manifest("a", "integer"), manifest("new", "integer"), manifest("b", "string")}
	server := []growctl.ServerTag{serverTag("a", "boolean"), serverTag("b", "integer")}

	steps, err := growctl.PlanApply(manifests, server, true)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if steps != nil {
		t.Errorf("expected no steps, got %v", summary(steps))
	}
	for _, want := range []string{
		"tag/a: type is boolean on the server, integer in the manifest",
		"tag/b: type is integer on the server, string in the manifest",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestPlanApply_UnchangedStepCarriesServerTag(t *testing.T) {
	existing := serverTag("a", "integer")

	steps, _ := growctl.PlanApply([]growctl.TagManifest{manifest("a", "integer")}, []growctl.ServerTag{existing}, false)

	if steps[0].Server != existing {
		t.Errorf("server tag: got %+v, want %+v", steps[0].Server, existing)
	}
}

func TestPlanDelete(t *testing.T) {
	existing := serverTag("a", "boolean")
	manifests := []growctl.TagManifest{manifest("a", "integer"), manifest("missing", "integer")}

	steps := growctl.PlanDelete(manifests, []growctl.ServerTag{existing, serverTag("other", "string")})

	if got, want := summary(steps), []string{"delete:a", "notfound:missing"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if steps[0].Server.ID != existing.ID {
		t.Errorf("delete step must target the server tag's ID")
	}
}

func TestStep_IsChange(t *testing.T) {
	for action, want := range map[growctl.Action]bool{
		growctl.ActionCreate:    true,
		growctl.ActionPrune:     true,
		growctl.ActionDelete:    true,
		growctl.ActionUnchanged: false,
		growctl.ActionNotFound:  false,
	} {
		if got := (growctl.Step{Action: action}).IsChange(); got != want {
			t.Errorf("action %d: IsChange() = %v, want %v", action, got, want)
		}
	}
}
