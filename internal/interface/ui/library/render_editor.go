package library

import (
	"fmt"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

// ── Editor columns ────────────────────────────────────────────────────────────

func (l *Library) renderEditorColumn(title, id, value string, onInput func(app.Context, app.Event), showApply bool) app.UI {
	base := app.Textarea().
		ID(id).
		Style("flex", "1").
		Style("resize", "none").
		Style("font-family", "monospace").
		Style("font-size", "13px")

	var textarea app.UI
	if onInput != nil {
		textarea = base.Text(value).OnInput(onInput)
	} else {
		textarea = base.Disabled(true)
	}

	applyDisabled := l.selectedID == ""
	applyBtn := app.Button().
		Style("margin-top", "4px").
		Style("padding", "4px 12px").
		Style("font-size", "13px").
		Style("border", "1px solid #ccc").
		Style("border-radius", "4px").
		Style("align-self", "flex-end").
		Text("Apply").
		OnClick(func(ctx app.Context, e app.Event) {
			l.applyChanges(ctx)
		})
	if applyDisabled {
		applyBtn = applyBtn.
			Style("opacity", "0.4").
			Style("cursor", "default").
			Disabled(true)
	} else {
		applyBtn = applyBtn.Style("cursor", "pointer")
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
		colBody = append(colBody, applyBtn)
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
			Style("color", "#999").
			Style("font-size", "13px").
			Text("No HTML to preview.")
	} else {
		content = app.IFrame().
			Attr("srcdoc", buildSrcdoc(l.editedHTML, l.editedScript, l.editedInputData)).
			Attr("sandbox", "allow-scripts").
			Style("width", "100%").
			Style("height", "100%").
			Style("border", "none")
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
				Style("border", "1px solid #ddd").
				Body(content),
		)
}

// buildSrcdoc constructs the iframe srcdoc for sandboxed widget preview.
// inputData is a raw JS expression passed to the widget's render(value) function.
func buildSrcdoc(htmlTemplate, script, inputData string) string {
	callRender := ""
	if inputData != "" {
		callRender = fmt.Sprintf("\ntry { render(%s); } catch(e) {}", inputData)
	}
	return fmt.Sprintf(`<!DOCTYPE html><html><body>%s<script>%s%s</script></body></html>`,
		htmlTemplate, script, callRender)
}
