package eventlog

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ReloadTargets says which server-backed lists an event makes stale.
// It is the single routing table of the parts that show server data (the
// project switcher of Root, Library, Project, Operation); each reloads the
// lists it shows. Libraries are the server's with the WidgetTypes of their
// Drafts (WidgetTypes); LibraryPins (the selected project's pins and the
// WidgetTypes of the releases they pin), Scenes, Widgets and Tags are those
// of the selected project.
type ReloadTargets struct {
	Projects, Libraries, WidgetTypes, LibraryPins, Scenes, Widgets, Tags bool
	// Scene is the one Scene whose own state and Widgets are stale (a Widget
	// of it changed, which bumps its version without a scene event); uuid.Nil
	// if none. The list of Scenes is not: only scene events and a resync
	// change it.
	Scene uuid.UUID
}

// WidgetsOf reports whether the Widgets of the Scene selected (uuid.Nil: none)
// are stale: those of any Scene, or of this one.
func (t ReloadTargets) WidgetsOf(selected uuid.UUID) bool {
	return t.Widgets || (t.Scene != uuid.Nil && t.Scene == selected)
}

// AllTargets is every list.
var AllTargets = ReloadTargets{Projects: true, Libraries: true, WidgetTypes: true, LibraryPins: true, Scenes: true, Widgets: true, Tags: true}

// The lists of each stream: what its resync reloads, since the events it
// published while disconnected are lost.
var (
	serverTargets  = ReloadTargets{Projects: true, Libraries: true, WidgetTypes: true}
	projectTargets = ReloadTargets{LibraryPins: true, Scenes: true, Widgets: true, Tags: true}
)

// ReloadTargetsFor routes an event of the streams (or EventResync,
// EventProjectResync) by its Type. tag_updated with the Tag's new state makes
// nothing stale: the modes apply the state themselves.
func ReloadTargetsFor(ev apiv0.Event) ReloadTargets {
	switch ev.Type {
	case EventResync:
		return serverTargets
	case EventProjectResync:
		return projectTargets
	case apiv0.EventProjectCreated, apiv0.EventProjectUpdated, apiv0.EventProjectDeleted:
		return ReloadTargets{Projects: true}
	case apiv0.EventWidgetTypeCreated, apiv0.EventWidgetTypeUpdated, apiv0.EventWidgetTypeDeleted:
		// A change of the Draft changes whether its library has changes
		// not released.
		return ReloadTargets{WidgetTypes: true, Libraries: true}
	case apiv0.EventLibraryCreated, apiv0.EventLibraryUpdated, apiv0.EventLibraryDeleted,
		apiv0.EventLibraryDiscardStarted, apiv0.EventLibraryReleased:
		// The end of a Discard (library_updated) changed the Draft.
		return ReloadTargets{Libraries: true, WidgetTypes: true}
	case apiv0.EventLibraryPinCreated, apiv0.EventLibraryPinUpdated, apiv0.EventLibraryPinDeleted:
		return ReloadTargets{LibraryPins: true}
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

// ApplyToTags returns tags with what ev tells of them by itself: the Tag's new
// state a tag_updated carries; on a resync of the project's stream, the
// seqs forgotten, since the server may have gone back. What ev makes
// stale is ReloadTargetsFor's.
func ApplyToTags(tags tagstate.Tags, ev apiv0.Event) tagstate.Tags {
	switch {
	case ev.Type == apiv0.EventTagUpdated && ev.Tag != nil:
		return tags.Apply(*ev.Tag)
	case ev.Type == EventProjectResync:
		return tags.ForgetSeqs()
	}
	return tags
}
