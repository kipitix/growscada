package project

import (
	"testing"

	"github.com/google/uuid"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

func TestDragChangedWidget_SavesOnlyAMovedAndChangedWidget(t *testing.T) {
	start := apiv0.WidgetResponse{ID: uuid.New(), Position: apiv0.PositionResponse{X: 10, Y: 20}, Size: apiv0.SizeResponse{Width: 100, Height: 50}}
	moved := start
	moved.Position.X = 15
	rotated := start
	rotated.Rotation.Degrees = 30

	cases := map[string]struct {
		didMove bool
		now     apiv0.WidgetResponse
		want    bool
	}{
		"click without a move":      {false, start, false},
		"moved back to the start":   {true, start, false},
		"moved to a new position":   {true, moved, true},
		"rotated":                   {true, rotated, true},
		"no move, geometry differs": {false, moved, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p := &Project{dragDidMove: tc.didMove, dragStartGeometry: geometryOf(start)}
			if got := p.dragChangedWidget(tc.now); got != tc.want {
				t.Errorf("dragChangedWidget: got %v, want %v", got, tc.want)
			}
		})
	}
}
