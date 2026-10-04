package jsonschema

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFreezeOlder_ThenCheckFrozen(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"0.1.json": "{}", "0.2.json": `{"a":1}`, "0.3.json": "current"})
	current := filepath.Join(dir, "0.3.json")

	if problems := checkFrozen(current); len(problems) != 2 {
		t.Fatalf("before freezing: problems = %v, want the two older versions unfrozen", problems)
	}
	if err := freezeOlder(current); err != nil {
		t.Fatal(err)
	}
	if problems := checkFrozen(current); len(problems) != 0 {
		t.Fatalf("after freezing: %v", problems)
	}

	// The current version is not frozen and may change.
	writeFiles(t, dir, map[string]string{"0.3.json": "changed"})
	if problems := checkFrozen(current); len(problems) != 0 {
		t.Fatalf("current changed: %v", problems)
	}

	// An older one may not, and freezing again does not bless the change.
	writeFiles(t, dir, map[string]string{"0.1.json": "{ }"})
	if err := freezeOlder(current); err != nil {
		t.Fatal(err)
	}
	problems := checkFrozen(current)
	if len(problems) != 1 || !strings.Contains(problems[0], "0.1.json") || !strings.Contains(problems[0], "must not change") {
		t.Fatalf("older changed: problems = %v", problems)
	}
}

func TestCheckFrozen_ListedFileMissing(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"0.1.json": "{}", "0.2.json": "current"})
	current := filepath.Join(dir, "0.2.json")
	if err := freezeOlder(current); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(dir, "0.1.json"))
	if problems := checkFrozen(current); len(problems) != 1 || !strings.Contains(problems[0], "missing") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestPreviousMinor(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"0.1.json": "", "0.3.json": "", "1.0.json": "", "1.2.json": "", frozenFile: ""})
	for current, want := range map[string]string{
		"0.1.json": "", "0.4.json": "0.3.json", "1.0.json": "", "1.2.json": "1.0.json",
	} {
		got, err := previousMinor(filepath.Join(dir, current))
		if err != nil {
			t.Fatal(err)
		}
		if want != "" {
			want = filepath.Join(dir, want)
		}
		if got != want {
			t.Errorf("previousMinor(%s) = %q, want %q", current, got, want)
		}
	}
}

func TestPublishedSchema(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.MkdirAll(filepath.Join(repo, "schemas", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, filepath.Join(repo, "schemas", "api"), map[string]string{"0.1.json": "published"})
	run("add", ".")
	run("commit", "-q", "-m", "publish 0.1")

	got, err := publishedSchema("main", filepath.Join(repo, "schemas", "api", "0.1.json"))
	if err != nil || string(got) != "published" {
		t.Errorf("published version: %q, %v", got, err)
	}
	got, err = publishedSchema("main", filepath.Join(repo, "schemas", "api", "0.2.json"))
	if err != nil || got != nil {
		t.Errorf("new version: %q, %v; want nil, nil", got, err)
	}
	if _, err := publishedSchema("nope", filepath.Join(repo, "schemas", "api", "0.1.json")); err == nil || !strings.Contains(err.Error(), errUnchecked.Error()) {
		t.Errorf("missing branch: err = %v, want errUnchecked", err)
	}
}
