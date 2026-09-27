package scene

import "fmt"

// SceneName - name of a scene.
// Value Object.
type SceneName struct {
	name string
}

var _ fmt.Stringer = SceneName{}

// NewSceneName creates a SceneName from a string.
func NewSceneName(aName string) (SceneName, error) {
	if aName == "" {
		return SceneName{}, fmt.Errorf("scene name must not be empty")
	}
	return SceneName{name: aName}, nil
}

// String implements [fmt.Stringer].
func (n SceneName) String() string { return n.name }
