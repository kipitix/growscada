// Package labels is the page that manages the server's Labels: their names
// and colors (task 85_2, task 90; ADR 0025). Labels are marks an Engineer puts
// on saved states; they belong to the Organization and are metadata, so the
// page creates, renames, recolors and deletes names, and never touches a
// Draft, a Version or the undo stack. The assignments themselves live in
// History (task 85_3).
//
// The page is one of the Misc section's pages: Root switches to it and back to
// the section's contents (task 90). It follows the server's event stream: a
// label changed in another tab is reread, and so is the list on a resync.
package labels

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// presets are the colors the palette offers; a label may also take any
// #RRGGBB value or no color at all.
var presets = []string{"#ef4444", "#f59e0b", "#22c55e", "#3b82f6", "#a855f7", "#64748b"}

// Page is the Misc section's Labels page. The section owns which page it shows
// and wires OnBack; the page keeps its list and the field being edited.
type Page struct {
	app.Compo

	client *apiclient.Client
	// Shown is whether the page is the one the Misc section shows; the section
	// sets it before rendering. Only a shown page takes the keyboard (task 90).
	Shown bool
	// OnBack asks the section to show its contents.
	OnBack func()

	compoCtx app.Context
	mounted  bool

	labels []apiv0.LabelResponse
	loaded bool
	loader uiutil.Reloader

	// The new label: its name, its optional color and the server's refusal.
	newName, newColor, newError string
	creating                    bool

	// The label renamed in place: its ID, the text and the server's refusal.
	editingID   uuid.UUID
	editText    string
	editError   string
	editBusy    bool
	paletteID   uuid.UUID
	paletteText string
	paletteErr  string
	paletteBusy bool

	// The label whose deletion is confirmed (uuid.Nil: none).
	deletingID  uuid.UUID
	deleting    bool
	deleteError string

	onKeyDown app.Func
}

// ActionOpen asks Root to show the Labels page. A mode fires it from the
// history screen's "Manage labels", where a labels menu is empty (task 85_3,
// task 90).
const ActionOpen = "labels.open"

// New creates the page; Root holds it for the tab's lifetime.
func New(client *apiclient.Client) *Page {
	return &Page{client: client}
}

func (p *Page) OnMount(ctx app.Context) {
	p.compoCtx, p.mounted = ctx, true
	// A load that was in flight when the page left dispatches into a
	// dismounted element; go-app drops it, leaving the reloader loading for
	// good. Start this mount with a clean one, since the old answer is lost
	// anyway.
	p.loader = uiutil.Reloader{}
	p.load()
	p.listenKeys()
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok || !eventlog.IsLabelEvent(ev.Type) {
			return
		}
		p.load()
	})
	ctx.Handle(eventlog.EventResync, func(ctx app.Context, _ app.Action) {
		p.load()
	})
}

func (p *Page) OnDismount() {
	p.mounted = false
	p.noKeys()
}

// Reload rereads the labels: the section calls it when it opens the page, so a
// page left closed for a while shows what the server holds now. The page loads
// through its own context, never through the caller's: the caller may be a
// menu row that is already gone when the answer arrives.
func (p *Page) Reload() {
	if p.mounted {
		p.load()
	}
}

// load rereads the labels. A failure is a toast, as every error of the UI is.
func (p *Page) load() {
	uiutil.Load(p.compoCtx, &p.loader, p.client.Labels, func(ctx app.Context, list []apiv0.LabelResponse) {
		p.labels, p.loaded = list, true
		ctx.Update()
	}, func(app.Context) { p.load() })
}

// ── Commands ──────────────────────────────────────────────────────────────────

// create adds the label of the footer row. An empty name is not sent: the
// server is the only validator, but a row of nothing is not a request.
func (p *Page) create(ctx app.Context) {
	if p.creating || p.busy() {
		return
	}
	name := strings.TrimSpace(p.newName)
	if name == "" {
		return
	}
	p.creating, p.newError = true, ""
	uiutil.Do(p.compoCtx, func(c context.Context) (apiv0.LabelResponse, error) {
		return p.client.CreateLabel(c, name, strings.TrimSpace(p.newColor))
	}, func(ctx app.Context, created apiv0.LabelResponse, err error) {
		p.creating = false
		if err != nil {
			p.newError = uiutil.ProblemText(err)
			ctx.Update()
			return
		}
		p.newName, p.newColor = "", ""
		p.load()
	})
}

// startRename opens the name field of the label; commitRename writes it.
func (p *Page) startRename(label apiv0.LabelResponse) {
	p.editingID, p.editText, p.editError = label.ID, label.Name, ""
	p.paletteID, p.deletingID = uuid.Nil, uuid.Nil
}

func (p *Page) cancelRename(ctx app.Context) {
	p.editingID, p.editText, p.editError = uuid.Nil, "", ""
	ctx.Update()
}

func (p *Page) commitRename(ctx app.Context) {
	if p.editBusy || p.editingID == uuid.Nil {
		return
	}
	name := strings.TrimSpace(p.editText)
	if name == "" {
		return
	}
	labelID := p.editingID
	p.editBusy, p.editError = true, ""
	uiutil.Do(p.compoCtx, func(c context.Context) (apiv0.LabelResponse, error) {
		return p.client.UpdateLabel(c, labelID, &name, nil)
	}, func(ctx app.Context, _ apiv0.LabelResponse, err error) {
		p.editBusy = false
		if err != nil {
			p.editError = uiutil.ProblemText(err)
			ctx.Update()
			return
		}
		p.editingID, p.editText = uuid.Nil, ""
		p.load()
	})
}

// openPalette opens the colors of the label; applyColor writes one and closes
// it.
func (p *Page) openPalette(label apiv0.LabelResponse) {
	p.paletteID, p.paletteText, p.paletteErr = label.ID, label.Color, ""
	p.editingID, p.deletingID = uuid.Nil, uuid.Nil
}

func (p *Page) closePalette(ctx app.Context) {
	p.paletteID, p.paletteText, p.paletteErr = uuid.Nil, "", ""
	ctx.Update()
}

func (p *Page) applyColor(ctx app.Context, color string) {
	if p.paletteBusy || p.paletteID == uuid.Nil {
		return
	}
	labelID := p.paletteID
	p.paletteBusy, p.paletteErr = true, ""
	uiutil.Do(p.compoCtx, func(c context.Context) (apiv0.LabelResponse, error) {
		return p.client.UpdateLabel(c, labelID, nil, &color)
	}, func(ctx app.Context, _ apiv0.LabelResponse, err error) {
		p.paletteBusy = false
		if err != nil {
			p.paletteErr = uiutil.ProblemText(err)
			ctx.Update()
			return
		}
		p.paletteID, p.paletteText = uuid.Nil, ""
		p.load()
	})
}

// startDelete asks for the confirmation of the label's removal.
func (p *Page) startDelete(label apiv0.LabelResponse) {
	p.deletingID, p.deleteError = label.ID, ""
	p.editingID, p.paletteID = uuid.Nil, uuid.Nil
}

func (p *Page) cancelDelete(ctx app.Context) {
	p.deletingID, p.deleteError = uuid.Nil, ""
	ctx.Update()
}

func (p *Page) commitDelete(ctx app.Context) {
	if p.deleting || p.deletingID == uuid.Nil {
		return
	}
	labelID := p.deletingID
	p.deleting, p.deleteError = true, ""
	uiutil.Call(p.compoCtx, func(c context.Context) (struct{}, error) {
		return struct{}{}, p.client.DeleteLabel(c, labelID)
	}, func(ctx app.Context, _ struct{}) {
		p.deleting = false
		p.deletingID = uuid.Nil
		p.load()
	}, func(ctx app.Context, err error) {
		p.deleting = false
		p.deleteError = uiutil.ProblemText(err)
		ctx.Update()
	})
}

// busy reports whether a write of the page is in flight.
func (p *Page) busy() bool {
	return p.creating || p.editBusy || p.paletteBusy || p.deleting
}

// ── Keyboard ──────────────────────────────────────────────────────────────────

// pageKey is what a keyboard event does on the page.
type pageKey int

const (
	pageKeyNone pageKey = iota
	pageKeyCreate
	pageKeyCancel
)

// keyAction is the action of a key while the page is shown: Ctrl/Cmd+Enter
// adds the label of the footer row, and Esc drops a field being edited. A page
// not shown (the Misc section on another page) takes no keys.
func (p *Page) keyAction(key string, ctrl, meta bool) pageKey {
	if !p.Shown {
		return pageKeyNone
	}
	if ctrl || meta {
		if key == "Enter" {
			return pageKeyCreate
		}
		return pageKeyNone
	}
	if key != "Escape" {
		return pageKeyNone
	}
	if p.editingID == uuid.Nil && p.paletteID == uuid.Nil && p.deletingID == uuid.Nil && p.newError == "" {
		return pageKeyNone
	}
	return pageKeyCancel
}

// listenKeys manages the keys of the page while it is shown. A field of the
// page stops the propagation of its own keys, so Esc and Enter inside one never
// reach these two.
func (p *Page) listenKeys() {
	if p.onKeyDown != nil {
		return
	}
	p.onKeyDown = app.FuncOf(func(this app.Value, args []app.Value) any {
		if len(args) == 0 {
			return nil
		}
		ev := args[0]
		switch p.keyAction(ev.Get("key").String(), ev.Get("ctrlKey").Bool(), ev.Get("metaKey").Bool()) {
		case pageKeyCreate:
			ev.Call("preventDefault")
			p.compoCtx.Dispatch(func(ctx app.Context) { p.create(ctx) })
		case pageKeyCancel:
			ev.Call("preventDefault")
			p.compoCtx.Dispatch(func(ctx app.Context) {
				p.editingID, p.paletteID, p.deletingID = uuid.Nil, uuid.Nil, uuid.Nil
				p.editError, p.paletteErr, p.deleteError, p.newError = "", "", "", ""
				ctx.Update()
			})
		}
		return nil
	})
	app.Window().Call("addEventListener", "keydown", p.onKeyDown)
}

func (p *Page) noKeys() {
	if p.onKeyDown == nil {
		return
	}
	app.Window().Call("removeEventListener", "keydown", p.onKeyDown)
	p.onKeyDown.Release()
	p.onKeyDown = nil
}

// ── Render ────────────────────────────────────────────────────────────────────

// Render draws the page: the way back to the Misc section and the title, then
// the labels in a list that scrolls in its own pane, and the new-label row.
func (p *Page) Render() app.UI {
	rows := []app.UI{}
	switch {
	case !p.loaded:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Style("color", "var(--text-muted)").
			Text("Loading labels…"))
	case len(p.labels) == 0:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Style("color", "var(--text-muted)").
			Text("No labels yet."))
	default:
		for _, l := range p.labels {
			rows = append(rows, p.renderRow(l))
		}
	}
	return app.Div().
		Attr("data-labels-page", "true").
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-height", "0").
		Style("font-size", "13px").
		Body(
			uiutil.PageBar("Labels", "Back to the Misc section",
				func(ctx app.Context, e app.Event) {
					if p.OnBack != nil {
						p.OnBack()
					}
				}),
			app.Div().
				Attr("data-labels-list", "true").
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow-y", "auto").
				Style("padding", "4px 10px").
				Body(rows...),
			p.renderFooter(),
		)
}

// renderRow is one label: its colored marker, its name — edited in place —,
// the number of saved states it marks, and its color and delete actions.
func (p *Page) renderRow(l apiv0.LabelResponse) app.UI {
	name := []app.UI{}
	if p.editingID == l.ID {
		name = append(name,
			app.Input().
				Attr("data-label-rename-input", "true").
				Type("text").
				Value(p.editText).
				Placeholder("Name").
				AutoFocus(true).
				Style("flex", "1").
				Style("padding", "2px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, _ app.Event) {
					p.editText = ctx.JSSrc().Get("value").String()
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					switch e.Get("key").String() {
					case "Enter":
						// The page adds a label on Ctrl/Cmd+Enter: the
						// field commits alone.
						e.StopImmediatePropagation()
						p.commitRename(ctx)
					case "Escape":
						// The page takes Esc as a drop of the change: the
						// field drops it alone.
						e.StopImmediatePropagation()
						p.cancelRename(ctx)
					}
				}),
			uiutil.IconButton(uiutil.IconCheck(), "ghost", !p.editBusy, "Save the name", uiutil.ButtonBar,
				func(ctx app.Context, e app.Event) { p.commitRename(ctx) }).
				Attr("data-label-rename-save", "true"),
			uiutil.IconButton(uiutil.IconClose(), "ghost", !p.editBusy, "Drop the change", uiutil.ButtonBar,
				func(ctx app.Context, e app.Event) { p.cancelRename(ctx) }).
				Attr("data-label-rename-cancel", "true"),
		)
	} else {
		name = append(name,
			renderChip(l.Name, l.Color),
			uiutil.IconButton(uiutil.IconPencil(), "ghost", !p.busy(), "Rename the label", uiutil.ButtonBar,
				func(ctx app.Context, e app.Event) { p.startRename(l); ctx.Update() },
				app.EventScope("rename/"+l.ID.String())).
				Attr("data-label-rename", "true"),
		)
	}

	details := []app.UI{
		app.Div().
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "6px").
			Style("flex", "1").
			Style("min-width", "0").
			Body(append([]app.UI{marker(l.Color)}, name...)...),
		app.Span().
			Attr("data-label-assigned", "true").
			Title("Saved states this label marks").
			Style("color", "var(--text-muted)").
			Text(fmt.Sprintf("%d", l.AssignedCount)),
		uiutil.Button("Color", "", !p.busy(), "Change the color", uiutil.ButtonRow,
			func(ctx app.Context, e app.Event) { p.openPalette(l); ctx.Update() },
			app.EventScope("color/"+l.ID.String())).
			Attr("data-label-color", "true"),
		uiutil.Button("Delete", "error", !p.busy(), "Delete the label", uiutil.ButtonRow,
			func(ctx app.Context, e app.Event) { p.startDelete(l); ctx.Update() },
			app.EventScope("delete/"+l.ID.String())).
			Attr("data-label-delete", "true"),
	}

	body := []app.UI{
		app.Div().
			Attr("data-label-row", l.ID.String()).
			Style("display", "flex").
			Style("align-items", "center").
			Style("gap", "8px").
			Style("padding", "6px 0").
			Style("border-bottom", "1px solid var(--border-subtle)").
			Body(details...),
	}
	if p.editError != "" && p.editingID == l.ID {
		body = append(body, errorLine(p.editError))
	}
	if p.paletteID == l.ID {
		body = append(body, p.renderPalette())
	}
	if p.deletingID == l.ID {
		body = append(body, p.renderDelete(l))
	}
	return app.Div().Body(body...)
}

// renderPalette is the colors of a label: the presets, a hex field and no
// color.
func (p *Page) renderPalette() app.UI {
	swatches := []app.UI{}
	for _, c := range presets {
		swatches = append(swatches, presetSwatch(c, func(ctx app.Context, e app.Event) { p.applyColor(ctx, c) }))
	}
	return app.Div().
		Attr("data-label-palette", "true").
		Style("display", "flex").
		Style("flex-wrap", "wrap").
		Style("align-items", "center").
		Style("gap", "6px").
		Style("padding", "4px 0 8px 20px").
		Body(append(swatches,
			app.Input().
				Attr("data-label-color-input", "true").
				Type("text").
				Value(p.paletteText).
				Placeholder("#RRGGBB").
				Style("width", "100px").
				Style("padding", "2px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, _ app.Event) {
					p.paletteText = ctx.JSSrc().Get("value").String()
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					if e.Get("key").String() == "Enter" {
						p.applyColor(ctx, p.paletteText)
					}
				}),
			uiutil.Button("Apply", "", !p.paletteBusy, "Apply this color", uiutil.ButtonRow,
				func(ctx app.Context, e app.Event) { p.applyColor(ctx, p.paletteText) }).
				Attr("data-label-color-apply", "true"),
			uiutil.Button("No color", "", !p.paletteBusy, "Clear the color", uiutil.ButtonRow,
				func(ctx app.Context, e app.Event) { p.applyColor(ctx, "") }).
				Attr("data-label-color-none", "true"),
			uiutil.Button("Close", "", !p.paletteBusy, "Close the colors", uiutil.ButtonRow,
				func(ctx app.Context, e app.Event) { p.closePalette(ctx) }),
			func() app.UI {
				if p.paletteErr != "" {
					return errorLine(p.paletteErr)
				}
				return app.Div()
			}(),
		)...)
}

// renderDelete is the confirmation of a delete, naming the label and warning
// how many saved states lose the mark.
func (p *Page) renderDelete(l apiv0.LabelResponse) app.UI {
	return app.Div().
		Attr("data-label-delete-confirm-panel", "true").
		Style("padding", "4px 0 10px 20px").
		Body(
			app.Div().
				Attr("data-label-delete-question", "true").
				Text(fmt.Sprintf("Delete the label %q? It is removed from %d saved states.", l.Name, l.AssignedCount)),
			app.Div().
				Style("display", "flex").
				Style("gap", "6px").
				Style("margin-top", "8px").
				Body(
					uiutil.Button("Delete", "error", !p.deleting, "Delete the label", uiutil.ButtonPage,
						func(ctx app.Context, e app.Event) { p.commitDelete(ctx) }).
						Attr("data-label-delete-confirm", "true"),
					uiutil.Button("Cancel", "", !p.deleting, "Keep the label", uiutil.ButtonPage,
						func(ctx app.Context, e app.Event) { p.cancelDelete(ctx) }),
				),
			func() app.UI {
				if p.deleteError != "" {
					return errorLine(p.deleteError)
				}
				return app.Div()
			}(),
		)
}

// renderFooter is the new-label row: its name, an optional color and Add.
func (p *Page) renderFooter() app.UI {
	swatches := []app.UI{}
	for _, c := range presets {
		swatches = append(swatches, presetSwatch(c, func(ctx app.Context, e app.Event) {
			p.newColor = c
			ctx.Update()
		}))
	}
	return app.Div().
		Style("padding", "8px 10px").
		Style("border-top", "1px solid var(--border)").
		Body(
			app.Div().
				Style("display", "flex").
				Style("align-items", "center").
				Style("gap", "6px").
				Body(
					app.Input().
						Attr("data-label-new-name", "true").
						Type("text").
						Value(p.newName).
						Placeholder("New label").
						Style("flex", "1").
						Style("padding", "4px 6px").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "4px").
						Style("background", "var(--input-bg)").
						Style("color", "var(--text)").
						OnInput(func(ctx app.Context, _ app.Event) {
							p.newName = ctx.JSSrc().Get("value").String()
						}).
						OnKeyDown(func(ctx app.Context, e app.Event) {
							if e.Get("key").String() == "Enter" {
								p.create(ctx)
							}
						}),
					app.Input().
						Attr("data-label-new-color", "true").
						Type("text").
						Value(p.newColor).
						Placeholder("#RRGGBB").
						Style("width", "90px").
						Style("padding", "4px 6px").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "4px").
						Style("background", "var(--input-bg)").
						Style("color", "var(--text)").
						OnInput(func(ctx app.Context, _ app.Event) {
							p.newColor = ctx.JSSrc().Get("value").String()
						}),
					uiutil.Button("Add", "accent", !p.creating, "Add the label", uiutil.ButtonPage,
						func(ctx app.Context, e app.Event) { p.create(ctx) }).
						Attr("data-label-add", "true"),
				),
			app.Div().
				Style("display", "flex").
				Style("align-items", "center").
				Style("gap", "6px").
				Style("margin-top", "8px").
				Body(append(swatches,
					app.Span().
						Style("color", "var(--text-muted)").
						Style("font-size", "12px").
						Text("Ctrl/Cmd+Enter adds; Esc drops the change."),
				)...),
			func() app.UI {
				if p.newError != "" {
					return errorLine(p.newError)
				}
				return app.Div()
			}(),
		)
}

// marker is the small dot of a label's color, neutral when it has none.
func marker(color string) app.UI {
	fill := "var(--border)"
	if color != "" {
		fill = color
	}
	return app.Span().
		Style("display", "inline-block").
		Style("width", "10px").
		Style("height", "10px").
		Style("flex-shrink", "0").
		Style("border-radius", "50%").
		Style("background", fill)
}

// renderChip is a label's name: its border is the label's color and the text
// the theme's, so the color never is the only difference (ADR 0025).
func renderChip(name, color string) app.UI {
	border, text := "var(--border)", "var(--text-muted)"
	if color != "" {
		border, text = color, "var(--text)"
	}
	return app.Span().
		Attr("data-label-chip", color).
		Style("padding", "1px 8px").
		Style("border", "1px solid "+border).
		Style("border-radius", "10px").
		Style("color", text).
		Style("word-break", "break-word").
		Text(name)
}

// presetSwatch is one color of the palette: the color itself is the face.
func presetSwatch(color string, onClick func(app.Context, app.Event)) app.UI {
	return app.Button().
		Attr("data-label-preset", color).
		Attr("aria-label", color).
		Title(color).
		Style("width", "16px").
		Style("height", "16px").
		Style("padding", "0").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "3px").
		Style("background", color).
		Style("cursor", "pointer").
		OnClick(onClick)
}

// errorLine is a server refusal under the field it belongs to.
func errorLine(text string) app.UI {
	return app.Div().
		Attr("data-label-error", "true").
		Style("color", "var(--error)").
		Style("font-size", "12px").
		Style("margin-top", "4px").
		Text(text)
}
