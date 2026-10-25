package eventlog

import (
	"fmt"
	"strings"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// category groups event types for the filter toggles shown in the panel.
type category string

const (
	categoryProject    category = "Project"
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
	categoryProject,
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
		categoryProject:    true,
		categoryTag:        true,
		categoryWidget:     true,
		categoryWidgetType: true,
		categoryScene:      true,
		categoryValue:      false,
		categoryClient:     true,
		categoryOther:      true,
	}
}

// eventTypeInfo describes how one type of the stream's events maps to a
// filter category and a human-readable description (with "%s" for the id, if
// the type carries one).
type eventTypeInfo struct {
	category category
	format   string
}

var eventTypeInfoByType = map[string]eventTypeInfo{
	apiv0.EventSystemReady: {categoryOther, "Server ready"},

	apiv0.EventProjectCreated: {categoryProject, "Project %s created"},
	apiv0.EventProjectUpdated: {categoryProject, "Project %s updated"},
	apiv0.EventProjectDeleted: {categoryProject, "Project %s deleted"},

	apiv0.EventTagCreated: {categoryTag, "Tag %s created"},
	apiv0.EventTagUpdated: {categoryValue, "Tag %s value updated"},
	apiv0.EventTagDeleted: {categoryTag, "Tag %s deleted"},

	apiv0.EventWidgetCreated: {categoryWidget, "Widget %s created"},
	apiv0.EventWidgetUpdated: {categoryWidget, "Widget %s updated"},
	apiv0.EventWidgetDeleted: {categoryWidget, "Widget %s deleted"},

	apiv0.EventWidgetTypeCreated: {categoryWidgetType, "WidgetType %s created"},
	apiv0.EventWidgetTypeUpdated: {categoryWidgetType, "WidgetType %s updated"},
	apiv0.EventWidgetTypeDeleted: {categoryWidgetType, "WidgetType %s deleted"},

	apiv0.EventSceneCreated: {categoryScene, "Scene %s created"},
	apiv0.EventSceneUpdated: {categoryScene, "Scene %s updated"},
	apiv0.EventSceneDeleted: {categoryScene, "Scene %s deleted"},

	apiv0.EventClientConnected:    {categoryClient, "Client %s connected"},
	apiv0.EventClientDisconnected: {categoryClient, "Client %s disconnected"},
}

// describe is the log text of an event of this type with the given id.
func (info eventTypeInfo) describe(id string) string {
	if !strings.Contains(info.format, "%s") {
		return info.format
	}
	return fmt.Sprintf(info.format, id)
}
