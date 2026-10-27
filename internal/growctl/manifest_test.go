package growctl_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/growctl"
)

const twoTags = `
apiVersion: growscada/v0.3
kind: Tag
metadata:
  name: pump1.running
spec:
  type: boolean
---
apiVersion: growscada/v0.3
kind: Tag
metadata:
  name: pump1.label
spec:
  type: string
`

func TestParseManifests_Valid_ReturnsTagsInOrder(t *testing.T) {
	parsed, err := growctl.ParseManifests([]byte(twoTags))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := parsed.Tags
	want := []growctl.TagManifest{
		{Name: "pump1.running", Type: "boolean"},
		{Name: "pump1.label", Type: "string"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseManifests_EmptyDocuments_AreSkipped(t *testing.T) {
	got, err := growctl.ParseManifests([]byte("---\n---\n" + twoTags + "\n---\n"))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got.Tags))
	}
}

func TestParseManifests_OlderMinor_IsAccepted(t *testing.T) {
	// "growscada/v0" reads as v0.0, which this growctl (v0.3) understands.
	got, err := growctl.ParseManifests([]byte("apiVersion: growscada/v0\nkind: Tag\nmetadata:\n  name: t\n" +
		"spec: {type: integer}\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "t" {
		t.Errorf("got %+v", got)
	}
}

func TestParseManifests_ProjectsAndTagsOfAProject(t *testing.T) {
	got, err := growctl.ParseManifests([]byte(`
apiVersion: growscada/v0.2
kind: Project
metadata:
  name: Line 1
---
apiVersion: growscada/v0.2
kind: Tag
metadata:
  name: speed
  project: Line 1
spec: {type: integer}
`))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got.Projects, []growctl.ProjectManifest{{Name: "Line 1"}}) {
		t.Errorf("projects: %+v", got.Projects)
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "speed" || got.Tags[0].Project != "Line 1" {
		t.Errorf("tags: %+v", got.Tags)
	}
}

func TestParseManifests_Invalid_ReturnsError(t *testing.T) {
	const header = "apiVersion: growscada/v0.1\nkind: Tag\nmetadata:\n  name: t\n"
	tests := []struct {
		name     string
		manifest string
		wantErr  string
	}{
		{"wrong apiVersion", "apiVersion: growscada/v2\nkind: Tag\n", "unsupported apiVersion"},
		{"missing apiVersion", "kind: Tag\n", "unsupported apiVersion"},
		{"other MAJOR", "apiVersion: growscada/v1.0\nkind: Tag\n", "unsupported apiVersion"},
		{"pre-versioning label", "apiVersion: growscada/v1\nkind: Tag\n", `replace it with "growscada/v0.`},
		{"newer MINOR with a new field", "apiVersion: growscada/v0.99\nkind: Tag\nmetadata:\n  name: t\nspec: {type: integer, unit: rpm}\n", "update growctl"},
		{"unknown kind", "apiVersion: growscada/v0.1\nkind: Scene\n", "unsupported kind"},
		{"unknown top-level field", header + "status: {}\nspec: {type: integer}\n", "field status not found"},
		{"unknown spec field", header + "spec: {type: integer, value: 1}\n", "field value not found"},
		{"missing name", "apiVersion: growscada/v0.1\nkind: Tag\nspec: {type: integer}\n", "metadata.name is required"},
		{"missing type", header + "spec: {}\n", "spec.type is required"},
		{"invalid type", header + "spec: {type: float}\n", "invalid spec.type"},
		// A tag's state is no configuration (ADR 0011): the fields of the
		// older versions are named as removed, in a document of any version.
		{"initialValue of v0.2", "apiVersion: growscada/v0.2\nkind: Tag\nmetadata:\n  name: t\nspec: {type: integer, initialValue: 0, initialQuality: good}\n",
			`tag/t: spec.initialValue was removed in apiVersion "growscada/v0.3"`},
		{"initialQuality of v0.3", "apiVersion: growscada/v0.3\nkind: Tag\nmetadata:\n  name: t\nspec: {type: integer, initialQuality: good}\n",
			"tag/t: spec.initialQuality was removed"},
		{"malformed yaml", "apiVersion: [", "document 1"},
		{"project without name", "apiVersion: growscada/v0.2\nkind: Project\nmetadata: {}\n", "metadata.name is required"},
		{"project with a project", "apiVersion: growscada/v0.2\nkind: Project\nmetadata: {name: a, project: b}\n", "field project not found"},
		{"project with a spec", "apiVersion: growscada/v0.2\nkind: Project\nmetadata: {name: a}\nspec: {}\n", "spec appeared in apiVersion"},
		{"project in v0.1", "apiVersion: growscada/v0.1\nkind: Project\nmetadata: {name: a}\n", `kind Project appeared in apiVersion "growscada/v0.2"`},
		{"metadata.project in v0.1", "apiVersion: growscada/v0.1\nkind: Tag\nmetadata: {name: t, project: a}\nspec: {type: integer}\n", "tag/t: metadata.project appeared in"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := growctl.ParseManifests([]byte(tt.manifest))

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestReadManifests_Directory_ReadsYAMLFilesInNameOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "b.yml"), tagYAML("second"))
	writeFile(t, filepath.Join(dir, "a.yaml"), tagYAML("first"))
	writeFile(t, filepath.Join(dir, "notes.txt"), "not a manifest")

	got, err := growctl.ReadManifests([]string{dir}, nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tags) != 2 || got.Tags[0].Name != "first" || got.Tags[1].Name != "second" {
		t.Errorf("got %+v, want first then second", got)
	}
}

func TestReadManifests_Stdin_ReadsFromReader(t *testing.T) {
	got, err := growctl.ReadManifests([]string{"-"}, strings.NewReader(tagYAML("from-stdin")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Tags) != 1 || got.Tags[0].Name != "from-stdin" {
		t.Errorf("got %+v", got)
	}
}

func TestReadManifests_DuplicateNameAcrossFiles_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	b := filepath.Join(dir, "b.yaml")
	writeFile(t, a, tagYAML("same"))
	writeFile(t, b, tagYAML("same"))

	_, err := growctl.ReadManifests([]string{a, b}, nil)

	if err == nil || !strings.Contains(err.Error(), "tag/same is declared more than once") {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestReadManifests_NoSources_ReturnsError(t *testing.T) {
	_, err := growctl.ReadManifests(nil, nil)

	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestReadManifests_ErrorMentionsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.yaml")
	writeFile(t, file, "apiVersion: growscada/v0.1\nkind: Scene\n")

	_, err := growctl.ReadManifests([]string{file}, nil)

	if err == nil || !strings.Contains(err.Error(), file) {
		t.Errorf("expected error mentioning %s, got %v", file, err)
	}
}

func tagYAML(name string) string {
	return "apiVersion: growscada/v0.3\nkind: Tag\nmetadata:\n  name: " + name +
		"\nspec:\n  type: integer\n"
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReadManifests_ExampleManifest_IsValid(t *testing.T) {
	got, err := growctl.ReadManifests([]string{filepath.Join("..", "..", "tests", "manifests", "example_tags.yaml")}, nil)

	if err != nil {
		t.Fatalf("example manifest is invalid: %v", err)
	}
	if len(got.Tags) == 0 {
		t.Error("example manifest declares no tags")
	}
}

func TestReadManifests_DuplicateProject_ReturnsError(t *testing.T) {
	const project = "apiVersion: growscada/v0.2\nkind: Project\nmetadata:\n  name: Line 1\n"

	_, err := growctl.ReadManifests([]string{"-"}, strings.NewReader(project+"---\n"+project))

	if err == nil || !strings.Contains(err.Error(), "project/Line 1 is declared more than once") {
		t.Errorf("expected duplicate error, got %v", err)
	}
}
