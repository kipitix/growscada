package restapi

import (
	"testing"
	"time"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/domain/client"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/interface/eventbus"
)

// TestNewAPIEvent_TranslatesEveryEventType checks newAPIEvent on a sample of
// every domain EventType: a type without a sample fails the test, so a new
// event cannot reach the stream untranslated.
func TestNewAPIEvent_TranslatesEveryEventType(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 30, 0, 0, time.UTC)
	ts := event.WithTimestamp(event.NewEventTimestamp(event.EventTimestampWithTime(at)))
	const timestamp = "2026-10-15T12:30:00Z"

	tagID := id.NewID[tag.Tag]()
	widgetID := id.NewID[scene.Widget]()
	sceneID := id.NewID[scene.Scene]()
	widgetTypeID := id.NewID[library.WidgetType]()
	clientID := id.NewID[client.Client]()

	name, _ := tag.NewTagName("pressure")
	value, _ := tag.TagTypeInteger.NewTagValue(int64(42))
	tagVersion, _ := version.New(version.WithNumber[tag.Tag](7))
	updatedTag, err := tag.ReconstituteTag(tagID, name, tag.TagTypeInteger, value, tag.TagQualityGood, tagVersion)
	if err != nil {
		t.Fatalf("reconstitute tag: %v", err)
	}

	samples := map[event.EventType]struct {
		event event.Event
		want  apiv0.Event
	}{
		event.EventTypeSystemReady: {eventbus.NewSystemReadyEvent(ts),
			apiv0.Event{Type: "system_ready"}},
		event.EventTypeTagCreated: {tag.NewTagCreatedEvent(tagID, ts),
			apiv0.Event{Type: "tag_created", ID: tagID.String()}},
		event.EventTypeTagUpdated: {tag.NewTagUpdatedEvent(updatedTag, ts),
			apiv0.Event{Type: "tag_updated", ID: tagID.String(), Tag: &apiv0.TagResponse{
				ID: tagID.UUID(), Name: "pressure", Type: "integer", Value: "42", Quality: "good", Version: 7,
			}}},
		event.EventTypeTagDeleted: {tag.NewTagDeletedEvent(tagID, ts),
			apiv0.Event{Type: "tag_deleted", ID: tagID.String()}},
		event.EventTypeWidgetTypeCreated: {library.NewWidgetTypeCreatedEvent(widgetTypeID, ts),
			apiv0.Event{Type: "widget_type_created", ID: widgetTypeID.String()}},
		event.EventTypeWidgetTypeUpdated: {library.NewWidgetTypeUpdatedEvent(widgetTypeID, ts),
			apiv0.Event{Type: "widget_type_updated", ID: widgetTypeID.String()}},
		event.EventTypeWidgetTypeDeleted: {library.NewWidgetTypeDeletedEvent(widgetTypeID, ts),
			apiv0.Event{Type: "widget_type_deleted", ID: widgetTypeID.String()}},
		event.EventTypeWidgetCreated: {scene.NewWidgetCreatedEvent(widgetID, sceneID, widgetTypeID, ts),
			apiv0.Event{Type: "widget_created", ID: widgetID.String(), SceneID: sceneID.String()}},
		event.EventTypeWidgetUpdated: {scene.NewWidgetUpdatedEvent(widgetID, sceneID, widgetTypeID, ts),
			apiv0.Event{Type: "widget_updated", ID: widgetID.String(), SceneID: sceneID.String()}},
		event.EventTypeWidgetDeleted: {scene.NewWidgetDeletedEvent(widgetID, sceneID, ts),
			apiv0.Event{Type: "widget_deleted", ID: widgetID.String(), SceneID: sceneID.String()}},
		event.EventTypeSceneCreated: {scene.NewSceneCreatedEvent(sceneID, ts),
			apiv0.Event{Type: "scene_created", ID: sceneID.String()}},
		event.EventTypeSceneUpdated: {scene.NewSceneUpdatedEvent(sceneID, ts),
			apiv0.Event{Type: "scene_updated", ID: sceneID.String()}},
		event.EventTypeSceneDeleted: {scene.NewSceneDeletedEvent(sceneID, ts),
			apiv0.Event{Type: "scene_deleted", ID: sceneID.String()}},
		event.EventTypeClientConnected: {eventbus.NewClientConnectedEvent(clientID, ts),
			apiv0.Event{Type: "client_connected", ID: clientID.String()}},
		event.EventTypeClientDisconnected: {eventbus.NewClientDisconnectedEvent(clientID, ts),
			apiv0.Event{Type: "client_disconnected", ID: clientID.String()}},
	}

	for _, et := range event.AllEventTypes() {
		t.Run(et.String(), func(t *testing.T) {
			sample, ok := samples[et]
			if !ok {
				t.Fatalf("no sample of %s: add one, and its translation to apiEventTypes", et)
			}
			if sample.event.Type() != et {
				t.Fatalf("the sample is a %s", sample.event.Type())
			}
			want := sample.want
			want.Timestamp = timestamp

			got := newAPIEvent(sample.event)

			if got.Tag == nil || want.Tag == nil {
				if got.Tag != want.Tag {
					t.Errorf("Tag: got %+v, want %+v", got.Tag, want.Tag)
				}
			} else if *got.Tag != *want.Tag {
				t.Errorf("Tag: got %+v, want %+v", *got.Tag, *want.Tag)
			}
			got.Tag, want.Tag = nil, nil
			if got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
}

// TestAPIEventTypes_UsesEveryContractEventType guards the table from the
// contract's side: a published name nothing translates to is never sent.
func TestAPIEventTypes_UsesEveryContractEventType(t *testing.T) {
	used := make(map[string]bool, len(apiEventTypes))
	for _, name := range apiEventTypes {
		if used[name] {
			t.Errorf("%q is the translation of two event types", name)
		}
		used[name] = true
	}
	for _, name := range apiv0.EventTypes() {
		if !used[name] {
			t.Errorf("contract event type %q is not in apiEventTypes", name)
		}
	}
}
