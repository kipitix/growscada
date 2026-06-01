package library

// ── DTOs ─────────────────────────────────────────────────────────────────────

type inputPortDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TypeHint    string `json:"type_hint"`
}

type widgetTypeItem struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	HtmlTemplate   string         `json:"html_template"`
	Script         string         `json:"script"`
	ScriptLanguage string         `json:"script_language"`
	DefaultWidth   int            `json:"default_width"`
	DefaultHeight  int            `json:"default_height"`
	InputPorts     []inputPortDTO `json:"input_ports"`
	Version        int            `json:"version"`
}

type getWidgetTypesResponse struct {
	WidgetTypes []widgetTypeItem `json:"widget_types"`
}

type createWidgetTypeRequest struct {
	Name           string         `json:"name"`
	HtmlTemplate   string         `json:"html_template"`
	Script         string         `json:"script"`
	ScriptLanguage string         `json:"script_language"`
	DefaultWidth   int            `json:"default_width"`
	DefaultHeight  int            `json:"default_height"`
	InputPorts     []inputPortDTO `json:"input_ports"`
}

type createWidgetTypeResponse struct {
	ID string `json:"id"`
}

type updateWidgetTypeRequest struct {
	Name           string         `json:"name"`
	HtmlTemplate   string         `json:"html_template"`
	Script         string         `json:"script"`
	ScriptLanguage string         `json:"script_language"`
	DefaultWidth   int            `json:"default_width"`
	DefaultHeight  int            `json:"default_height"`
	InputPorts     []inputPortDTO `json:"input_ports"`
	Version        int            `json:"version"`
}
