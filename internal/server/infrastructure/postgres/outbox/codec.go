// Package outbox stores the domain events of committed changes and delivers
// them after commit (ADR 0008). A repository appends an aggregate's pending
// events with Append in the transaction that saves the aggregate; the
// Dispatcher reads them back in commit order, hands each to its sink and
// deletes it.
package outbox

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// payload is the stored form of an event; the event type is stored beside it.
// It is internal to the server, not a contract (ADR 0005): a row lives
// seconds, and a change of this format must keep decoding the previous one
// until the outbox has drained.
type payload struct {
	Timestamp time.Time `json:"timestamp"`
	// ID is the ID of the aggregate (or Widget) the event is about.
	ID uuid.UUID `json:"id"`
	// SceneID is the Scene holding the Widget of a Widget event.
	SceneID *uuid.UUID `json:"scene_id,omitempty"`
	// Tag is the new state of the Tag of a tag_updated event.
	Tag *tagState `json:"tag,omitempty"`
}

type tagState struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Quality string `json:"quality"`
	Version int    `json:"version"`
}

// Encode returns the stored form of a domain event. Events of delivery
// (connection lifecycle, system) are never stored and fail to encode.
func Encode(e event.Event) ([]byte, error) {
	p := payload{Timestamp: e.Timestamp().Time()}
	switch ev := e.(type) {
	case tag.TagUpdatedEvent:
		p.ID = ev.TagID().UUID()
		t := ev.Tag()
		p.Tag = &tagState{
			Name:    t.Name().String(),
			Type:    t.Type().String(),
			Value:   t.Value().String(),
			Quality: t.Quality().String(),
			Version: t.Version().Number(),
		}
	case tag.TagEvent:
		p.ID = ev.TagID().UUID()
	case library.WidgetTypeEvent:
		p.ID = ev.WidgetTypeID().UUID()
	case scene.WidgetEvent:
		p.ID = ev.WidgetID().UUID()
		sceneID := ev.SceneID().UUID()
		p.SceneID = &sceneID
	case scene.SceneEvent:
		p.ID = ev.SceneID().UUID()
	default:
		return nil, fmt.Errorf("cannot encode event %s: not a domain event", e.Type())
	}
	return json.Marshal(p)
}

// Decode rebuilds the event of the given type from its stored form.
func Decode(eventType event.EventType, data []byte) (event.Event, error) {
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("cannot decode %s payload: %w", eventType, err)
	}
	ts := event.WithTimestamp(event.NewEventTimestamp(event.EventTimestampWithTime(p.Timestamp)))

	tagID := id.NewID(id.IDWithUUID[tag.Tag](p.ID))
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](p.ID))
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](p.ID))
	widgetID := id.NewID(id.IDWithUUID[scene.Widget](p.ID))

	switch eventType {
	case event.EventTypeTagCreated:
		return tag.NewTagCreatedEvent(tagID, ts), nil
	case event.EventTypeTagUpdated:
		t, err := decodeTagState(tagID, p.Tag)
		if err != nil {
			return nil, err
		}
		return tag.NewTagUpdatedEvent(t, ts), nil
	case event.EventTypeTagDeleted:
		return tag.NewTagDeletedEvent(tagID, ts), nil
	case event.EventTypeWidgetTypeCreated:
		return library.NewWidgetTypeCreatedEvent(widgetTypeID, ts), nil
	case event.EventTypeWidgetTypeUpdated:
		return library.NewWidgetTypeUpdatedEvent(widgetTypeID, ts), nil
	case event.EventTypeWidgetTypeDeleted:
		return library.NewWidgetTypeDeletedEvent(widgetTypeID, ts), nil
	case event.EventTypeSceneCreated:
		return scene.NewSceneCreatedEvent(sceneID, ts), nil
	case event.EventTypeSceneUpdated:
		return scene.NewSceneUpdatedEvent(sceneID, ts), nil
	case event.EventTypeSceneDeleted:
		return scene.NewSceneDeletedEvent(sceneID, ts), nil
	}

	// Widget events: the Scene that holds the widget is stored beside it.
	if p.SceneID == nil {
		return nil, fmt.Errorf("cannot decode %s payload: no scene_id", eventType)
	}
	holder := id.NewID(id.IDWithUUID[scene.Scene](*p.SceneID))
	switch eventType {
	case event.EventTypeWidgetCreated:
		return scene.NewWidgetCreatedEvent(widgetID, holder, ts), nil
	case event.EventTypeWidgetUpdated:
		return scene.NewWidgetUpdatedEvent(widgetID, holder, ts), nil
	case event.EventTypeWidgetDeleted:
		return scene.NewWidgetDeletedEvent(widgetID, holder, ts), nil
	}
	return nil, fmt.Errorf("cannot decode event of type %s: not a domain event", eventType)
}

func decodeTagState(tagID id.ID[tag.Tag], s *tagState) (tag.Tag, error) {
	if s == nil {
		return nil, fmt.Errorf("cannot decode tag_updated payload: no tag")
	}
	name, err := tag.NewTagName(s.Name)
	if err != nil {
		return nil, fmt.Errorf("cannot decode tag name: %w", err)
	}
	tagType, err := tag.NewTagType(s.Type)
	if err != nil {
		return nil, fmt.Errorf("cannot decode tag type: %w", err)
	}
	value, err := tagType.NewTagValue(s.Value)
	if err != nil {
		return nil, fmt.Errorf("cannot decode tag value: %w", err)
	}
	quality, err := tag.NewTagQuality(s.Quality)
	if err != nil {
		return nil, fmt.Errorf("cannot decode tag quality: %w", err)
	}
	ver, err := version.New(version.WithNumber[tag.Tag](s.Version))
	if err != nil {
		return nil, fmt.Errorf("cannot decode tag version: %w", err)
	}
	return tag.ReconstituteTag(tagID, name, tagType, value, quality, ver)
}
