package eventlog

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ReloadTargets says which server-backed lists an event makes stale.
// It is the single routing table of the modes that show server data
// (Library, Project, Operation); each reloads the lists it shows.
type ReloadTargets struct {
	WidgetTypes, Scenes, Widgets, Tags bool
	// Scene is the one Scene whose own state and Widgets are stale (a Widget
	// of it changed, which bumps its version without a scene event); uuid.Nil
	// if none. The list of Scenes is not: only scene events and a resync
	// change it.
	Scene uuid.UUID
}

// AllTargets is every list: what a resync reloads, since the events
// published while disconnected are lost.
var AllTargets = ReloadTargets{WidgetTypes: true, Scenes: true, Widgets: true, Tags: true}

// ReloadTargetsFor routes an event of the stream (or EventResync) by its
// Type. tag_updated with the Tag's new state makes nothing stale: the modes
// apply the state themselves.
func ReloadTargetsFor(ev apiv0.Event) ReloadTargets {
	switch ev.Type {
	case EventResync:
		return AllTargets
	case apiv0.EventWidgetTypeCreated, apiv0.EventWidgetTypeUpdated, apiv0.EventWidgetTypeDeleted:
		return ReloadTargets{WidgetTypes: true}
	case apiv0.EventSceneCreated, apiv0.EventSceneUpdated, apiv0.EventSceneDeleted:
		return ReloadTargets{Scenes: true}
	case apiv0.EventWidgetCreated, apiv0.EventWidgetUpdated, apiv0.EventWidgetDeleted:
		// The only place scene_id is read.
		if sceneID := uiutil.ParseID(ev.SceneID); sceneID != uuid.Nil {
			return ReloadTargets{Scene: sceneID}
		}
		// Not knowing the Scene, every Scene (the widget change bumped its
		// version) and the shown Widgets may be stale.
		return ReloadTargets{Scenes: true, Widgets: true}
	case apiv0.EventTagUpdated:
		if ev.Tag != nil {
			return ReloadTargets{}
		}
		return ReloadTargets{Tags: true}
	case apiv0.EventTagCreated, apiv0.EventTagDeleted:
		return ReloadTargets{Tags: true}
	}
	return ReloadTargets{}
}
