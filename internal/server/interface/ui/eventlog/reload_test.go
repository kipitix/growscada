package eventlog

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/tagstate"
)

func TestReloadTargetsFor(t *testing.T) {
	sceneID := uuid.MustParse("00000000-0000-0000-0000-000000000003")
	tests := []struct {
		ev   apiv0.Event
		want ReloadTargets
	}{
		{apiv0.Event{Type: apiv0.EventWidgetTypeCreated}, ReloadTargets{WidgetTypes: true, Libraries: true}},
		{apiv0.Event{Type: apiv0.EventWidgetTypeUpdated}, ReloadTargets{WidgetTypes: true, Libraries: true}},
		{apiv0.Event{Type: apiv0.EventWidgetTypeDeleted}, ReloadTargets{WidgetTypes: true, Libraries: true}},
		{apiv0.Event{Type: apiv0.EventLibraryReleased}, ReloadTargets{Libraries: true, Releases: true, Commits: true}},
		{apiv0.Event{Type: apiv0.EventLibraryUpdated}, ReloadTargets{WidgetTypes: true, Libraries: true}},
		{apiv0.Event{Type: apiv0.EventLibraryPinUpdated}, ReloadTargets{LibraryPins: true}},
		{apiv0.Event{Type: apiv0.EventSceneCreated}, ReloadTargets{Scenes: true}},
		{apiv0.Event{Type: apiv0.EventSceneUpdated}, ReloadTargets{Scenes: true}},
		{apiv0.Event{Type: apiv0.EventSceneDeleted}, ReloadTargets{Scenes: true}},
		{apiv0.Event{Type: apiv0.EventWidgetCreated, SceneID: sceneID.String()}, ReloadTargets{Scene: sceneID}},
		{apiv0.Event{Type: apiv0.EventWidgetUpdated, SceneID: sceneID.String()}, ReloadTargets{Scene: sceneID}},
		{apiv0.Event{Type: apiv0.EventWidgetDeleted, SceneID: sceneID.String()}, ReloadTargets{Scene: sceneID}},
		// Without its Scene (an older server) or with one that does not parse.
		{apiv0.Event{Type: apiv0.EventWidgetUpdated}, ReloadTargets{Scenes: true, Widgets: true}},
		{apiv0.Event{Type: apiv0.EventWidgetDeleted, SceneID: "not-a-uuid"}, ReloadTargets{Scenes: true, Widgets: true}},
		{apiv0.Event{Type: apiv0.EventTagCreated}, ReloadTargets{Tags: true}},
		{apiv0.Event{Type: apiv0.EventTagUpdated, Tag: &apiv0.TagResponse{ID: sceneID}}, ReloadTargets{}},
		{apiv0.Event{Type: apiv0.EventTagUpdated}, ReloadTargets{Tags: true}},
		{apiv0.Event{Type: apiv0.EventTagDeleted}, ReloadTargets{Tags: true}},
		{apiv0.Event{Type: EventResync}, ReloadTargets{Projects: true, Libraries: true, WidgetTypes: true, Commits: true}},
		{apiv0.Event{Type: EventProjectResync}, ReloadTargets{LibraryPins: true, Scenes: true, Widgets: true, Tags: true, Deploy: true, Commits: true, Releases: true}},
		{apiv0.Event{Type: apiv0.EventProjectCreated}, ReloadTargets{Projects: true, Deploy: true}},
		{apiv0.Event{Type: apiv0.EventProjectUpdated}, ReloadTargets{Projects: true, Deploy: true}},
		{apiv0.Event{Type: apiv0.EventProjectDeleted}, ReloadTargets{Projects: true, Deploy: true}},
		// The UI shows no Devices, Environments and pairs yet (task 66): a
		// write of the Draft changes no list, and the Deployment it runs
		// changes on deployment_created alone (task 72_4).
		{apiv0.Event{Type: apiv0.EventDeviceCreated}, ReloadTargets{}},
		{apiv0.Event{Type: apiv0.EventEnvironmentUpdated}, ReloadTargets{}},
		{apiv0.Event{Type: apiv0.EventEnvironmentDeviceDeleted}, ReloadTargets{}},
		{apiv0.Event{Type: apiv0.EventDeploymentCreated}, ReloadTargets{Deploy: true}},
		{apiv0.Event{Type: apiv0.EventCommitCreated}, ReloadTargets{Commits: true}},
		{apiv0.Event{Type: apiv0.EventCommitTitleUpdated}, ReloadTargets{Commits: true}},
		{apiv0.Event{Type: apiv0.EventReleaseCreated}, ReloadTargets{Releases: true, Commits: true}},
		{apiv0.Event{Type: apiv0.EventReleaseNoteUpdated}, ReloadTargets{Releases: true, Commits: true}},
		{apiv0.Event{Type: apiv0.EventClientConnected}, ReloadTargets{}},

		{apiv0.Event{Type: apiv0.EventSystemReady}, ReloadTargets{}},
		{apiv0.Event{Type: "unknown"}, ReloadTargets{}},
		{apiv0.Event{}, ReloadTargets{}},
	}
	for _, tt := range tests {
		if got := ReloadTargetsFor(tt.ev); got != tt.want {
			t.Errorf("ReloadTargetsFor(%+v) = %+v, want %+v", tt.ev, got, tt.want)
		}
	}
}

// notShownYet are the event types that change no server-backed list of the
// UI: the modes apply them themselves (Operation reads its own Journal, ADR
// 0020), or they belong to content it does not show yet (task 66 shows
// Devices, Environments and their pairs).
var notShownYet = map[string]bool{
	apiv0.EventDeviceCreated:            true,
	apiv0.EventDeviceUpdated:            true,
	apiv0.EventDeviceDeleted:            true,
	apiv0.EventEnvironmentCreated:       true,
	apiv0.EventEnvironmentUpdated:       true,
	apiv0.EventEnvironmentDeleted:       true,
	apiv0.EventEnvironmentDeviceCreated: true,
	apiv0.EventEnvironmentDeviceUpdated: true,
	apiv0.EventEnvironmentDeviceDeleted: true,
}

// TestReloadTargetsFor_CoversEveryKnownEvent guards the table against a new
// aggregate event type added to the event log but not routed.
func TestReloadTargetsFor_CoversEveryKnownEvent(t *testing.T) {
	for eventType, info := range eventTypeInfoByType {
		if info.category == categoryClient || eventType == apiv0.EventSystemReady || notShownYet[eventType] {
			continue // change no server-backed list
		}
		ev := apiv0.Event{Type: eventType}
		if eventType == apiv0.EventTagUpdated {
			// With the Tag's state the modes apply it instead of reloading;
			// without it, the Tags are reread.
			if got := ReloadTargetsFor(apiv0.Event{Type: eventType, Tag: &apiv0.TagResponse{}}); got != (ReloadTargets{}) {
				t.Errorf("%q with its Tag reloads %+v", eventType, got)
			}
		}
		if ReloadTargetsFor(ev) == (ReloadTargets{}) {
			t.Errorf("%q reloads nothing", eventType)
		}
	}
}

// TestReloadTargets_DraftChanged: the events that write a Draft's content ask
// for a reread of the stack (ADR 0017); a Tag's value, a Deploy and a Release
// change no Draft and leave the stack alone (task 72_4).
func TestReloadTargets_DraftChanged(t *testing.T) {
	changes := []apiv0.Event{
		{Type: apiv0.EventTagCreated},
		{Type: apiv0.EventTagDeleted},
		{Type: apiv0.EventSceneUpdated},
		{Type: apiv0.EventWidgetCreated},
		{Type: apiv0.EventLibraryPinDeleted},
		{Type: apiv0.EventWidgetTypeUpdated},
		{Type: EventResync},
	}
	for _, ev := range changes {
		if !ReloadTargetsFor(ev).DraftChanged() {
			t.Errorf("%q leaves the stack alone", ev.Type)
		}
	}
	unchanged := []apiv0.Event{
		{Type: apiv0.EventTagUpdated, Tag: &apiv0.TagResponse{}}, // a value, not a DraftChange
		{Type: apiv0.EventDeploymentCreated},                     // a Deploy takes a Release (task 72_4)
		{Type: apiv0.EventReleaseCreated},                        // a Release copies a Commit
		{Type: apiv0.EventLibraryReleased},                       // a Library Release copies a Commit (task 72_5)
		{Type: apiv0.EventDeviceCreated},                         // no Device list yet (task 66)
		{Type: apiv0.EventClientConnected},
		{Type: apiv0.EventSystemReady},
	}
	for _, ev := range unchanged {
		if ReloadTargetsFor(ev).DraftChanged() {
			t.Errorf("%q rereads the stack", ev.Type)
		}
	}
}

func TestReloadTargets_WidgetsOf(t *testing.T) {
	selected, other := uuid.New(), uuid.New()

	for name, tc := range map[string]struct {
		targets  ReloadTargets
		selected uuid.UUID
		want     bool
	}{
		"every Scene's":      {ReloadTargets{Widgets: true}, selected, true},
		"the selected one's": {ReloadTargets{Scene: selected}, selected, true},
		"another Scene's":    {ReloadTargets{Scene: other}, selected, false},
		"none selected":      {ReloadTargets{}, uuid.Nil, false},
		"no Widgets":         {ReloadTargets{Scenes: true}, selected, false},
	} {
		if got := tc.targets.WidgetsOf(tc.selected); got != tc.want {
			t.Errorf("%s: got %v, want %v", name, got, tc.want)
		}
	}
}

func TestApplyToTags(t *testing.T) {
	id := uuid.New()
	known := tagstate.Tags{}.Merge(0, []apiv0.TagResponse{{ID: id, Value: "1", Seq: 5}})
	value := func(ts tagstate.Tags) string { tag, _ := ts.Find(id); return tag.Value }

	updated := ApplyToTags(known, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String(), Tag: &apiv0.TagResponse{ID: id, Value: "2", Seq: 6}})
	if value(updated) != "2" {
		t.Errorf("tag_updated: got %q, want its state", value(updated))
	}
	if got := ApplyToTags(known, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String()}); value(got) != "1" {
		t.Errorf("tag_updated without a state: got %q, want the Tags as they were", value(got))
	}
	forgot := ApplyToTags(updated, apiv0.Event{Type: EventProjectResync})
	if got := ApplyToTags(forgot, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String(), Tag: &apiv0.TagResponse{ID: id, Value: "fresh", Seq: 1}}); value(got) != "fresh" {
		t.Errorf("after a resync of the project's stream: got %q, want a state of the started-over seqs", value(got))
	}
	// The server's stream carries no Tags: its resync leaves them as they are.
	if got := ApplyToTags(updated, apiv0.Event{Type: EventResync}); value(ApplyToTags(got, apiv0.Event{Type: apiv0.EventTagUpdated, ID: id.String(), Tag: &apiv0.TagResponse{ID: id, Value: "old", Seq: 1}})) != "2" {
		t.Errorf("after a resync of the server's stream the seqs must be kept")
	}
	if got := ApplyToTags(known, apiv0.Event{Type: apiv0.EventSceneCreated}); value(got) != "1" {
		t.Errorf("another event: got %q", value(got))
	}
}
