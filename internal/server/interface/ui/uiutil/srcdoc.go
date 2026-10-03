package uiutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uidto"
)

// styleCloseRE matches </style> in any capitalisation or with trailing whitespace.
var styleCloseRE = regexp.MustCompile(`(?i)</style\s*>`)

var jsIdentRE = regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$]*$`)

// IsValidJSIdentifier reports whether s can safely be used as an unquoted JS
// object key. Call this before accepting user-supplied port names.
func IsValidJSIdentifier(s string) bool {
	return jsIdentRE.MatchString(s)
}

// SceneWidgetBackground is the background of a Widget placed on a Scene
// (Operation, Project's canvas): none, so the Widget is drawn right on the
// Scene's background. Previews shown on their own (Library, Project's widget
// type thumbnails) use the theme's background instead (IframeBgColor).
const SceneWidgetBackground = "transparent"

// IframeBgColor reads --bg from the parent document's computed CSS.
func IframeBgColor() string {
	color := strings.TrimSpace(
		app.Window().Call("getComputedStyle",
			app.Window().Get("document").Get("documentElement"),
		).Call("getPropertyValue", "--bg").String(),
	)
	if color == "" {
		return "#ffffff"
	}
	return color
}

// IframeTextMuted reads --text-muted from the parent document's computed CSS.
func IframeTextMuted() string {
	color := strings.TrimSpace(
		app.Window().Call("getComputedStyle",
			app.Window().Get("document").Get("documentElement"),
		).Call("getPropertyValue", "--text-muted").String(),
	)
	if color == "" {
		return "#888888"
	}
	return color
}

// SetIframeSrcdoc sets the srcdoc property of the iframe identified by id.
func SetIframeSrcdoc(id, srcdoc string) {
	elem := app.Window().Get("document").Call("getElementById", id)
	if !elem.IsNull() && !elem.IsUndefined() {
		elem.Set("srcdoc", srcdoc)
	}
}

// Input is one InputPort's entry in the `inputs` object a WidgetType script's
// render(inputs) receives: `inputs.<port>.value`. Fields are only ever added
// to it, never changed (docs/widget_type_contract.md).
type Input struct {
	// Value is the JS value of the port; nil means undefined.
	Value any
}

// MarshalJSON encodes the Input as `{"value": ...}`, omitting "value" when it
// is undefined so the script sees `inputs.<port>.value === undefined`.
func (in Input) MarshalJSON() ([]byte, error) {
	if in.Value == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(struct {
		Value any `json:"value"`
	}{in.Value})
}

// Inputs is the `inputs` object passed to render(inputs), keyed by port name.
type Inputs map[string]Input

// InputsJSON encodes inputs as a JSON object. The encoder escapes <, > and &,
// so the result can be embedded in a <script> element as is.
func InputsJSON(inputs Inputs) string {
	if inputs == nil {
		inputs = Inputs{}
	}
	b, err := json.Marshal(inputs)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// NewInput converts a raw string value to the port's JS value by type: an
// "integer", "boolean" or "string" TagType or type hint. An empty type ("any"
// hint) keeps a non-empty value as a string. A value that is empty or does not
// parse falls back to the type's default: 0, false, "" or, for "any", undefined.
func NewInput(raw, typ string) Input {
	switch typ {
	case "integer":
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return Input{Value: n}
		}
		return Input{Value: int64(0)}
	case "boolean":
		return Input{Value: raw == "true"}
	case "string":
		return Input{Value: raw}
	default:
		if raw == "" {
			return Input{}
		}
		return Input{Value: raw}
	}
}

// PreviewInputs builds the inputs of a preview (Library, Project) from values
// typed in by the Engineer, converted by each port's type hint.
func PreviewInputs(values map[string]string, ports []uidto.InputPortDTO) Inputs {
	inputs := make(Inputs, len(ports))
	for _, p := range ports {
		inputs[p.Name] = NewInput(values[p.Name], p.TypeHint)
	}
	return inputs
}

// srcdocPage wraps a widget's HTML and scripts into the sandboxed iframe
// document: it injects the background colour and a connect-src CSP, and
// escapes any </script> in user-authored code to prevent early script-tag
// termination. tail is trusted code run after the WidgetType script in a
// separate <script>, so an error thrown by the script does not stop it.
func srcdocPage(htmlTemplate, script, tail, bgColor string) string {
	// Strip </style> (any capitalisation/spacing) so a CSS variable value cannot
	// close the injected <style> block prematurely.
	bgColor = styleCloseRE.ReplaceAllString(bgColor, "")
	// Escape </script> so a literal occurrence in user-authored JS cannot
	// terminate the enclosing <script> element and inject new HTML.
	safeScript := strings.ReplaceAll(script, "</script>", `<\/script>`)
	return fmt.Sprintf(
		`<!DOCTYPE html><html><head><meta http-equiv="Content-Security-Policy" content="connect-src 'none'"><style>html,body{background:%s;margin:0;padding:0}</style></head><body>%s<script>%s</script><script>%s</script></body></html>`,
		bgColor, htmlTemplate, safeScript, tail)
}

// BuildSrcdoc constructs the iframe srcdoc for a sandboxed widget preview: the
// WidgetType's render(inputs) is called once with the given inputs, if the
// type has any ports. The script contract is in docs/widget_type_contract.md.
func BuildSrcdoc(htmlTemplate, script string, inputs Inputs, ports []uidto.InputPortDTO, bgColor string) string {
	tail := ""
	if len(ports) > 0 {
		tail = fmt.Sprintf("try{render(%s);}catch(e){}", InputsJSON(inputs))
	}
	return srcdocPage(htmlTemplate, script, tail, bgColor)
}

// Messages exchanged with a live widget iframe (BuildLiveSrcdoc).
const (
	// MessageReady is sent by the iframe to its parent once the WidgetType
	// script has run: `{type: "ready"}`.
	MessageReady = "ready"
	// MessageInputs is sent by the parent to the iframe to (re)render it:
	// `{type: "inputs", inputs: {...}}`.
	MessageInputs = "inputs"
)

// liveWidgetTail listens for inputs messages from the parent window only and
// announces readiness, so the parent sends inputs no earlier than the iframe
// can apply them.
const liveWidgetTail = `window.addEventListener("message",function(e){` +
	`if(e.source!==window.parent)return;var m=e.data;` +
	`if(!m||m.type!=="` + MessageInputs + `"||typeof render!=="function")return;` +
	`try{render(m.inputs);}catch(err){}});` +
	`window.parent.postMessage({type:"` + MessageReady + `"},"*");`

// BuildLiveSrcdoc constructs the srcdoc of a live widget (Operation): the
// iframe is loaded once and render(inputs) is called on every inputs message
// from the parent, without reloading it. A live widget is always on a Scene,
// so its background is SceneWidgetBackground. The script contract is in
// docs/widget_type_contract.md.
func BuildLiveSrcdoc(htmlTemplate, script string) string {
	return srcdocPage(htmlTemplate, script, liveWidgetTail, SceneWidgetBackground)
}
