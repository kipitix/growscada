package label

import "errors"

var (
	// ErrLabelNotFound - the label does not exist in the Organization.
	ErrLabelNotFound = errors.New("label not found")
	// ErrLabelNameTaken - another label of the Organization holds the same
	// name, ignoring case.
	ErrLabelNameTaken = errors.New("label name is already taken")
	// ErrInvalidLabelName - the name is empty, too long or holds a line break.
	ErrInvalidLabelName = errors.New("invalid label name")
	// ErrInvalidLabelColor - the color is not #RRGGBB and not empty.
	ErrInvalidLabelColor = errors.New("invalid label color")
)
