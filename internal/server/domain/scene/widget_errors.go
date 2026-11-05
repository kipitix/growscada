package scene

import "errors"

var (
	// ErrWidgetNotFound is returned when a widget cannot be located within its scene.
	ErrWidgetNotFound = errors.New("widget not found")
	// ErrWidgetAlreadyExists is returned when a widget is added to a scene
	// that already holds a widget with the same ID.
	ErrWidgetAlreadyExists = errors.New("widget already exists")
	// ErrWidgetTypeMismatch is returned when a widget is checked against a
	// WidgetType other than the one its TypeID names.
	ErrWidgetTypeMismatch = errors.New("widget type mismatch")
	// ErrPortNotDeclared is returned when a widget binds a port its
	// WidgetType does not declare.
	ErrPortNotDeclared = errors.New("port is not declared on widget type")
	// ErrBoundTagNotFound is returned when a widget binds a port to a Tag
	// that does not exist.
	ErrBoundTagNotFound = errors.New("bound tag not found")
	// ErrPortTypeMismatch is returned when a widget binds a port to a Tag
	// whose TagType the port's type hint does not accept.
	ErrPortTypeMismatch = errors.New("port type hint does not accept tag type")
	// ErrWidgetMovedTwice is returned when a Scene.MoveWidgets request names
	// the same widget more than once.
	ErrWidgetMovedTwice = errors.New("widget is moved twice")
	// ErrNothingToMove is returned when a Scene.MoveWidgets request moves no
	// widget: every widget it names already holds the position it is moved to.
	ErrNothingToMove = errors.New("nothing to move")
)
