package projectv0

import (
	"testing"

	"github.com/kipitix/growscada/contract/internal/jsonschema"
)

func TestSchemaIsCommitted(t *testing.T) {
	jsonschema.CheckCommitted(t, "../../../schemas/project/"+SchemaVersion.String()+".json", jsonschema.Document{
		ID:    "https://github.com/kipitix/growscada/schemas/project/" + SchemaVersion.String() + ".json",
		Title: "GrowSCADA project format " + SchemaVersion.String(),
		Tag:   "json",
		Roots: []any{Header{}},
	})
}
