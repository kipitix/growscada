package restapi

import (
	"testing"

	"github.com/google/uuid"
)

func TestSafeFileStem_KeepsOnlySafeCharacters(t *testing.T) {
	cases := map[string]string{
		"Main":            "Main",
		"Line 1":          "Line-1",
		"a/b\\c":          "a-b-c",
		"  spaced  name ": "spaced-name",
		"":                "",
		"///":             "",
		"v1.2_beta":       "v1.2_beta",
	}
	for name, want := range cases {
		if got := safeFileStem(name); got != want {
			t.Errorf("safeFileStem(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestProjectFileName_FallsBackToTheProjectID(t *testing.T) {
	projectID := uuid.New()
	if got := projectFileName("///", projectID, "draft"); got != projectID.String()+"-draft.json" {
		t.Errorf("filename %q, want the project's ID", got)
	}
	if got := projectFileName("Line 1", projectID, "revision-3"); got != "Line-1-revision-3.json" {
		t.Errorf("filename %q", got)
	}
}
