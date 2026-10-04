package manifestv0

import (
	"testing"

	"github.com/kipitix/growscada/contract"
	"github.com/kipitix/growscada/contract/internal/jsonschema"
)

func TestParseAPIVersion(t *testing.T) {
	valid := map[string]contract.SchemaVersion{
		"growscada/v0":   {Major: 0, Minor: 0},
		"growscada/v0.1": {Major: 0, Minor: 1},
		"growscada/v1.2": {Major: 1, Minor: 2},
	}
	for text, want := range valid {
		if got, err := ParseAPIVersion(text); err != nil || got != want {
			t.Errorf("ParseAPIVersion(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
	for _, text := range []string{"", "growscada/", "growscada/v", "growscada/0.1", "other/v0.1", "growscada/v0.1.2", "growscada/vx"} {
		if v, err := ParseAPIVersion(text); err == nil {
			t.Errorf("ParseAPIVersion(%q) = %v, want error", text, v)
		}
	}
	if got := APIVersion(); got != "growscada/v"+SchemaVersion().String() {
		t.Errorf("APIVersion() = %q", got)
	}
}

func TestSchemaIsCommitted(t *testing.T) {
	jsonschema.CheckCommitted(t, "../../../schemas/manifest/"+SchemaVersion().String()+".json", jsonschema.Document{
		ID:    "https://github.com/kipitix/growscada/schemas/manifest/" + SchemaVersion().String() + ".json",
		Title: "GrowSCADA growctl manifest " + SchemaVersion().String(),
		Tag:   "yaml",
		Roots: []any{TagDocument{}},
	})
}
