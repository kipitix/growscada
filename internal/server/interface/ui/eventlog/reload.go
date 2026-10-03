package eventlog

// ReloadTargets says which server-backed lists a ServerEvent makes stale.
// It is the single routing table of the modes that show server data
// (Library, Project, Operation); each reloads the lists it shows.
type ReloadTargets struct {
	WidgetTypes, Scenes, Widgets, Tags bool
}

// AllTargets is every list: what a resync reloads, since the events
// published while disconnected are lost.
var AllTargets = ReloadTargets{WidgetTypes: true, Scenes: true, Widgets: true, Tags: true}

// ReloadTargetsFor routes a ServerEvent by its Type.
func ReloadTargetsFor(eventType string) ReloadTargets {
	switch eventType {
	case EventResync:
		return AllTargets
	case "widget_type_created", "widget_type_updated", "widget_type_deleted":
		return ReloadTargets{WidgetTypes: true}
	case "scene_created", "scene_updated", "scene_deleted":
		return ReloadTargets{Scenes: true}
	case "widget_created", "widget_updated", "widget_deleted":
		// A widget change bumps its Scene's version (the shared optimistic
		// lock) without a scene event, so the scenes are reloaded as well.
		return ReloadTargets{Scenes: true, Widgets: true}
	case "tag_created", "tag_updated", "tag_deleted":
		// tag_updated carries the Tag's new state; a mode that applies it
		// directly (Operation) reloads only when it cannot.
		return ReloadTargets{Tags: true}
	}
	return ReloadTargets{}
}
