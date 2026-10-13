package project

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestWidgetRequest_IsASnapshot(t *testing.T) {
	tagID := uuid.New()
	w := apiv0.WidgetResponse{
		Labels:       []string{"a"},
		PortBindings: []apiv0.PortBinding{{PortName: "value", TagID: tagID}},
	}
	req := widgetRequest(w, 1)

	// bindPort and the like change the widget in place before the body is sent.
	w.Labels[0] = "b"
	w.PortBindings[0].TagID = uuid.New()

	if req.Labels[0] != "a" || req.PortBindings[0].TagID != tagID {
		t.Errorf("the request follows the widget: %+v", req)
	}
	if empty := widgetRequest(apiv0.WidgetResponse{}, 1); empty.Labels == nil || empty.PortBindings == nil {
		t.Errorf("nil slices are sent as null: %+v", empty)
	}
}
