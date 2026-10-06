package outbox_test

import (
	"testing"
	"time"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
	"github.com/kipitix/growscada/internal/server/infrastructure/postgres/outbox"
)

// deliveryEventTypes are the event types of delivery, never stored.
var deliveryEventTypes = map[event.EventType]bool{
	event.EventTypeSystemReady:        true,
	event.EventTypeClientConnected:    true,
	event.EventTypeClientDisconnected: true,
}

// sampleEvents returns one event of every domain event type.
func sampleEvents(t *testing.T) []event.Event {
	t.Helper()
	ts := event.WithTimestamp(event.NewEventTimestamp(
		event.EventTimestampWithTime(time.Date(2026, 10, 6, 12, 30, 15, 123456789, time.UTC))))

	name, _ := tag.NewTagName("boiler.pressure")
	value, _ := tag.TagTypeInteger.NewTagValue(int64(-42))
	ver, _ := version.New(version.WithNumber[tag.Tag](7))
	aTag, err := tag.ReconstituteTag(id.NewID[tag.Tag](), name, tag.TagTypeInteger, value, tag.TagQualityUncertain, ver)
	if err != nil {
		t.Fatalf("ReconstituteTag: %v", err)
	}

	tagID := id.NewID[tag.Tag]()
	wtID := id.NewID[library.WidgetType]()
	sceneID := id.NewID[scene.Scene]()
	widgetID := id.NewID[scene.Widget]()
	return []event.Event{
		tag.NewTagCreatedEvent(tagID, ts),
		tag.NewTagUpdatedEvent(aTag, ts),
		tag.NewTagDeletedEvent(tagID, ts),
		library.NewWidgetTypeCreatedEvent(wtID, ts),
		library.NewWidgetTypeUpdatedEvent(wtID, ts),
		library.NewWidgetTypeDeletedEvent(wtID, ts),
		scene.NewSceneCreatedEvent(sceneID, ts),
		scene.NewSceneUpdatedEvent(sceneID, ts),
		scene.NewSceneDeletedEvent(sceneID, ts),
		scene.NewWidgetCreatedEvent(widgetID, sceneID, ts),
		scene.NewWidgetUpdatedEvent(widgetID, sceneID, ts),
		scene.NewWidgetDeletedEvent(widgetID, sceneID, ts),
	}
}

func TestCodec_SamplesCoverEveryDomainEventType(t *testing.T) {
	covered := map[event.EventType]bool{}
	for _, e := range sampleEvents(t) {
		covered[e.Type()] = true
	}
	for _, et := range event.AllEventTypes() {
		if !deliveryEventTypes[et] && !covered[et] {
			t.Errorf("no sample of %s: add one, and teach the codec to store it", et)
		}
	}
}

func TestCodec_EveryEventSurvivesRoundTrip(t *testing.T) {
	for _, original := range sampleEvents(t) {
		t.Run(original.Type().String(), func(t *testing.T) {
			data, err := outbox.Encode(original)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}
			decoded, err := outbox.Decode(original.Type(), data)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if decoded.Type() != original.Type() {
				t.Errorf("type %s, want %s", decoded.Type(), original.Type())
			}
			if !decoded.Timestamp().Time().Equal(original.Timestamp().Time()) {
				t.Errorf("timestamp %s, want %s", decoded.Timestamp(), original.Timestamp())
			}
			assertSameSubject(t, original, decoded)
		})
	}
}

func assertSameSubject(t *testing.T, original, decoded event.Event) {
	t.Helper()
	switch want := original.(type) {
	case tag.TagUpdatedEvent:
		got := decoded.(tag.TagUpdatedEvent)
		w, g := want.Tag(), got.Tag()
		if g.ID() != w.ID() || g.Name() != w.Name() || g.Type() != w.Type() ||
			g.Value().String() != w.Value().String() || g.Quality() != w.Quality() || g.Version() != w.Version() {
			t.Errorf("tag %s, want %s", g, w)
		}
	case tag.TagEvent:
		if got := decoded.(tag.TagEvent); got.TagID() != want.TagID() {
			t.Errorf("tag ID %s, want %s", got.TagID(), want.TagID())
		}
	case library.WidgetTypeEvent:
		if got := decoded.(library.WidgetTypeEvent); got.WidgetTypeID() != want.WidgetTypeID() {
			t.Errorf("widget type ID %s, want %s", got.WidgetTypeID(), want.WidgetTypeID())
		}
	case scene.WidgetEvent:
		got := decoded.(scene.WidgetEvent)
		if got.WidgetID() != want.WidgetID() || got.SceneID() != want.SceneID() {
			t.Errorf("widget %s in scene %s, want %s in %s", got.WidgetID(), got.SceneID(), want.WidgetID(), want.SceneID())
		}
	case scene.SceneEvent:
		if got := decoded.(scene.SceneEvent); got.SceneID() != want.SceneID() {
			t.Errorf("scene ID %s, want %s", got.SceneID(), want.SceneID())
		}
	default:
		t.Fatalf("unexpected event %T", original)
	}
}

func TestCodec_DeliveryEventTypes_AreNotDecoded(t *testing.T) {
	for et := range deliveryEventTypes {
		if _, err := outbox.Decode(et, []byte(`{"id":"00000000-0000-0000-0000-000000000000"}`)); err == nil {
			t.Errorf("expected %s to be refused", et)
		}
	}
}
