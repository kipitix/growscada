package project

import "testing"

func TestDragChangedWidget_SavesOnlyAMovedAndChangedWidget(t *testing.T) {
	start := widgetItem{ID: "w", Position: positionDTO{X: 10, Y: 20}, Size: sizeDTO{Width: 100, Height: 50}, Rotation: rotationDTO{Degrees: 0}}
	moved := start
	moved.Position.X = 15
	rotated := start
	rotated.Rotation.Degrees = 30

	cases := map[string]struct {
		didMove bool
		now     widgetItem
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
			p := &Project{dragDidMove: tc.didMove, dragStartGeometry: start.geometry()}
			if got := p.dragChangedWidget(tc.now); got != tc.want {
				t.Errorf("dragChangedWidget: got %v, want %v", got, tc.want)
			}
		})
	}
}
