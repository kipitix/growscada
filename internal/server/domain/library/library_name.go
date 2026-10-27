package library

import "fmt"

// LibraryName - name of a library, unique among the live Libraries of its
// Organization.
// Value Object.
type LibraryName struct {
	name string
}

var _ fmt.Stringer = LibraryName{}

// NewLibraryName creates a LibraryName from a string.
func NewLibraryName(aName string) (LibraryName, error) {
	if aName == "" {
		return LibraryName{}, fmt.Errorf("library name must not be empty")
	}
	return LibraryName{name: aName}, nil
}

// String implements [fmt.Stringer].
func (n LibraryName) String() string { return n.name }
