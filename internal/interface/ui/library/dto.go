package library

// ── DTOs ─────────────────────────────────────────────────────────────────────

type widgetTypeItem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
	DefaultWidth   int    `json:"default_width"`
	DefaultHeight  int    `json:"default_height"`
	Version        int    `json:"version"`
}

type getWidgetTypesResponse struct {
	WidgetTypes []widgetTypeItem `json:"widget_types"`
}

type createWidgetTypeRequest struct {
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
	DefaultWidth   int    `json:"default_width"`
	DefaultHeight  int    `json:"default_height"`
}

type createWidgetTypeResponse struct {
	ID string `json:"id"`
}

type updateWidgetTypeRequest struct {
	Name           string `json:"name"`
	HtmlTemplate   string `json:"html_template"`
	Script         string `json:"script"`
	ScriptLanguage string `json:"script_language"`
	DefaultWidth   int    `json:"default_width"`
	DefaultHeight  int    `json:"default_height"`
	Version        int    `json:"version"`
}
