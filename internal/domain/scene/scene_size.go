package scene

import "fmt"

// SceneSize - dimensions of the scene canvas in pixels.
// Value Object.
type SceneSize struct {
	width  int
	height int
}

var _ fmt.Stringer = SceneSize{}

// NewSceneSize creates a SceneSize value object.
func NewSceneSize(width, height int) (SceneSize, error) {
	if width <= 0 {
		return SceneSize{}, fmt.Errorf("scene width must be positive, got %d", width)
	}
	if height <= 0 {
		return SceneSize{}, fmt.Errorf("scene height must be positive, got %d", height)
	}
	return SceneSize{width: width, height: height}, nil
}

func (s SceneSize) Width() int  { return s.width }
func (s SceneSize) Height() int { return s.height }

// String implements [fmt.Stringer].
func (s SceneSize) String() string {
	return fmt.Sprintf("%dx%d", s.width, s.height)
}
