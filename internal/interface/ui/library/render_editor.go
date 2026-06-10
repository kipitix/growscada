package library

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/uidto"
)

// inputDataField is a thin component wrapping a single Input Data text field.
// OnMount/OnUpdate explicitly set the DOM value property (not just the attribute)
// so that the displayed value clears correctly when switching between WidgetTypes.
// go-app's virtual DOM stores empty string as a missing attribute and removes
// the attribute via removeAttribute, which does NOT clear input.value in the
// browser — only assigning the property directly does.
type inputDataField struct {
	app.Compo
	FieldID     string
	Placeholder string
	Val         string
	PortName    string
	OnChange    func(string)
	lastVal     string // tracks last written DOM value to skip redundant Defer calls
}

func (f *inputDataField) Render() app.UI {
	onChange := f.OnChange
	portName := f.PortName
	return app.Input().
		ID(f.FieldID).
		Type("text").
		Placeholder(f.Placeholder).
		Value(f.Val).
		Style("width", "100%").
		Style("font-size", "12px").
		Style("padding", "3px 6px").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "3px").
		Style("background", "var(--input-bg)").
		Style("color", "var(--text)").
		Style("box-sizing", "border-box").
		OnInput(func(ctx app.Context, e app.Event) {
			if onChange != nil {
				onChange(ctx.JSSrc().Get("value").String())
			}
		}, app.EventScope(portName))
}

func (f *inputDataField) setDOMValue(ctx app.Context) {
	fieldID := f.FieldID
	val := f.Val
	// Defer runs after the DOM patch cycle, so getElementById finds the element
	// with its already-updated id and we can set the value property directly.
	ctx.Defer(func(ctx app.Context) {
		elem := app.Window().Get("document").Call("getElementById", fieldID)
		if !elem.IsNull() && !elem.IsUndefined() {
			elem.Set("value", val)
		}
	})
}

func (f *inputDataField) OnMount(ctx app.Context) {
	f.lastVal = f.Val
	f.setDOMValue(ctx)
}

func (f *inputDataField) OnUpdate(ctx app.Context) {
	if f.Val == f.lastVal {
		return
	}
	f.lastVal = f.Val
	f.setDOMValue(ctx)
}

// previewFrame is a thin component that owns the sandboxed preview iframe.
// It sets iframe.srcdoc via a JS property assignment (not setAttribute) on
// mount and every update, because browsers only reload an iframe when the
// srcdoc *property* is written — mutating the HTML attribute has no effect.
type previewFrame struct {
	app.Compo
	ID     string
	Srcdoc string
}

func (p *previewFrame) Render() app.UI {
	return app.IFrame().
		ID(p.ID).
		Attr("sandbox", "allow-scripts").
		Attr("allowtransparency", "true").
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("background", "transparent")
}

func (p *previewFrame) setSrcdoc() {
	elem := app.Window().Get("document").Call("getElementById", p.ID)
	if !elem.IsNull() && !elem.IsUndefined() {
		elem.Set("srcdoc", p.Srcdoc)
	}
}

func (p *previewFrame) OnMount(ctx app.Context)  { p.setSrcdoc() }
func (p *previewFrame) OnUpdate(ctx app.Context) { p.setSrcdoc() }

// ── Editor columns ────────────────────────────────────────────────────────────

func (l *Library) renderEditorColumn(title, id, value string, onInput func(app.Context, app.Event), showApply bool) app.UI {
	base := app.Textarea().
		ID(id).
		Style("flex", "1").
		Style("resize", "none").
		Style("font-family", "monospace").
		Style("font-size", "13px").
		Style("background", "var(--input-bg)").
		Style("color", "var(--text)").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "3px").
		Style("padding", "4px 6px").
		Style("box-sizing", "border-box")

	var textarea app.UI
	if onInput != nil {
		textarea = base.Text(value).OnInput(onInput)
	} else {
		textarea = base.Disabled(true)
	}

	colBody := []app.UI{
		app.H3().Style("margin", "0 0 8px 0").Text(title),
		app.Div().
			Style("flex", "1").
			Style("min-height", "0").
			Style("display", "flex").
			Style("flex-direction", "column").
			Body(textarea),
	}
	if showApply {
		colBody = append(colBody, l.renderApplyButton())
	}

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(colBody...)
}

func (l *Library) renderPreviewColumn() app.UI {
	var content app.UI
	if l.editedHTML == "" {
		content = app.Div().
			Style("color", "var(--text-muted)").
			Style("font-size", "13px").
			Text("No HTML to preview.")
	} else {
		content = &previewFrame{
			ID:     "preview-iframe",
			Srcdoc: buildSrcdoc(l.editedHTML, l.editedScript, l.editedInputValues, l.editedInputPorts, iframeBgColor()),
		}
	}
	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Preview"),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow", "auto").
				Style("border", "1px solid var(--border)").
				Body(content),
		)
}

// renderInputPortsColumn renders the Input Ports management panel.
func (l *Library) renderInputPortsColumn() app.UI {
	disabled := l.selectedID == ""

	// Existing ports list
	portRows := make([]app.UI, 0, len(l.editedInputPorts))
	for i, p := range l.editedInputPorts {
		idx := i
		typeLabel := uidto.TypeHintLabel(p.TypeHint)
		desc := p.Description
		if desc == "" {
			desc = "—"
		}
		row := app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "4px").
			Style("padding", "3px 0").
			Style("border-bottom", "1px solid var(--border)").
			Body(
				app.Div().
					Style("flex", "1").
					Style("font-size", "13px").
					Body(
						app.Span().Style("font-weight", "600").Text(p.Name),
						app.Span().Style("color", "var(--text-muted)").Style("margin-left", "4px").Text("("+typeLabel+")"),
						app.Div().Style("font-size", "11px").Style("color", "var(--text-muted)").Text(desc),
					),
				app.Button().
					Style("font-size", "11px").
					Style("padding", "1px 6px").
					Style("cursor", "pointer").
					Style("border", "1px solid var(--border-input)").
					Style("border-radius", "3px").
					Style("background", "var(--bg-hover)").
					Style("color", "var(--text)").
					Text("✕").
					Disabled(disabled).
					OnClick(func(ctx app.Context, e app.Event) {
						ports := make([]uidto.InputPortDTO, 0, len(l.editedInputPorts)-1)
						for j, pp := range l.editedInputPorts {
							if j != idx {
								ports = append(ports, pp)
							}
						}
						l.editedInputPorts = ports
					}),
			)
		portRows = append(portRows, row)
	}

	emptyNote := app.If(len(l.editedInputPorts) == 0,
		func() app.UI {
			return app.Div().
				Style("font-size", "12px").
				Style("color", "var(--text-muted)").
				Style("padding", "4px 0").
				Text("No ports defined.")
		},
	)

	// Type hint options
	typeOptions := []app.UI{
		app.Option().Value("").Text("any"),
		app.Option().Value("string").Text("string"),
		app.Option().Value("boolean").Text("boolean"),
		app.Option().Value("integer").Text("integer"),
	}

	// Add-port form
	addForm := app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("gap", "4px").
		Style("margin-top", "8px").
		Body(
			app.Input().
				Type("text").
				Placeholder("Port name (JS identifier)").
				Value(l.newPortName).
				Style("font-size", "12px").
				Style("padding", "3px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "3px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				Disabled(disabled).
				OnInput(func(ctx app.Context, e app.Event) {
					l.newPortName = ctx.JSSrc().Get("value").String()
				}),
			app.Input().
				Type("text").
				Placeholder("Description (optional)").
				Value(l.newPortDesc).
				Style("font-size", "12px").
				Style("padding", "3px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "3px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				Disabled(disabled).
				OnInput(func(ctx app.Context, e app.Event) {
					l.newPortDesc = ctx.JSSrc().Get("value").String()
				}),
			app.Select().
				Style("font-size", "12px").
				Style("padding", "3px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "3px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				Disabled(disabled).
				OnChange(func(ctx app.Context, e app.Event) {
					l.newPortType = ctx.JSSrc().Get("value").String()
				}).
				Body(typeOptions...),
			app.Button().
				Style("font-size", "12px").
				Style("padding", "3px 8px").
				Style("cursor", "pointer").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "3px").
				Style("background", "var(--bg-hover)").
				Style("color", "var(--text)").
				Style("align-self", "flex-start").
				Text("+ Add Port").
				Disabled(disabled).
				OnClick(func(ctx app.Context, e app.Event) {
					name := strings.TrimSpace(l.newPortName)
					if name == "" {
						return
					}
					// Prevent duplicate names in UI
					for _, p := range l.editedInputPorts {
						if p.Name == name {
							return
						}
					}
					l.editedInputPorts = append(l.editedInputPorts, uidto.InputPortDTO{
						Name:        name,
						Description: l.newPortDesc,
						TypeHint:    l.newPortType,
					})
					l.newPortName = ""
					l.newPortDesc = ""
					l.newPortType = ""
				}),
		)

	portList := make([]app.UI, 0, len(portRows)+1)
	portList = append(portList, emptyNote)
	portList = append(portList, portRows...)

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Input Ports"),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow-y", "auto").
				Body(portList...),
			addForm,
			l.renderApplyButton(),
		)
}

// renderInputDataColumn renders a per-port value table for the preview sandbox.
func (l *Library) renderInputDataColumn() app.UI {
	emptyNote := app.If(len(l.editedInputPorts) == 0, func() app.UI {
		return app.Div().
			Style("font-size", "12px").
			Style("color", "var(--text-muted)").
			Style("padding", "4px 0").
			Text("Define Input Ports first.")
	})

	rows := make([]app.UI, 0, len(l.editedInputPorts))
	for _, p := range l.editedInputPorts {
		portName := p.Name
		typeLabel := uidto.TypeHintLabel(p.TypeHint)
		placeholder := map[string]string{
			"integer": "0",
			"boolean": "true",
			"string":  "hello",
		}[p.TypeHint]
		if placeholder == "" {
			placeholder = "value"
		}
		val := l.editedInputValues[portName]

		row := app.Tr().Body(
			app.Td().
				Style("padding", "4px 8px 4px 0").
				Style("font-size", "13px").
				Style("font-weight", "600").
				Style("white-space", "nowrap").
				Style("vertical-align", "middle").
				Text(portName),
			app.Td().
				Style("padding", "4px 6px").
				Style("vertical-align", "middle").
				Body(
					app.Span().
						Style("font-size", "11px").
						Style("padding", "2px 6px").
						Style("border-radius", "3px").
						Style("background", "var(--bg-hover)").
						Style("color", "var(--text-muted)").
						Style("white-space", "nowrap").
						Text(typeLabel),
				),
			app.Td().
				Style("padding", "4px 0").
				Style("width", "100%").
				Style("vertical-align", "middle").
				Body(
					&inputDataField{
						FieldID:     "input-data-" + portName,
						Placeholder: placeholder,
						Val:         val,
						PortName:    portName,
						OnChange: func(v string) {
							if l.editedInputValues == nil {
								l.editedInputValues = make(map[string]string)
							}
							l.editedInputValues[portName] = v
						},
					},
				),
		)
		rows = append(rows, row)
	}

	tableUI := app.If(len(l.editedInputPorts) > 0, func() app.UI {
		return app.Table().
			Style("width", "100%").
			Style("border-collapse", "collapse").
			Body(rows...)
	})

	return app.Div().
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Body(
			app.H3().Style("margin", "0 0 8px 0").Text("Input Data"),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow-y", "auto").
				Body(emptyNote, tableUI),
		)
}

// iframeBgColor reads the current --bg CSS variable from the parent document.
// Called at render time so each srcdoc embeds the correct theme background.
func iframeBgColor() string {
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

// buildSrcdoc constructs the iframe srcdoc for sandboxed widget preview.
// It builds an `inputs` object from inputValues keyed by port name.
func buildSrcdoc(htmlTemplate, script string, inputValues map[string]string, ports []uidto.InputPortDTO, bgColor string) string {
	callRender := ""
	if len(ports) > 0 {
		parts := make([]string, 0, len(ports))
		for _, p := range ports {
			parts = append(parts, p.Name+":"+portValueToJS(inputValues[p.Name], p.TypeHint))
		}
		callRender = fmt.Sprintf("\nvar inputs={%s};\ntry{render(inputs);}catch(e){}", strings.Join(parts, ","))
	}
	return fmt.Sprintf(`<!DOCTYPE html><html><head><style>html,body{background:%s;margin:0;padding:0}</style></head><body>%s<script>%s%s</script></body></html>`,
		bgColor, htmlTemplate, script, callRender)
}

func (l *Library) renderApplyButton() app.UI {
	btn := app.Button().
		Style("margin-top", "4px").
		Style("padding", "4px 12px").
		Style("font-size", "13px").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("align-self", "flex-end").
		Style("background", "var(--bg-hover)").
		Style("color", "var(--text)").
		Text("Apply").
		OnClick(func(ctx app.Context, e app.Event) {
			l.applyChanges(ctx)
		})
	if l.selectedID == "" {
		return btn.Style("opacity", "0.4").Style("cursor", "default").Disabled(true)
	}
	return btn.Style("cursor", "pointer")
}

// portValueToJS converts a user-entered string to a JS literal based on type hint.
// Values are validated/encoded to prevent JS injection in the preview srcdoc.
func portValueToJS(val, typeHint string) string {
	if val == "" {
		switch typeHint {
		case "integer":
			return "0"
		case "boolean":
			return "false"
		case "string":
			return `""`
		default:
			return "undefined"
		}
	}
	switch typeHint {
	case "integer":
		if _, err := strconv.ParseInt(val, 10, 64); err == nil {
			return val
		}
		return "0"
	case "boolean":
		if val == "true" || val == "false" {
			return val
		}
		return "false"
	default: // "string", "unknown", "" and any future type hints
		b, _ := json.Marshal(val)
		return string(b)
	}
}
