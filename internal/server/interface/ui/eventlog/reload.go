package eventlog

import (
	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ReloadTargets says which server-backed lists an event makes stale.
// It is the single routing table of the parts that show server data (the
// project switcher of Root, Library, Project); each reloads the lists it
// shows. Libraries are the server's with the WidgetTypes of their Drafts
// (WidgetTypes); LibraryPins (the selected project's pins and the
// WidgetTypes of the releases they pin), Scenes, Widgets and Tags are those
// of the selected project. Deploy is the Project mode's deployment state (the
// Deployment it runs and whether the Draft differs from it). Operation reads
// none of these: it follows the Journal from a Player of its own (ADR 0020).
type ReloadTargets struct {
	Projects, Libraries, WidgetTypes, LibraryPins, Scenes, Widgets, Tags, Deploy, Commits bool
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

// DraftChanged reports whether the event may have changed a Draft's DraftChange
// stack (ADR 0017): every write of its content is one change, and a Deploy or
// a Release clears the stack. Such an event is what the modes reread the stack
// on, alongside a resync; an event of the Draft's lifecycle alone (a Project or
// Library renamed or deleted) is not.
func (t ReloadTargets) DraftChanged() bool {
	return t.Tags || t.Scenes || t.Widgets || t.LibraryPins || t.WidgetTypes || t.Deploy
}

// AllTargets is every list.
var AllTargets = ReloadTargets{Projects: true, Libraries: true, WidgetTypes: true, LibraryPins: true, Scenes: true, Widgets: true, Tags: true, Deploy: true, Commits: true}

// The lists of each stream: what its resync reloads, since the events it
// published while disconnected are lost.
var (
	serverTargets  = ReloadTargets{Projects: true, Libraries: true, WidgetTypes: true, Commits: true}
	projectTargets = ReloadTargets{LibraryPins: true, Scenes: true, Widgets: true, Tags: true, Deploy: true, Commits: true}
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
	case apiv0.EventCommitCreated, apiv0.EventCommitTitleUpdated:
		// A Save or a rename changes the history alone: no content of the
		// Draft and no DraftChange stack (task 72_2; ADR 0021).
		return ReloadTargets{Commits: true}
	case apiv0.EventDeploymentCreated:
		// The Deployment the Project runs changed: Project refreshes its badge;
		// nothing of the Draft is touched. Operation does not read this stream
		// (ADR 0020).
		return ReloadTargets{Deploy: true}
	case apiv0.EventProjectCreated, apiv0.EventProjectUpdated, apiv0.EventProjectDeleted:
		// A Project was made, renamed or removed: the mode rereads the list.
		return ReloadTargets{Projects: true, Deploy: true}
	case apiv0.EventWidgetTypeCreated, apiv0.EventWidgetTypeUpdated, apiv0.EventWidgetTypeDeleted:
		// A change of the Draft changes whether its library has changes
		// not released.
		return ReloadTargets{WidgetTypes: true, Libraries: true}
	case apiv0.EventLibraryCreated, apiv0.EventLibraryUpdated, apiv0.EventLibraryDeleted,
		apiv0.EventLibraryReleased:
		return ReloadTargets{Libraries: true, WidgetTypes: true}
	case apiv0.EventLibraryPinCreated, apiv0.EventLibraryPinUpdated, apiv0.EventLibraryPinDeleted:
		return ReloadTargets{LibraryPins: true, Deploy: true}
	case apiv0.EventSceneCreated, apiv0.EventSceneUpdated, apiv0.EventSceneDeleted:
		return ReloadTargets{Scenes: true, Deploy: true}
	case apiv0.EventWidgetCreated, apiv0.EventWidgetUpdated, apiv0.EventWidgetDeleted:
		// The only place scene_id is read.
		if sceneID := uiutil.ParseID(ev.SceneID); sceneID != uuid.Nil {
			return ReloadTargets{Scene: sceneID, Deploy: true}
		}
		// Not knowing the Scene, every Scene (the widget change bumped its
		// version) and the shown Widgets may be stale.
		return ReloadTargets{Scenes: true, Widgets: true, Deploy: true}
	case apiv0.EventTagUpdated:
		if ev.Tag != nil {
			return ReloadTargets{}
		}
		return ReloadTargets{Tags: true}
	case apiv0.EventTagCreated, apiv0.EventTagDeleted:
		return ReloadTargets{Tags: true, Deploy: true}
	case apiv0.EventDeviceCreated, apiv0.EventDeviceUpdated, apiv0.EventDeviceDeleted,
		apiv0.EventEnvironmentCreated, apiv0.EventEnvironmentUpdated, apiv0.EventEnvironmentDeleted,
		apiv0.EventEnvironmentDeviceCreated, apiv0.EventEnvironmentDeviceUpdated, apiv0.EventEnvironmentDeviceDeleted:
		// The UI shows no Devices yet (task 66), but they are part of the
		// ProjectFile: the Project mode's deployment state may be stale.
		return ReloadTargets{Deploy: true}
	}
	// The Library events make nothing of a Project stale: a Project sees a
	// Library only through the Release it pins, whose switch is a pin event.
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
