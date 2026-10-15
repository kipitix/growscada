package apiclient_test

import (
	"testing"

	"github.com/kipitix/growscada/internal/apiclient"
)

func TestParseEvent_ReadsTolerantly(t *testing.T) {
	ev, ok := apiclient.ParseEvent(`{"type":"tag_updated","timestamp":"2026-10-13T10:00:00Z",
		"id":"00000000-0000-0000-0000-000000000001","origin":"device",
		"tag":{"id":"00000000-0000-0000-0000-000000000001","name":"speed","type":"integer",
		"value":"42","quality":"good","version":3,"unit":"rpm"}}`)
	if !ok || ev.Type != "tag_updated" || ev.Timestamp != "2026-10-13T10:00:00Z" ||
		ev.Tag == nil || ev.Tag.Value != "42" || ev.Tag.Version != 3 {
		t.Fatalf("got %+v, %v", ev, ok)
	}

	if ev, ok := apiclient.ParseEvent(`{"type":"scene_created","id":"x"}`); !ok || ev.ID != "x" || ev.Tag != nil {
		t.Errorf("an event without a Tag: got %+v, %v", ev, ok)
	}
	if ev, ok := apiclient.ParseEvent(`{"type":"tag_updated","id":"x","tag":null}`); !ok || ev.Tag != nil {
		t.Errorf("a null Tag: got %+v, %v", ev, ok)
	}
	ev, ok = apiclient.ParseEvent(`{"type":"tag_updated","timestamp":"t","id":"x","scene_id":"s","tag":{"id":"","version":"3"}}`)
	if !ok || ev.Type != "tag_updated" || ev.Timestamp != "t" || ev.ID != "x" || ev.SceneID != "s" || ev.Tag != nil {
		t.Errorf("a Tag that does not fit: got %+v, %v; want the event without it", ev, ok)
	}
	for _, bad := range []string{``, `not json`, `{}`, `[]`, `"tag_created"`, `{"type":3}`,
		`{"type":"widget_updated","scene_id":7}`, `{"type":"scene_created","id":{}}`} {
		if _, ok := apiclient.ParseEvent(bad); ok {
			t.Errorf("ParseEvent(%q) should fail", bad)
		}
	}
}

func TestParseEvent_ReadsTheSceneOfAWidget(t *testing.T) {
	ev, ok := apiclient.ParseEvent(`{"type":"widget_updated","timestamp":"2026-10-15T10:00:00Z",
		"id":"00000000-0000-0000-0000-000000000002","scene_id":"00000000-0000-0000-0000-000000000003"}`)
	if !ok || ev.Type != "widget_updated" || ev.ID != "00000000-0000-0000-0000-000000000002" ||
		ev.SceneID != "00000000-0000-0000-0000-000000000003" {
		t.Errorf("got %+v, %v", ev, ok)
	}
	if ev, ok := apiclient.ParseEvent(`{"type":"widget_deleted","id":"w"}`); !ok || ev.SceneID != "" {
		t.Errorf("a widget event without scene_id: got %+v, %v", ev, ok)
	}
}
