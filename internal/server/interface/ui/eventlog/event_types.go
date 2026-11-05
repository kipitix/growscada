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
	categoryLibrary    category = "Library"
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
	categoryLibrary,
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
		categoryLibrary:    true,
		categoryScene:      true,
		categoryValue:      false,
		categoryClient:     true,
		categoryOther:      true,
	}
}

// isProjectContent reports whether an event is of a project's content (Tags,
// their values, Scenes, Widgets, Library Pins, Devices, Environments and their
// pairs, the Revision, and the EditLock of the project's Draft), carried by
// the stream of that project; anything else is the server's. A Library's
// EditLock is the server's, though its type is a Draft's.
func isProjectContent(ev apiv0.Event) bool {
	switch ev.Type {
	case apiv0.EventLibraryPinCreated, apiv0.EventLibraryPinUpdated, apiv0.EventLibraryPinDeleted,
		apiv0.EventDeviceCreated, apiv0.EventDeviceUpdated, apiv0.EventDeviceDeleted,
		apiv0.EventEnvironmentCreated, apiv0.EventEnvironmentUpdated, apiv0.EventEnvironmentDeleted,
		apiv0.EventEnvironmentDeviceCreated, apiv0.EventEnvironmentDeviceUpdated, apiv0.EventEnvironmentDeviceDeleted,
		apiv0.EventRevisionCreated:
		return true
	case apiv0.EventEditLockAcquired, apiv0.EventEditLockReleased,
		apiv0.EventEditLockTakenOver, apiv0.EventEditLockExpired:
		// A Project's lock travels on that Project's stream; a Library's,
		// which names its library, on the server's (ADR 0015).
		return ev.LibraryID == ""
	}
	switch eventTypeInfoByType[ev.Type].category {
	case categoryTag, categoryValue, categoryScene, categoryWidget:
		return true
	}
	return false
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

	apiv0.EventLibraryCreated:        {categoryLibrary, "Library %s created"},
	apiv0.EventLibraryUpdated:        {categoryLibrary, "Library %s updated"},
	apiv0.EventLibraryDeleted:        {categoryLibrary, "Library %s deleted"},
	apiv0.EventLibraryDiscardStarted: {categoryLibrary, "Library %s being discarded"},
	apiv0.EventLibraryReleased:       {categoryLibrary, "Library %s released"},
	apiv0.EventLibraryPinCreated:     {categoryLibrary, "Library %s pinned"},
	apiv0.EventLibraryPinUpdated:     {categoryLibrary, "Library %s pin switched"},
	apiv0.EventLibraryPinDeleted:     {categoryLibrary, "Library %s unpinned"},

	apiv0.EventSceneCreated: {categoryScene, "Scene %s created"},
	apiv0.EventSceneUpdated: {categoryScene, "Scene %s updated"},
	apiv0.EventSceneDeleted: {categoryScene, "Scene %s deleted"},

	apiv0.EventClientConnected:    {categoryClient, "Client %s connected"},
	apiv0.EventClientDisconnected: {categoryClient, "Client %s disconnected"},

	// The Devices and Environments of a project. Their own filter
	// categories wait for the Devices panel (task 66).
	apiv0.EventDeviceCreated:            {categoryOther, "Device %s created"},
	apiv0.EventDeviceUpdated:            {categoryOther, "Device %s updated"},
	apiv0.EventDeviceDeleted:            {categoryOther, "Device %s deleted"},
	apiv0.EventEnvironmentCreated:       {categoryOther, "Environment %s created"},
	apiv0.EventEnvironmentUpdated:       {categoryOther, "Environment %s updated"},
	apiv0.EventEnvironmentDeleted:       {categoryOther, "Environment %s deleted"},
	apiv0.EventEnvironmentDeviceCreated: {categoryOther, "EnvironmentDevice %s created"},
	apiv0.EventEnvironmentDeviceUpdated: {categoryOther, "EnvironmentDevice %s updated"},
	apiv0.EventEnvironmentDeviceDeleted: {categoryOther, "EnvironmentDevice %s deleted"},

	apiv0.EventRevisionCreated: {categoryProject, "Project deployed"},

	apiv0.EventEditLockAcquired:  {categoryProject, "Draft %s checked out"},
	apiv0.EventEditLockReleased:  {categoryProject, "Draft %s checked in"},
	apiv0.EventEditLockTakenOver: {categoryProject, "Draft %s taken over"},
	apiv0.EventEditLockExpired:   {categoryProject, "Draft %s lock expired"},
}

// describe is the log text of an event of this type with the given id.
func (info eventTypeInfo) describe(id string) string {
	if !strings.Contains(info.format, "%s") {
		return info.format
	}
	return fmt.Sprintf(info.format, id)
}
