package project

import "github.com/kipitix/growscada/internal/interface/ui/uidto"

// ── DTOs ─────────────────────────────────────────────────────────────────────

type portBindingDTO struct {
	PortName string `json:"port_name"`
	TagID    string `json:"tag_id"`
}

type widgetTypeItem struct {
	ID            string               `json:"id"`
	Name          string               `json:"name"`
	HtmlTemplate  string               `json:"html_template"`
	Script        string               `json:"script"`
	DefaultWidth  int                  `json:"default_width"`
	DefaultHeight int                  `json:"default_height"`
	InputPorts    []uidto.InputPortDTO `json:"input_ports"`
}

type getWidgetTypesResponse struct {
	WidgetTypes []widgetTypeItem `json:"widget_types"`
}

type sceneItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
	Version        int    `json:"version"`
}

type getScenesResponse struct {
	Scenes []sceneItem `json:"scenes"`
}

type createSceneRequest struct {
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
}

type createSceneResponse struct {
	ID string `json:"id"`
}

type updateSceneRequest struct {
	Name           string `json:"name"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	BackgroundHTML string `json:"background_html"`
	Version        int    `json:"version"`
}

type updateSceneResponse struct {
	Version int `json:"version"`
}

// Widget geometry sub-DTOs (match server restdto shape).

type positionDTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z int     `json:"z"`
}

type sizeDTO struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type originDTO struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type rotationDTO struct {
	Degrees float64 `json:"degrees"`
}

type transformMatrixDTO struct {
	A   float64 `json:"a"`
	B   float64 `json:"b"`
	C   float64 `json:"c"`
	D   float64 `json:"d"`
	E   float64 `json:"e"`
	F   float64 `json:"f"`
	CSS string  `json:"css"`
}

type widgetItem struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Position        positionDTO        `json:"position"`
	Size            sizeDTO            `json:"size"`
	Origin          originDTO          `json:"origin"`
	Rotation        rotationDTO        `json:"rotation"`
	TransformMatrix transformMatrixDTO `json:"transform_matrix"`
	TypeID          string             `json:"type_id"`
	SceneID         string             `json:"scene_id"`
	Labels          []string           `json:"labels"`
	PortBindings    []portBindingDTO   `json:"port_bindings"`
	Version         int                `json:"version"`
}

type getWidgetsResponse struct {
	Widgets []widgetItem `json:"widgets"`
}

type createWidgetRequest struct {
	Name         string           `json:"name"`
	Position     positionDTO      `json:"position"`
	Size         sizeDTO          `json:"size"`
	Origin       originDTO        `json:"origin"`
	Rotation     rotationDTO      `json:"rotation"`
	TypeID       string           `json:"type_id"`
	SceneID      string           `json:"scene_id"`
	Labels       []string         `json:"labels"`
	PortBindings []portBindingDTO `json:"port_bindings"`
}

type createWidgetResponse struct {
	ID string `json:"id"`
}

type updateWidgetRequest struct {
	Name         string           `json:"name"`
	Position     positionDTO      `json:"position"`
	Size         sizeDTO          `json:"size"`
	Origin       originDTO        `json:"origin"`
	Rotation     rotationDTO      `json:"rotation"`
	TypeID       string           `json:"type_id"`
	SceneID      string           `json:"scene_id"`
	Labels       []string         `json:"labels"`
	PortBindings []portBindingDTO `json:"port_bindings"`
	Version      int              `json:"version"`
}

type updateWidgetResponse struct {
	Version int `json:"version"`
}

type tagItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Quality string `json:"quality"`
	Version int    `json:"version"`
}

type getTagsResponse struct {
	Tags []tagItem `json:"tags"`
}

type createTagRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

type createTagResponse struct {
	ID string `json:"id"`
}
