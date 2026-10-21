package uiutil

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	widgetscriptv0 "github.com/kipitix/growscada/contract/widgetscript/v0"
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

// IframeSrcdoc gives a component's iframe its document, a field of the
// component that calls Mount from OnMount and Update from OnUpdate. It sets
// the srcdoc property (not the attribute: only the property reloads the
// iframe) and only when the document changed, since setting it reloads the
// iframe. It remembers the document it set, so an update with the same
// document (the canvas re-renders on every drag move) costs no copy of it
// out of the DOM.
//
// The component passes its own element (JSValue), never looks it up by id:
// go-app reuses a component for another item in the same place (e.g. a
// Widget of another Scene), and OnUpdate may run before the element's id is
// updated, so a lookup by the new id finds nothing and the iframe keeps
// showing the previous item.
type IframeSrcdoc struct {
	last string
	set  bool
}

// Mount gives the freshly mounted iframe frame the document srcdoc, unless
// it already has it.
func (s *IframeSrcdoc) Mount(frame app.Value, srcdoc string) {
	s.set = false
	if !isElement(frame) {
		return
	}
	if frame.Get("srcdoc").String() != srcdoc {
		frame.Set("srcdoc", srcdoc)
	}
	s.last, s.set = srcdoc, true
}

// Update gives frame the document srcdoc if it differs from the one set last.
func (s *IframeSrcdoc) Update(frame app.Value, srcdoc string) {
	if s.set && s.last == srcdoc {
		return
	}
	if !isElement(frame) {
		return
	}
	frame.Set("srcdoc", srcdoc)
	s.last, s.set = srcdoc, true
}

func isElement(v app.Value) bool {
	return v != nil && !v.IsNull() && !v.IsUndefined()
}

// InputsJSON encodes inputs as a JSON object. The encoder escapes <, > and &,
// so the result can be embedded in a <script> element as is.
func InputsJSON(inputs widgetscriptv0.Inputs) string {
	if inputs == nil {
		inputs = widgetscriptv0.Inputs{}
	}
	b, err := json.Marshal(inputs)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// NewInput converts a value given as text to the input of a port of type typ:
// an "integer", "boolean" or "string" TagType or type hint, or "" for the
// "any" hint, which keeps the text as a string. The input's text is the value
// in canonical form ("+007" → "7"). ok is false when text is not a value of
// typ: an integer that does not fit int64, a boolean other than "true" or
// "false".
func NewInput(text, typ string) (widgetscriptv0.Input, bool) {
	var value widgetscriptv0.Value
	switch typ {
	case "integer":
		n, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return widgetscriptv0.Input{}, false
		}
		value, text = widgetscriptv0.IntegerValue(n), strconv.FormatInt(n, 10)
	case "boolean":
		if text != "true" && text != "false" {
			return widgetscriptv0.Input{}, false
		}
		value = widgetscriptv0.BooleanValue(text == "true")
	default:
		value = widgetscriptv0.StringValue(text)
	}
	return widgetscriptv0.Input{Value: value, Text: &text}, true
}

// NoValueInput is the input of a port without a value (no PortBinding, a
// deleted Tag, nothing valid typed into a preview): no text, and the type
// hint's default value — 0, false, "" or, for "any", undefined.
func NoValueInput(typeHint string) widgetscriptv0.Input {
	switch typeHint {
	case "integer":
		return widgetscriptv0.Input{Value: widgetscriptv0.IntegerValue(0)}
	case "boolean":
		return widgetscriptv0.Input{Value: widgetscriptv0.BooleanValue(false)}
	case "string":
		return widgetscriptv0.Input{Value: widgetscriptv0.StringValue("")}
	default:
		return widgetscriptv0.Input{}
	}
}

// PreviewInputs builds the inputs of a preview (Library, Project) from values
// typed in by the Engineer, converted by each port's type hint. An empty
// value, or one that is not a value of the port's type, leaves the port
// without a value, as in Operation.
func PreviewInputs(values map[string]string, ports []apiv0.InputPort) widgetscriptv0.Inputs {
	inputs := make(widgetscriptv0.Inputs, len(ports))
	for _, p := range ports {
		in := NoValueInput(p.TypeHint)
		if text := values[p.Name]; text != "" {
			if typed, ok := NewInput(text, p.TypeHint); ok {
				in = typed
			}
		}
		inputs[p.Name] = in
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
func BuildSrcdoc(htmlTemplate, script string, inputs widgetscriptv0.Inputs, ports []apiv0.InputPort, bgColor string) string {
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

// TypeHintLabel returns the display label of an InputPort's type hint; an
// empty one means the port accepts any TagType.
func TypeHintLabel(typeHint string) string {
	if typeHint == "" {
		return "any"
	}
	return typeHint
}
