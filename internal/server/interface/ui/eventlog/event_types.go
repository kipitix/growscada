package eventlog

// category groups event types for the filter toggles shown in the panel.
type category string

const (
	categoryTag        category = "Tag"
	categoryWidget     category = "Widget"
	categoryWidgetType category = "WidgetType"
	categoryScene      category = "Scene"
	categoryValue      category = "Value"
	categoryClient     category = "Client"
	categoryOther      category = "Other"
)

// allCategories lists every filter toggle, in display order.
var allCategories = []category{
	categoryTag,
	categoryWidget,
	categoryWidgetType,
	categoryScene,
	categoryValue,
	categoryClient,
	categoryOther,
}

// defaultEnabled is the initial filter state: everything is shown except
// per-write tag value updates, which would otherwise flood the log.
func defaultFilters() map[category]bool {
	return map[category]bool{
		categoryTag:        true,
		categoryWidget:     true,
		categoryWidgetType: true,
		categoryScene:      true,
		categoryValue:      false,
		categoryClient:     true,
		categoryOther:      true,
	}
}

// eventTypeInfo describes how one server EventType.String() value maps to a
// filter category and a human-readable description (with "%s" for the id).
type eventTypeInfo struct {
	category category
	format   string
}

var eventTypeInfoByType = map[string]eventTypeInfo{
	"tag_created": {categoryTag, "Tag %s created"},
	"tag_updated": {categoryValue, "Tag %s value updated"},
	"tag_deleted": {categoryTag, "Tag %s deleted"},

	"widget_created": {categoryWidget, "Widget %s created"},
	"widget_updated": {categoryWidget, "Widget %s updated"},
	"widget_deleted": {categoryWidget, "Widget %s deleted"},

	"widget_type_created": {categoryWidgetType, "WidgetType %s created"},
	"widget_type_updated": {categoryWidgetType, "WidgetType %s updated"},
	"widget_type_deleted": {categoryWidgetType, "WidgetType %s deleted"},

	"scene_created": {categoryScene, "Scene %s created"},
	"scene_updated": {categoryScene, "Scene %s updated"},
	"scene_deleted": {categoryScene, "Scene %s deleted"},

	"client_connected":    {categoryClient, "Client %s connected"},
	"client_disconnected": {categoryClient, "Client %s disconnected"},
}
