// Package livescene renders a Scene with live Tag values: the Widgets'
// iframes, the Scene background and a Quality marker over each Widget. It is
// given the whole state and loads nothing itself, so the same view serves
// Operation (state from the REST API and SSE) and, later, the History Player.
package livescene

import "github.com/kipitix/growscada/internal/server/interface/ui/uidto"

// Scene is a Scene as the REST API returns it.
type Scene struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
	Version        int    `json:"version"`
}

// WidgetType is the part of a WidgetType the view needs.
type WidgetType struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	HtmlTemplate string               `json:"html_template"`
	Script       string               `json:"script"`
	InputPorts   []uidto.InputPortDTO `json:"input_ports"`
}

// PortBinding links one of a Widget's InputPorts to the Tag that feeds it.
type PortBinding struct {
	PortName string `json:"port_name"`
	TagID    string `json:"tag_id"`
}

// Position is a Widget's position on the Scene; Z orders overlapping Widgets.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z int     `json:"z"`
}

// Size is a Widget's size in Scene pixels.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// TransformMatrix is a Widget's placement (position, origin and rotation)
// as a CSS transform computed by the server.
type TransformMatrix struct {
	CSS string `json:"css"`
}

// Widget is the part of a Widget the view needs.
type Widget struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Position        Position        `json:"position"`
	Size            Size            `json:"size"`
	TransformMatrix TransformMatrix `json:"transform_matrix"`
	TypeID          string          `json:"type_id"`
	SceneID         string          `json:"scene_id"`
	PortBindings    []PortBinding   `json:"port_bindings"`
}

// Tag is a Tag's state as the REST API and the tag_updated event carry it.
type Tag struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	Value   string  `json:"value"`
	Quality Quality `json:"quality"`
	Version int     `json:"version"`
}
