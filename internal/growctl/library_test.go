package growctl_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kipitix/growscada/internal/growctl"
)

const libraryManifest = `apiVersion: growscada/v0.4
kind: Library
metadata:
  name: Gauges
---
apiVersion: growscada/v0.4
kind: WidgetType
metadata:
  name: Lamp
  library: Gauges
spec:
  html: <div/>
  script: ""
  width: 40
  height: 40
  inputPorts:
    - name: on
      typeHint: boolean
`

func TestParseManifests_LibraryWidgetTypeAndPins(t *testing.T) {
	got, err := growctl.ParseManifests([]byte(libraryManifest + `---
apiVersion: growscada/v0.4
kind: Project
metadata:
  name: Line 1
spec:
  libraryPins:
    Gauges: 2
`))
	if err != nil {
		t.Fatalf("ParseManifests: %v", err)
	}
	if len(got.Libraries) != 1 || got.Libraries[0].Name != "Gauges" {
		t.Errorf("libraries: %+v", got.Libraries)
	}
	if len(got.WidgetTypes) != 1 {
		t.Fatalf("widget types: %+v", got.WidgetTypes)
	}
	wt := got.WidgetTypes[0]
	if wt.Library != "Gauges" || wt.Name() != "Lamp" || wt.Definition.ScriptLanguage != "javascript" ||
		wt.Definition.DefaultWidth != 40 || len(wt.Definition.InputPorts) != 1 || wt.Definition.InputPorts[0].TypeHint != "boolean" {
		t.Errorf("widget type: %+v", wt)
	}
	if len(got.Projects) != 1 || got.Projects[0].LibraryPins["Gauges"] != 2 {
		t.Errorf("projects: %+v", got.Projects)
	}
}

func TestParseManifests_InvalidLibraryDocuments(t *testing.T) {
	widgetType := func(spec string) string {
		return "apiVersion: growscada/v0.4\nkind: WidgetType\nmetadata: {name: Lamp, library: Gauges}\nspec: " + spec + "\n"
	}
	cases := []struct{ name, manifest, want string }{
		{"library of an older version", "apiVersion: growscada/v0.3\nkind: Library\nmetadata: {name: Gauges}\n", "appeared in apiVersion"},
		{"library without name", "apiVersion: growscada/v0.4\nkind: Library\nmetadata: {}\n", "metadata.name is required"},
		{"library with a spec", "apiVersion: growscada/v0.4\nkind: Library\nmetadata: {name: Gauges}\nspec: {}\n", "field spec not found"},
		{"widget type without library", "apiVersion: growscada/v0.4\nkind: WidgetType\nmetadata: {name: Lamp}\nspec: {html: '', script: '', width: 1, height: 1}\n", "metadata.library is required"},
		{"widget type without html", widgetType("{script: '', width: 1, height: 1}"), "spec.html is required"},
		{"zero width", widgetType("{html: '', script: '', width: 0, height: 1}"), "invalid spec.width"},
		{"unknown language", widgetType("{html: '', script: '', scriptLanguage: ruby, width: 1, height: 1}"), "invalid spec.scriptLanguage"},
		{"bad port name", widgetType("{html: '', script: '', width: 1, height: 1, inputPorts: [{name: 'a b'}]}"), "invalid spec.inputPorts"},
		{"duplicate port", widgetType("{html: '', script: '', width: 1, height: 1, inputPorts: [{name: a}, {name: a}]}"), "duplicate input port name"},
		{"unknown type hint", widgetType("{html: '', script: '', width: 1, height: 1, inputPorts: [{name: a, typeHint: float}]}"), "typeHint"},
		{"pin of release 0", "apiVersion: growscada/v0.4\nkind: Project\nmetadata: {name: a}\nspec: {libraryPins: {Gauges: 0}}\n", "invalid spec.libraryPins.Gauges"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := growctl.ParseManifests([]byte(tc.manifest))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %v does not contain %q", err, tc.want)
			}
		})
	}
}

func TestReadManifests_DuplicateWidgetTypeInALibrary_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), libraryManifest)
	writeFile(t, filepath.Join(dir, "b.yaml"), strings.SplitN(libraryManifest, "---\n", 2)[1])

	if _, err := growctl.ReadManifests([]string{dir}, nil); err == nil || !strings.Contains(err.Error(), "widgettype/Gauges/Lamp is declared more than once") {
		t.Errorf("expected the duplicate named, got %v", err)
	}
}

func TestReadManifests_ExampleLibraryManifest_IsValid(t *testing.T) {
	got, err := growctl.ReadManifests([]string{filepath.Join("..", "..", "tests", "manifests", "example_library.yaml")}, nil)
	if err != nil {
		t.Fatalf("example manifest is invalid: %v", err)
	}
	if len(got.Libraries) != 1 || len(got.WidgetTypes) != 2 || len(got.Projects) != 1 {
		t.Errorf("example manifest: %+v", got)
	}
}
