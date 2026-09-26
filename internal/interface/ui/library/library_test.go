package library

import "testing"

func TestIsWidgetTypeEvent(t *testing.T) {
	for eventType, want := range map[string]bool{
		"widget_type_created": true,
		"widget_type_updated": true,
		"widget_type_deleted": true,
		"widget_created":      false,
		"tag_updated":         false,
		"":                    false,
	} {
		if got := isWidgetTypeEvent(eventType); got != want {
			t.Errorf("isWidgetTypeEvent(%q) = %v, want %v", eventType, got, want)
		}
	}
}
