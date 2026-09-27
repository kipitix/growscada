package growctl_test

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/growctl"
	"github.com/kipitix/growscada/internal/server/application"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/servertest"
)

// The Postgres container starts only when an end-to-end test runs, so the unit
// tests of this package do not need Docker.
var postgres servertest.Postgres

func TestMain(m *testing.M) {
	code := m.Run()
	postgres.Close()
	os.Exit(code)
}

// runGrowctl runs growctl against the server with the manifest on stdin ("-f -").
func runGrowctl(t *testing.T, srv *httptest.Server, manifest string, args ...string) (string, error) {
	t.Helper()
	out, _, err := execGrowctl(t, srv, manifest, append(args, "-f", "-")...)
	return out, err
}

// execGrowctl runs growctl against the server with the given stdin and args,
// returning stdout and stderr.
func execGrowctl(t *testing.T, srv *httptest.Server, stdin string, args ...string) (string, string, error) {
	t.Helper()
	cmd := growctl.NewRootCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append(args, "--server", srv.URL))
	err := cmd.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

func tagsByName(t *testing.T, svc application.TagService) map[string]appdto.Tag {
	t.Helper()
	tags, err := svc.FindAllTags(context.Background())
	if err != nil {
		t.Fatalf("FindAllTags: %v", err)
	}
	byName := make(map[string]appdto.Tag, len(tags))
	for _, tg := range tags {
		byName[tg.Name] = tg
	}
	return byName
}

func exitCode(err error) int {
	var exitErr *growctl.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	if err != nil {
		return 1
	}
	return 0
}

func assertOutput(t *testing.T, got string, want ...string) {
	t.Helper()
	if expected := strings.Join(want, "\n") + "\n"; got != expected {
		t.Errorf("output:\ngot:\n%s\nwant:\n%s", got, expected)
	}
}

func TestApply_EmptyServer_CreatesTagsWithInitialValues(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))

	out, err := runGrowctl(t, srv, twoTags, "apply")

	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertOutput(t, out, "tag/pump1.running created", "tag/pump1.label created")
	tags := tagsByName(t, svc)
	running := tags["pump1.running"]
	if running.Type != "boolean" || running.Value != "false" || running.Quality != "good" {
		t.Errorf("pump1.running: got %+v", running)
	}
	if label := tags["pump1.label"]; label.Type != "string" || label.Value != "" || label.Quality != "uncertain" {
		t.Errorf("pump1.label: got %+v", label)
	}
}

func TestApply_Twice_IsIdempotentAndKeepsCurrentValue(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	if _, err := runGrowctl(t, srv, twoTags, "apply"); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	running := tagsByName(t, svc)["pump1.running"]
	if _, err := svc.SetTagValueByID(context.Background(), appdto.UpdateTagInput{
		ID: running.ID, Value: "true", Quality: "bad", Version: running.Version,
	}); err != nil {
		t.Fatalf("SetTagValueByID: %v", err)
	}

	out, err := runGrowctl(t, srv, twoTags, "apply")

	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	assertOutput(t, out, "tag/pump1.running unchanged", "tag/pump1.label unchanged")
	after := tagsByName(t, svc)["pump1.running"]
	if after.ID != running.ID || after.Value != "true" || after.Quality != "bad" || after.Version != running.Version+1 {
		t.Errorf("existing tag must keep its identity and current value, got %+v", after)
	}
}

func TestApply_WithoutPrune_KeepsExtraServerTags(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "extra", "integer")

	if _, err := runGrowctl(t, srv, twoTags, "apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	if _, ok := tagsByName(t, svc)["extra"]; !ok {
		t.Error("extra tag must be kept without --prune")
	}
}

func TestApply_Prune_DeletesExtraServerTags(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "zeta", "integer")
	createTag(t, svc, "alpha", "string")

	out, err := runGrowctl(t, srv, twoTags, "apply", "--prune")

	if err != nil {
		t.Fatalf("apply --prune: %v", err)
	}
	assertOutput(t, out, "tag/pump1.running created", "tag/pump1.label created", "tag/alpha pruned", "tag/zeta pruned")
	if tags := tagsByName(t, svc); len(tags) != 2 {
		t.Errorf("expected only the manifest's 2 tags, got %d", len(tags))
	}
}

func TestApply_TypeMismatch_FailsWithoutChangingAnything(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "pump1.running", "integer")
	createTag(t, svc, "extra", "integer")

	out, err := runGrowctl(t, srv, twoTags, "apply", "--prune")

	if err == nil || !strings.Contains(err.Error(), "tag/pump1.running: type is integer on the server, boolean in the manifest") {
		t.Fatalf("expected type mismatch error, got %v", err)
	}
	if out != "" {
		t.Errorf("expected no output, got %q", out)
	}
	tags := tagsByName(t, svc)
	if _, created := tags["pump1.label"]; created {
		t.Error("pump1.label must not be created when the plan fails")
	}
	if _, pruned := tags["extra"]; !pruned {
		t.Error("extra must not be pruned when the plan fails")
	}
}

func TestApply_InvalidManifest_FailsWithoutChangingAnything(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))

	_, err := runGrowctl(t, srv, twoTags+"---\napiVersion: growscada/v1\nkind: Scene\n", "apply")

	if err == nil || !strings.Contains(err.Error(), "unsupported kind") {
		t.Fatalf("expected manifest error, got %v", err)
	}
	if tags := tagsByName(t, svc); len(tags) != 0 {
		t.Errorf("expected no tags, got %d", len(tags))
	}
}

func TestApply_ServerUnreachable_ReturnsError(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	srv.Close()

	_, err := runGrowctl(t, srv, twoTags, "apply")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDiff_ReportsChangesWithExitCode1(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "pump1.label", "string")
	createTag(t, svc, "extra", "integer")

	out, err := runGrowctl(t, srv, twoTags, "diff", "--prune")

	if code := exitCode(err); code != 1 {
		t.Fatalf("exit code: expected 1, got %d (%v)", code, err)
	}
	assertOutput(t, out, "+ tag/pump1.running (boolean)", "- tag/extra")
	if tags := tagsByName(t, svc); len(tags) != 2 {
		t.Errorf("diff must not change the server, got %d tags", len(tags))
	}
}

func TestDiff_NoChanges_ExitCode0AndNoOutput(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))
	if _, err := runGrowctl(t, srv, twoTags, "apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	out, err := runGrowctl(t, srv, twoTags, "diff")

	if code := exitCode(err); code != 0 {
		t.Fatalf("exit code: expected 0, got %d (%v)", code, err)
	}
	if out != "" {
		t.Errorf("expected no output, got %q", out)
	}
}

func TestDiff_Error_ExitCode2(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "pump1.running", "integer")

	_, err := runGrowctl(t, srv, twoTags, "diff")

	if code := exitCode(err); code != 2 {
		t.Errorf("exit code: expected 2, got %d (%v)", code, err)
	}
}

func TestDelete_DeletesByNameAndReportsMissing(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	// Same name, different type: delete ignores the spec.
	createTag(t, svc, "pump1.running", "integer")
	createTag(t, svc, "extra", "integer")

	out, err := runGrowctl(t, srv, twoTags, "delete")

	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	assertOutput(t, out, "tag/pump1.running deleted", "tag/pump1.label not found")
	tags := tagsByName(t, svc)
	if _, ok := tags["pump1.running"]; ok {
		t.Error("pump1.running must be deleted")
	}
	if _, ok := tags["extra"]; !ok {
		t.Error("tags absent from the manifest must be kept")
	}
}

func TestUsageError_ExitCode2(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))

	tests := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"diff", "--no-such-flag", "-f", "-"}},
		{"apply without -f", []string{"apply"}},
		{"diff without -f", []string{"diff"}},
		{"unexpected argument", []string{"apply", "extra", "-f", "-"}},
		{"unknown command", []string{"no-such-command"}},
		{"unknown get resource", []string{"get", "no-such-resource"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := execGrowctl(t, srv, twoTags, tt.args...)

			if code := exitCode(err); code != 2 {
				t.Errorf("exit code: expected 2, got %d (%v)", code, err)
			}
		})
	}
}

func createTag(t *testing.T, svc application.TagService, name, tagType string) {
	t.Helper()
	value := map[string]string{"integer": "0", "string": "", "boolean": "false"}[tagType]
	if _, err := svc.CreateTag(context.Background(), appdto.CreateTagInput{
		Name: name, Type: tagType, Value: value, Quality: "good",
	}); err != nil {
		t.Fatalf("CreateTag(%q): %v", name, err)
	}
}

// --- name patterns ---

func TestApply_PruneWithPattern_PrunesOnlyInsideScope(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "pump1.old", "integer")  // in scope, not declared → pruned
	createTag(t, svc, "valve1.old", "integer") // out of scope → kept

	out, err := runGrowctl(t, srv, twoTags, "apply", "--prune", "--pattern", "pump1.*")

	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertOutput(t, out, "tag/pump1.running created", "tag/pump1.label created", "tag/pump1.old pruned")
	tags := tagsByName(t, svc)
	if _, ok := tags["valve1.old"]; !ok {
		t.Error("a tag outside the --pattern scope must not be pruned")
	}
}

func TestApply_PruneWithRegex_DoesNotPruneDeclaredTagsOutsideScope(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "pump1.label", "string") // declared, outside the regex scope
	createTag(t, svc, "pump1.old", "integer")

	out, err := runGrowctl(t, srv, twoTags, "apply", "--prune", "--regex", `\.(running|old)$`)

	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertOutput(t, out, "tag/pump1.running created", "tag/pump1.label unchanged", "tag/pump1.old pruned")
}

func TestDiff_PruneWithPattern_ShowsOnlyScopedPrunes(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "pump1.old", "integer")
	createTag(t, svc, "valve1.old", "integer")

	out, err := runGrowctl(t, srv, twoTags, "diff", "--prune", "--pattern", "pump?.*")

	if code := exitCode(err); code != 1 {
		t.Fatalf("exit code: expected 1, got %d (%v)", code, err)
	}
	assertOutput(t, out, "+ tag/pump1.running (boolean)", "+ tag/pump1.label (string)", "- tag/pump1.old")
}

func TestNameFilterUsageErrors_ExitCode2WithoutChanges(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"pattern without prune", []string{"apply", "-f", "-", "--pattern", "pump*"}},
		{"pattern and regex", []string{"apply", "-f", "-", "--prune", "--pattern", "a*", "--regex", "a"}},
		{"invalid regex", []string{"apply", "-f", "-", "--prune", "--regex", "pump("}},
		{"diff pattern without prune", []string{"diff", "-f", "-", "--pattern", "pump*"}},
		{"delete with -f and pattern", []string{"delete", "-f", "-", "--pattern", "pump*"}},
		{"delete without target", []string{"delete"}},
		{"get invalid regex", []string{"get", "tags", "--regex", "pump("}},
		{"get unknown output", []string{"get", "tags", "-o", "json"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, svc := servertest.StartAPI(t, postgres.DB(t))
			createTag(t, svc, "keep", "integer")

			_, _, err := execGrowctl(t, srv, twoTags, tt.args...)

			if code := exitCode(err); code != 2 {
				t.Errorf("exit code: expected 2, got %d (%v)", code, err)
			}
			if tags := tagsByName(t, svc); len(tags) != 1 {
				t.Errorf("usage error must not change the server, got %d tags", len(tags))
			}
		})
	}
}

func TestDeletePattern_Yes_DeletesMatchingTags(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "sim.b", "integer")
	createTag(t, svc, "sim.a", "integer")
	createTag(t, svc, "real.a", "integer")

	out, _, err := execGrowctl(t, srv, "", "delete", "--pattern", "sim.*", "--yes")

	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	assertOutput(t, out, "tag/sim.a deleted", "tag/sim.b deleted")
	if tags := tagsByName(t, svc); len(tags) != 1 {
		t.Errorf("expected only real.a to remain, got %v", tags)
	}
}

func TestDeletePattern_Confirmed_DeletesAfterPrompt(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "sim.a", "integer")

	out, prompt, err := execGrowctl(t, srv, "y\n", "delete", "--regex", "^sim")

	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(prompt, "tag/sim.a") || !strings.Contains(prompt, "Delete 1 tag(s)? [y/N]") {
		t.Errorf("prompt must list the tags and ask, got %q", prompt)
	}
	assertOutput(t, out, "tag/sim.a deleted")
}

func TestDeletePattern_NotConfirmed_DeletesNothing(t *testing.T) {
	for _, answer := range []string{"n\n", "\n", ""} {
		srv, svc := servertest.StartAPI(t, postgres.DB(t))
		createTag(t, svc, "sim.a", "integer")

		out, prompt, err := execGrowctl(t, srv, answer, "delete", "--pattern", "sim.*")

		if err != nil {
			t.Fatalf("answer %q: delete: %v", answer, err)
		}
		if out != "" || !strings.Contains(prompt, "Aborted") {
			t.Errorf("answer %q: expected abort, got out=%q prompt=%q", answer, out, prompt)
		}
		if tags := tagsByName(t, svc); len(tags) != 1 {
			t.Errorf("answer %q: nothing must be deleted", answer)
		}
	}
}

func TestDeletePattern_NoMatch_ReportsAndSucceeds(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))

	out, errOut, err := execGrowctl(t, srv, "", "delete", "--pattern", "nothing*")

	if err != nil || out != "" || !strings.Contains(errOut, "No tags match") {
		t.Errorf("got out=%q err-out=%q err=%v", out, errOut, err)
	}
}

func TestGetTags_Table_SortedByName(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "b.speed", "integer")
	createTag(t, svc, "a.running", "boolean")

	out, _, err := execGrowctl(t, srv, "", "get", "tags")

	if err != nil {
		t.Fatalf("get: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "NAME") ||
		!strings.HasPrefix(lines[1], "a.running") || !strings.HasPrefix(lines[2], "b.speed") {
		t.Errorf("unexpected table:\n%s", out)
	}
	for _, want := range []string{"boolean", "false", "good"} {
		if !strings.Contains(lines[1], want) {
			t.Errorf("row %q must contain %q", lines[1], want)
		}
	}
}

func TestGetTags_Filtered(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	createTag(t, svc, "pump1.speed", "integer")
	createTag(t, svc, "pump2.speed", "integer")
	createTag(t, svc, "valve1.state", "boolean")

	for _, args := range [][]string{
		{"get", "tags", "--pattern", "pump?.speed"},
		{"get", "tags", "--regex", `^pump\d`},
	} {
		out, _, err := execGrowctl(t, srv, "", args...)

		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if strings.Count(out, "\n") != 3 || strings.Contains(out, "valve1") {
			t.Errorf("%v: expected pump1 and pump2 only, got:\n%s", args, out)
		}
	}
}

func TestGetTags_YAML_RoundTripsThroughApply(t *testing.T) {
	srv, svc := servertest.StartAPI(t, postgres.DB(t))
	if _, err := runGrowctl(t, srv, twoTags, "apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	exported, _, err := execGrowctl(t, srv, "", "get", "tags", "-o", "yaml")
	if err != nil {
		t.Fatalf("get -o yaml: %v", err)
	}
	manifests, err := growctl.ParseManifests([]byte(exported))
	if err != nil {
		t.Fatalf("exported YAML is not a valid manifest: %v\n%s", err, exported)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(manifests))
	}

	// Re-applying the export to an empty server recreates the same tags.
	if _, err := postgres.DB(t).ExecContext(context.Background(), "DELETE FROM tags"); err != nil {
		t.Fatalf("clean tags: %v", err)
	}
	if _, err := runGrowctl(t, srv, exported, "apply"); err != nil {
		t.Fatalf("apply exported: %v", err)
	}
	if running := tagsByName(t, svc)["pump1.running"]; running.Type != "boolean" || running.Value != "false" || running.Quality != "good" {
		t.Errorf("pump1.running after round trip: %+v", running)
	}
}

func TestGetTags_Empty_ReportsNoTags(t *testing.T) {
	srv, _ := servertest.StartAPI(t, postgres.DB(t))

	out, errOut, err := execGrowctl(t, srv, "", "get", "tags")

	if err != nil || out != "" || !strings.Contains(errOut, "No tags found") {
		t.Errorf("got out=%q err-out=%q err=%v", out, errOut, err)
	}
}
