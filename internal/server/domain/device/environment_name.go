package device

import "fmt"

// EnvironmentName - name of an Environment, unique among the live
// Environments of its Project. A Device Gateway names the Environment it
// serves, and the Runtime node resolves that name to a pair with a Device.
// Value Object.
type EnvironmentName struct {
	name string
}

var _ fmt.Stringer = EnvironmentName{}

// NewEnvironmentName creates an EnvironmentName from a string.
func NewEnvironmentName(aName string) (EnvironmentName, error) {
	if aName == "" {
		return EnvironmentName{}, fmt.Errorf("environment name must not be empty")
	}
	return EnvironmentName{name: aName}, nil
}

// IsValid reports whether the name is not the zero value: an Environment
// without a name cannot be stored.
func (n EnvironmentName) IsValid() bool { return n.name != "" }

// String implements [fmt.Stringer].
func (n EnvironmentName) String() string { return n.name }
