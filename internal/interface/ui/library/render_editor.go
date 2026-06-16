package library

import (
	"fmt"
	"strings"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/interface/ui/uidto"
	"github.com/kipitix/growscada/internal/interface/ui/uiutil"
)

// inputDataField is a thin component wrapping a single Input Data field.
// OnMount/OnUpdate explicitly set the DOM value property (not just the attribute)
// so that the displayed value clears correctly when switching between WidgetTypes.
// go-app's virtual DOM stores empty string as a missing attribute and removes
// the attribute via removeAttribute, which does NOT clear input.value in the
// browser — only assigning the property directly does.
type inputDataField struct {
	app.Compo
	FieldID     string
	InputType   string // "text" | "number"
	Placeholder string
	Val         string
	PortName    string
	OnChange    func(string)
	lastVal     string // tracks last written DOM value to skip redundant Defer calls
}

func (f *inputDataField) Render() app.UI {
	onChange := f.OnChange
	portName := f.PortName
	inputType := f.InputType
	if inputType == "" {
		inputType = "text"
	}
	return app.Input().
		Class("inp").
		ID(f.FieldID).
		Type(inputType).
		Placeholder(f.Placeholder).
		Value(f.Val).
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

func (p *previewFrame) OnMount(ctx app.Context)  { uiutil.SetIframeSrcdoc(p.ID, p.Srcdoc) }
func (p *previewFrame) OnUpdate(ctx app.Context) { uiutil.SetIframeSrcdoc(p.ID, p.Srcdoc) }

// ── Code column ───────────────────────────────────────────────────────────────

func (l *Library) renderCodePanel() app.UI {
	disabled := l.selectedID == ""

	segBtn := func(tab, label string) app.UI {
		class := "seg-btn"
		if l.codeTab == tab {
			class += " active"
		}
		return app.Button().
			Class(class).
			Text(label).
			OnClick(func(ctx app.Context, e app.Event) {
				l.codeTab = tab
			})
	}

	value := l.editedHTML
	onInput := func(ctx app.Context, e app.Event) {
		l.editedHTML = ctx.JSSrc().Get("value").String()
	}
	if l.codeTab == "js" {
		value = l.editedScript
		onInput = func(ctx app.Context, e app.Event) {
			l.editedScript = ctx.JSSrc().Get("value").String()
		}
	}

	textarea := app.Textarea().
		Class("code-editor").
		Text(value)
	if disabled {
		textarea = textarea.Disabled(true)
	} else {
		textarea = textarea.OnInput(onInput)
	}

	return app.Div().
		Class("panel").
		Style("flex", "1.3 1 0").
		Body(
			app.Div().Class("panel-head").Style("height", "40px").Style("flex-basis", "40px").Body(
				app.Div().Class("seg").Body(
					segBtn("html", "Template"),
					segBtn("js", "Script"),
				),
				app.Div().Class("panel-head-actions").Body(
					l.renderApplyButton(),
				),
			),
			app.Div().Class("panel-body").Style("padding", "0").Body(
				app.Div().Class("codepanel").Body(textarea),
			),
		)
}

func (l *Library) renderApplyButton() app.UI {
	disabled := l.selectedID == ""
	return app.Button().
		Class("btn", "btn-accent", "btn-sm").
		Text("✓ Apply").
		Disabled(disabled).
		OnClick(func(ctx app.Context, e app.Event) {
			l.applyChanges(ctx)
		})
}

// ── Ports + Data column ──────────────────────────────────────────────────────

func (l *Library) renderPortsDataPanel() app.UI {
	return app.Div().
		Class("panel").
		Style("flex", "0 0 296px").
		Body(
			app.Div().Class("panel-head").Body(
				app.Div().Class("panel-title").Text("Input Ports"),
				app.Span().Class("panel-count").Text(len(l.editedInputPorts)),
				app.Div().Class("panel-head-actions").Body(
					l.renderApplyButton(),
				),
			),
			app.Div().Class("panel-body").Style("flex", "0 0 auto").Style("max-height", "46%").Body(
				l.renderPortsList(),
			),
			app.Div().Class("panel-head").Style("border-top", "1px solid var(--border-soft)").Body(
				app.Div().Class("panel-title").Text("Input Data"),
				app.Span().Class("panel-count").Text(len(l.editedInputPorts)),
			),
			app.Div().Class("panel-body").Style("flex", "1 1 0").Body(
				l.renderDataList(),
			),
		)
}

func (l *Library) renderTypeBadge(typeHint string) app.UI {
	label := uidto.TypeHintLabel(typeHint)
	class := "badge badge-" + label
	return app.Span().Class(class).Text(label)
}

func (l *Library) renderPortsList() app.UI {
	disabled := l.selectedID == ""

	rows := make([]app.UI, 0, len(l.editedInputPorts)+1)
	for i, p := range l.editedInputPorts {
		idx := i
		desc := p.Description
		if desc == "" {
			desc = "—"
		}
		rows = append(rows, app.Div().Class("port-row").Body(
			app.Div().Class("port-handle").Text("⏚"),
			app.Div().Class("port-main").Body(
				app.Div().Class("port-name-row").Body(
					app.Span().Class("port-name").Text(p.Name),
					l.renderTypeBadge(p.TypeHint),
				),
				app.Div().Class("port-desc").Text(desc),
			),
			app.Button().
				Class("port-x").
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
		))
	}

	if len(l.editedInputPorts) == 0 {
		rows = append(rows, app.Div().Class("empty").Text("No ports defined."))
	}

	typeOptions := []app.UI{
		app.Option().Value("").Text("any"),
		app.Option().Value("string").Text("string"),
		app.Option().Value("boolean").Text("boolean"),
		app.Option().Value("integer").Text("integer"),
	}

	addForm := app.Div().Class("addport").Body(
		app.Input().
			Class("inp").
			Type("text").
			Placeholder("Port name (JS identifier)").
			Value(l.newPortName).
			Disabled(disabled).
			OnInput(func(ctx app.Context, e app.Event) {
				l.newPortName = ctx.JSSrc().Get("value").String()
			}),
		app.Input().
			Class("inp").
			Type("text").
			Placeholder("Description (optional)").
			Value(l.newPortDesc).
			Disabled(disabled).
			OnInput(func(ctx app.Context, e app.Event) {
				l.newPortDesc = ctx.JSSrc().Get("value").String()
			}),
		app.Select().
			Class("inp").
			Disabled(disabled).
			OnChange(func(ctx app.Context, e app.Event) {
				l.newPortType = ctx.JSSrc().Get("value").String()
			}).
			Body(typeOptions...),
		app.Button().
			Class("btn", "btn-sm", "btn-block").
			Text("+ Add Port").
			Disabled(disabled).
			OnClick(func(ctx app.Context, e app.Event) {
				name := strings.TrimSpace(l.newPortName)
				if name == "" || !uiutil.IsValidJSIdentifier(name) {
					return
				}
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

	rows = append(rows, addForm)
	return app.Div().Class("ports").Body(rows...)
}

func (l *Library) renderDataList() app.UI {
	if len(l.editedInputPorts) == 0 {
		return app.Div().Class("empty").Text("Define Input Ports first.")
	}

	rows := make([]app.UI, 0, len(l.editedInputPorts))
	for _, p := range l.editedInputPorts {
		portName := p.Name
		val := l.editedInputValues[portName]

		onChange := func(v string) {
			if l.editedInputValues == nil {
				l.editedInputValues = make(map[string]string)
			}
			l.editedInputValues[portName] = v
		}

		var control app.UI
		switch p.TypeHint {
		case "boolean":
			control = l.renderBooleanToggle(portName, val, onChange)
		case "integer":
			control = &inputDataField{
				FieldID:     "input-data-" + portName,
				InputType:   "number",
				Placeholder: "0",
				Val:         val,
				PortName:    portName,
				OnChange:    onChange,
			}
		default:
			placeholder := "value"
			if p.TypeHint == "string" {
				placeholder = "hello"
			}
			control = &inputDataField{
				FieldID:     "input-data-" + portName,
				InputType:   "text",
				Placeholder: placeholder,
				Val:         val,
				PortName:    portName,
				OnChange:    onChange,
			}
		}

		rows = append(rows, app.Div().Class("data-row").Body(
			app.Div().Class("data-info").Body(
				app.Span().Class("data-name").Text(portName),
				l.renderTypeBadge(p.TypeHint),
			),
			app.Div().Class("data-val").Body(control),
		))
	}
	return app.Div().Class("data").Body(rows...)
}

func (l *Library) renderBooleanToggle(portName, val string, onChange func(string)) app.UI {
	on := val == "true"
	class := "toggle"
	if on {
		class += " on"
	}
	label := "false"
	if on {
		label = "true"
	}
	return app.Div().
		Class(class).
		OnClick(func(ctx app.Context, e app.Event) {
			newVal := "true"
			if on {
				newVal = "false"
			}
			onChange(newVal)
		}).
		Body(
			app.Div().Class("toggle-track").Body(
				app.Div().Class("toggle-knob"),
			),
			app.Span().Class("toggle-label").Text(label),
		)
}

// ── Preview column ───────────────────────────────────────────────────────────

func (l *Library) renderPreviewPanel() app.UI {
	var stage app.UI
	if l.editedHTML == "" {
		stage = app.Div().Class("empty").Text("No HTML to preview.")
	} else {
		stage = &previewFrame{
			ID:     "preview-iframe",
			Srcdoc: uiutil.BuildSrcdoc(l.editedHTML, l.editedScript, l.editedInputValues, l.editedInputPorts, uiutil.IframeBgColor()),
		}
	}

	widgetName := l.editedName
	if l.selectedID == "" {
		widgetName = "—"
	}

	valueParts := make([]string, 0, len(l.editedInputPorts))
	for _, p := range l.editedInputPorts {
		valueParts = append(valueParts, p.Name+" = "+l.editedInputValues[p.Name])
	}

	return app.Div().
		Class("panel").
		Style("flex", "1.05 1 0").
		Body(
			app.Div().Class("panel-head").Body(
				app.Div().Class("panel-title").Text("Preview"),
				app.Div().Class("panel-head-actions").Body(
					app.Span().Class("badge").Text("live"),
				),
			),
			app.Div().Class("preview-wrap").Body(
				app.Div().Class("preview-frame").Body(
					app.Div().Class("preview-card").Body(stage),
				),
				app.Div().Class("preview-meta").Body(
					app.Span().Class("dot"),
					app.Span().Text(widgetName),
					app.Span().Class("preview-meta-values").Text(strings.Join(valueParts, "   ")),
				),
			),
		)
}

// ── Status bar ────────────────────────────────────────────────────────────────

func (l *Library) renderStatusBar() app.UI {
	widgetName := l.editedName
	if l.selectedID == "" {
		widgetName = "No widget selected"
	}
	mode := "template.svg"
	if l.codeTab == "js" {
		mode = "render.js"
	}
	return app.Div().Class("statusbar").Body(
		app.Div().Class("status-item").Text(widgetName),
		app.Div().Class("status-item", "status-soft").Text(mode),
		app.Div().Class("status-sep"),
		app.Div().Class("status-item", "status-soft").Text(fmt.Sprintf("%d ports bound", len(l.editedInputPorts))),
		app.Div().Class("status-item").Body(
			app.Span().Class("status-dot"),
			app.Text(" Live"),
		),
	)
}
