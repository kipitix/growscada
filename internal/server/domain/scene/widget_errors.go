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
)
