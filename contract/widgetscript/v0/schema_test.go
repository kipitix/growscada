package widgetscriptv0

import (
	"testing"

	"github.com/kipitix/growscada/contract/internal/jsonschema"
)

func TestSchemaIsCommitted(t *testing.T) {
	jsonschema.CheckCommitted(t, "../../../schemas/widgetscript/"+SchemaVersion().String()+".json", jsonschema.Document{
		ID:    "https://github.com/kipitix/growscada/schemas/widgetscript/" + SchemaVersion().String() + ".json",
		Title: "GrowSCADA WidgetType script contract " + SchemaVersion().String(),
		Tag:   "json",
		Roots: []any{Inputs{}},
	})
}
