package recordv0

import (
	"testing"

	"github.com/kipitix/growscada/contract/internal/jsonschema"
)

func TestSchemaIsCommitted(t *testing.T) {
	jsonschema.CheckCommitted(t, "../../../schemas/record/"+SchemaVersion.String()+".json", jsonschema.Document{
		ID:    "https://github.com/kipitix/growscada/schemas/record/" + SchemaVersion.String() + ".json",
		Title: "GrowSCADA operational record format " + SchemaVersion.String(),
		Tag:   "json",
		Roots: []any{Header{}},
	})
}
