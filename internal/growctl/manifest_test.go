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
apiVersion: growscada/v1
kind: Tag
metadata:
  name: pump1.running
spec:
  type: boolean
  initialValue: false
  initialQuality: good
---
apiVersion: growscada/v1
kind: Tag
metadata:
  name: pump1.label
spec:
  type: string
  initialValue: ""
  initialQuality: uncertain
`

func TestParseManifests_Valid_ReturnsTagsInOrder(t *testing.T) {
	got, err := growctl.ParseManifests([]byte(twoTags))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []growctl.TagManifest{
		{Name: "pump1.running", Type: "boolean", InitialValue: "false", InitialQuality: "good"},
		{Name: "pump1.label", Type: "string", InitialValue: "", InitialQuality: "uncertain"},
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
	if len(got) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got))
	}
}

func TestParseManifests_Invalid_ReturnsError(t *testing.T) {
	const header = "apiVersion: growscada/v1\nkind: Tag\nmetadata:\n  name: t\n"
	tests := []struct {
		name     string
		manifest string
		wantErr  string
	}{
		{"wrong apiVersion", "apiVersion: growscada/v2\nkind: Tag\n", "unsupported apiVersion"},
		{"missing apiVersion", "kind: Tag\n", "unsupported apiVersion"},
		{"unknown kind", "apiVersion: growscada/v1\nkind: Scene\n", "unsupported kind"},
		{"unknown top-level field", header + "status: {}\nspec: {type: integer, initialValue: 0, initialQuality: good}\n", "field status not found"},
		{"unknown spec field", header + "spec: {type: integer, initialValue: 0, initialQuality: good, value: 1}\n", "field value not found"},
		{"missing name", "apiVersion: growscada/v1\nkind: Tag\nspec: {type: integer, initialValue: 0, initialQuality: good}\n", "metadata.name is required"},
		{"missing type", header + "spec: {initialValue: 0, initialQuality: good}\n", "spec.type is required"},
		{"missing initialValue", header + "spec: {type: integer, initialQuality: good}\n", "spec.initialValue is required"},
		{"missing initialQuality", header + "spec: {type: integer, initialValue: 0}\n", "spec.initialQuality is required"},
		{"invalid type", header + "spec: {type: float, initialValue: 0, initialQuality: good}\n", "invalid spec.type"},
		{"value not matching type", header + "spec: {type: integer, initialValue: abc, initialQuality: good}\n", "invalid spec.initialValue"},
		{"invalid quality", header + "spec: {type: integer, initialValue: 0, initialQuality: unknown}\n", "invalid spec.initialQuality"},
		{"malformed yaml", "apiVersion: [", "document 1"},
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
	if len(got) != 2 || got[0].Name != "first" || got[1].Name != "second" {
		t.Errorf("got %+v, want first then second", got)
	}
}

func TestReadManifests_Stdin_ReadsFromReader(t *testing.T) {
	got, err := growctl.ReadManifests([]string{"-"}, strings.NewReader(tagYAML("from-stdin")))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "from-stdin" {
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
	writeFile(t, file, "apiVersion: growscada/v1\nkind: Scene\n")

	_, err := growctl.ReadManifests([]string{file}, nil)

	if err == nil || !strings.Contains(err.Error(), file) {
		t.Errorf("expected error mentioning %s, got %v", file, err)
	}
}

func tagYAML(name string) string {
	return "apiVersion: growscada/v1\nkind: Tag\nmetadata:\n  name: " + name +
		"\nspec:\n  type: integer\n  initialValue: \"0\"\n  initialQuality: good\n"
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
	if len(got) == 0 {
		t.Error("example manifest declares no tags")
	}
}
