// Package labels is the one global window that manages the server's Labels:
// their names and colors (task 85_2; ADR 0025). Labels are marks an Engineer
// puts on saved states; they belong to the Organization and are metadata, so
// the window creates, renames, recolors and deletes names, and never touches a
// Draft, a Version or the undo stack. The assignments themselves live in
// History (task 85_3).
//
// The window is opened from the header of every mode (Root owns whether it is
// open) and follows the server's event stream: a label changed in another tab
// is reread, and so is the list on a resync.
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
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// presets are the colors the palette offers; a label may also take any
// #RRGGBB value or no color at all.
var presets = []string{"#ef4444", "#f59e0b", "#22c55e", "#3b82f6", "#a855f7", "#64748b"}

// Dialog is the global Labels window. Root owns Open (and renders the window
// on every screen); the dialog keeps its list and the field being edited.
type Dialog struct {
	app.Compo

	client *apiclient.Client
	// Open is whether the window is shown; Root sets it before rendering.
	Open bool
	// OnClose asks Root to close the window.
	OnClose func()

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

// New creates the window; Root holds it for the tab's lifetime.
func New(client *apiclient.Client) *Dialog {
	return &Dialog{client: client}
}

func (d *Dialog) OnMount(ctx app.Context) {
	d.compoCtx, d.mounted = ctx, true
	d.load(ctx)
	d.listenKeys()
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok || !isLabelEvent(ev.Type) {
			return
		}
		d.load(ctx)
	})
	ctx.Handle(eventlog.EventResync, func(ctx app.Context, _ app.Action) {
		d.load(ctx)
	})
}

func (d *Dialog) OnDismount() {
	d.mounted = false
	d.noKeys()
}

// Reload rereads the labels: Root calls it when it opens the window, so a
// window left closed for a while shows what the server holds now.
func (d *Dialog) Reload(ctx app.Context) {
	if d.mounted {
		d.load(ctx)
	}
}

// isLabelEvent reports whether an event of the server's stream is a change of
// a label.
func isLabelEvent(eventType string) bool {
	switch eventType {
	case apiv0.EventLabelCreated, apiv0.EventLabelUpdated, apiv0.EventLabelDeleted:
		return true
	}
	return false
}

// load rereads the labels. A failure is a toast, as every error of the UI is.
func (d *Dialog) load(ctx app.Context) {
	uiutil.Load(ctx, &d.loader, d.client.Labels, func(ctx app.Context, list []apiv0.LabelResponse) {
		d.labels, d.loaded = list, true
		ctx.Update()
	}, d.load)
}

// ── Commands ──────────────────────────────────────────────────────────────────

// create adds the label of the footer row. An empty name is not sent: the
// server is the only validator, but a row of nothing is not a request.
func (d *Dialog) create(ctx app.Context) {
	if d.creating || d.busy() {
		return
	}
	name := strings.TrimSpace(d.newName)
	if name == "" {
		return
	}
	d.creating, d.newError = true, ""
	uiutil.Do(d.compoCtx, func(c context.Context) (apiv0.LabelResponse, error) {
		return d.client.CreateLabel(c, name, strings.TrimSpace(d.newColor))
	}, func(ctx app.Context, created apiv0.LabelResponse, err error) {
		d.creating = false
		if err != nil {
			d.newError = problemText(err)
			ctx.Update()
			return
		}
		d.newName, d.newColor = "", ""
		d.load(ctx)
	})
}

// startRename opens the name field of the label; commitRename writes it.
func (d *Dialog) startRename(label apiv0.LabelResponse) {
	d.editingID, d.editText, d.editError = label.ID, label.Name, ""
	d.paletteID, d.deletingID = uuid.Nil, uuid.Nil
}

func (d *Dialog) cancelRename(ctx app.Context) {
	d.editingID, d.editText, d.editError = uuid.Nil, "", ""
	ctx.Update()
}

func (d *Dialog) commitRename(ctx app.Context) {
	if d.editBusy || d.editingID == uuid.Nil {
		return
	}
	name := strings.TrimSpace(d.editText)
	if name == "" {
		return
	}
	labelID := d.editingID
	d.editBusy, d.editError = true, ""
	uiutil.Do(d.compoCtx, func(c context.Context) (apiv0.LabelResponse, error) {
		return d.client.UpdateLabel(c, labelID, &name, nil)
	}, func(ctx app.Context, _ apiv0.LabelResponse, err error) {
		d.editBusy = false
		if err != nil {
			d.editError = problemText(err)
			ctx.Update()
			return
		}
		d.editingID, d.editText = uuid.Nil, ""
		d.load(ctx)
	})
}

// openPalette opens the colors of the label; applyColor writes one and closes
// it.
func (d *Dialog) openPalette(label apiv0.LabelResponse) {
	d.paletteID, d.paletteText, d.paletteErr = label.ID, label.Color, ""
	d.editingID, d.deletingID = uuid.Nil, uuid.Nil
}

func (d *Dialog) closePalette(ctx app.Context) {
	d.paletteID, d.paletteText, d.paletteErr = uuid.Nil, "", ""
	ctx.Update()
}

func (d *Dialog) applyColor(ctx app.Context, color string) {
	if d.paletteBusy || d.paletteID == uuid.Nil {
		return
	}
	labelID := d.paletteID
	d.paletteBusy, d.paletteErr = true, ""
	uiutil.Do(d.compoCtx, func(c context.Context) (apiv0.LabelResponse, error) {
		return d.client.UpdateLabel(c, labelID, nil, &color)
	}, func(ctx app.Context, _ apiv0.LabelResponse, err error) {
		d.paletteBusy = false
		if err != nil {
			d.paletteErr = problemText(err)
			ctx.Update()
			return
		}
		d.paletteID, d.paletteText = uuid.Nil, ""
		d.load(ctx)
	})
}

// startDelete asks for the confirmation of the label's removal.
func (d *Dialog) startDelete(label apiv0.LabelResponse) {
	d.deletingID, d.deleteError = label.ID, ""
	d.editingID, d.paletteID = uuid.Nil, uuid.Nil
}

func (d *Dialog) cancelDelete(ctx app.Context) {
	d.deletingID, d.deleteError = uuid.Nil, ""
	ctx.Update()
}

func (d *Dialog) commitDelete(ctx app.Context) {
	if d.deleting || d.deletingID == uuid.Nil {
		return
	}
	labelID := d.deletingID
	d.deleting, d.deleteError = true, ""
	uiutil.Call(d.compoCtx, func(c context.Context) (struct{}, error) {
		return struct{}{}, d.client.DeleteLabel(c, labelID)
	}, func(ctx app.Context, _ struct{}) {
		d.deleting = false
		d.deletingID = uuid.Nil
		d.load(ctx)
	}, func(ctx app.Context, err error) {
		d.deleting = false
		d.deleteError = problemText(err)
		ctx.Update()
	})
}

// busy reports whether a write of the window is in flight.
func (d *Dialog) busy() bool {
	return d.creating || d.editBusy || d.paletteBusy || d.deleting
}

// ── Keyboard ──────────────────────────────────────────────────────────────────

// listenKeys manages the keys of the window: Ctrl/Cmd+Enter adds the label of
// the footer row, and Esc closes a field being edited first and then the
// window. A field of the window stops the propagation of its own keys, so Esc
// and Enter inside one never reach these two.
func (d *Dialog) listenKeys() {
	if d.onKeyDown != nil {
		return
	}
	d.onKeyDown = app.FuncOf(func(this app.Value, args []app.Value) any {
		if len(args) == 0 || !d.Open {
			return nil
		}
		ev := args[0]
		key := ev.Get("key").String()
		if key != "Escape" && key != "Enter" {
			return nil
		}
		if ev.Get("ctrlKey").Bool() || ev.Get("metaKey").Bool() {
			if key != "Enter" {
				return nil
			}
			ev.Call("preventDefault")
			d.compoCtx.Dispatch(func(ctx app.Context) { d.create(ctx) })
			return nil
		}
		if key != "Escape" {
			return nil
		}
		ev.Call("preventDefault")
		if d.editingID != uuid.Nil || d.paletteID != uuid.Nil || d.deletingID != uuid.Nil || d.newError != "" {
			d.compoCtx.Dispatch(func(ctx app.Context) {
				d.editingID, d.paletteID, d.deletingID = uuid.Nil, uuid.Nil, uuid.Nil
				d.editError, d.paletteErr, d.deleteError, d.newError = "", "", "", ""
				ctx.Update()
			})
			return nil
		}
		d.compoCtx.Dispatch(func(ctx app.Context) { d.close(ctx) })
		return nil
	})
	app.Window().Call("addEventListener", "keydown", d.onKeyDown)
}

func (d *Dialog) noKeys() {
	if d.onKeyDown == nil {
		return
	}
	app.Window().Call("removeEventListener", "keydown", d.onKeyDown)
	d.onKeyDown.Release()
	d.onKeyDown = nil
}

// close asks Root to close the window.
func (d *Dialog) close(ctx app.Context) {
	if d.OnClose != nil {
		d.OnClose()
	}
	ctx.Update()
}

// ── Render ────────────────────────────────────────────────────────────────────

func (d *Dialog) Render() app.UI {
	if !d.Open {
		return app.Div()
	}
	rows := []app.UI{}
	switch {
	case !d.loaded:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Style("color", "var(--text-muted)").
			Text("Loading labels…"))
	case len(d.labels) == 0:
		rows = append(rows, app.Div().
			Style("padding", "8px").
			Style("color", "var(--text-muted)").
			Text("No labels yet."))
	default:
		for _, l := range d.labels {
			rows = append(rows, d.renderRow(l))
		}
	}
	return app.Div().
		Attr("data-labels-dialog", "true").
		Style("position", "fixed").
		Style("inset", "0").
		Style("z-index", "40").
		Style("display", "flex").
		Style("align-items", "center").
		Style("justify-content", "center").
		Style("background", "rgba(0,0,0,0.35)").
		Body(
			app.Div().
				Attr("role", "dialog").
				Attr("aria-modal", "true").
				Attr("aria-label", "Labels").
				Style("width", "560px").
				Style("max-width", "calc(100vw - 32px)").
				Style("max-height", "calc(100vh - 64px)").
				Style("display", "flex").
				Style("flex-direction", "column").
				Style("border", "1px solid var(--border)").
				Style("border-radius", "6px").
				Style("background", "var(--bg)").
				Style("box-shadow", "var(--toast-shadow)").
				Style("font-size", "13px").
				Body(
					app.Div().
						Style("display", "flex").
						Style("align-items", "center").
						Style("gap", "8px").
						Style("padding", "10px 14px").
						Style("border-bottom", "1px solid var(--border)").
						Body(
							uiutil.IconTag(),
							app.Span().
								Style("font-size", "14px").
								Style("font-weight", "600").
								Text("Labels"),
							app.Div().Style("flex", "1"),
							uiutil.IconButton(uiutil.IconClose(), "ghost", true, "Close the Labels window",
								uiutil.ButtonBar, func(ctx app.Context, e app.Event) { d.close(ctx) }).
								Attr("data-labels-close", "true"),
						),
					app.Div().
						Attr("data-labels-list", "true").
						Style("flex", "1").
						Style("min-height", "0").
						Style("overflow-y", "auto").
						Style("padding", "6px 14px").
						Body(rows...),
					d.renderFooter(),
				),
		)
}

// renderRow is one label: its colored marker, its name — edited in place —,
// the number of saved states it marks, and its color and delete actions.
func (d *Dialog) renderRow(l apiv0.LabelResponse) app.UI {
	name := []app.UI{}
	if d.editingID == l.ID {
		name = append(name,
			app.Input().
				Attr("data-label-rename-input", "true").
				Type("text").
				Value(d.editText).
				Placeholder("Name").
				AutoFocus(true).
				Style("flex", "1").
				Style("padding", "2px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, _ app.Event) {
					d.editText = ctx.JSSrc().Get("value").String()
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					switch e.Get("key").String() {
					case "Enter":
						// The window adds a label on Ctrl/Cmd+Enter: the
						// field commits alone.
						e.StopImmediatePropagation()
						d.commitRename(ctx)
					case "Escape":
						// The window closes on Esc: the field drops the change
						// alone.
						e.StopImmediatePropagation()
						d.cancelRename(ctx)
					}
				}),
			uiutil.IconButton(uiutil.IconCheck(), "ghost", !d.editBusy, "Save the name", uiutil.ButtonBar,
				func(ctx app.Context, e app.Event) { d.commitRename(ctx) }).
				Attr("data-label-rename-save", "true"),
			uiutil.IconButton(uiutil.IconClose(), "ghost", !d.editBusy, "Drop the change", uiutil.ButtonBar,
				func(ctx app.Context, e app.Event) { d.cancelRename(ctx) }).
				Attr("data-label-rename-cancel", "true"),
		)
	} else {
		name = append(name,
			renderChip(l.Name, l.Color),
			uiutil.IconButton(uiutil.IconPencil(), "ghost", !d.busy(), "Rename the label", uiutil.ButtonBar,
				func(ctx app.Context, e app.Event) { d.startRename(l); ctx.Update() },
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
		uiutil.Button("Color", "", !d.busy(), "Change the color", uiutil.ButtonRow,
			func(ctx app.Context, e app.Event) { d.openPalette(l); ctx.Update() },
			app.EventScope("color/"+l.ID.String())).
			Attr("data-label-color", "true"),
		uiutil.Button("Delete", "error", !d.busy(), "Delete the label", uiutil.ButtonRow,
			func(ctx app.Context, e app.Event) { d.startDelete(l); ctx.Update() },
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
	if d.editError != "" && d.editingID == l.ID {
		body = append(body, errorLine(d.editError))
	}
	if d.paletteID == l.ID {
		body = append(body, d.renderPalette())
	}
	if d.deletingID == l.ID {
		body = append(body, d.renderDelete(l))
	}
	return app.Div().Body(body...)
}

// renderPalette is the colors of a label: the presets, a hex field and no
// color.
func (d *Dialog) renderPalette() app.UI {
	swatches := []app.UI{}
	for _, c := range presets {
		swatches = append(swatches, presetSwatch(c, func(ctx app.Context, e app.Event) { d.applyColor(ctx, c) }))
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
				Value(d.paletteText).
				Placeholder("#RRGGBB").
				Style("width", "100px").
				Style("padding", "2px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, _ app.Event) {
					d.paletteText = ctx.JSSrc().Get("value").String()
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					if e.Get("key").String() == "Enter" {
						d.applyColor(ctx, d.paletteText)
					}
				}),
			uiutil.Button("Apply", "", !d.paletteBusy, "Apply this color", uiutil.ButtonRow,
				func(ctx app.Context, e app.Event) { d.applyColor(ctx, d.paletteText) }).
				Attr("data-label-color-apply", "true"),
			uiutil.Button("No color", "", !d.paletteBusy, "Clear the color", uiutil.ButtonRow,
				func(ctx app.Context, e app.Event) { d.applyColor(ctx, "") }).
				Attr("data-label-color-none", "true"),
			uiutil.Button("Close", "", !d.paletteBusy, "Close the colors", uiutil.ButtonRow,
				func(ctx app.Context, e app.Event) { d.closePalette(ctx) }),
			func() app.UI {
				if d.paletteErr != "" {
					return errorLine(d.paletteErr)
				}
				return app.Div()
			}(),
		)...)
}

// renderDelete is the confirmation of a delete, naming the label and warning
// how many saved states lose the mark.
func (d *Dialog) renderDelete(l apiv0.LabelResponse) app.UI {
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
					uiutil.Button("Delete", "error", !d.deleting, "Delete the label", uiutil.ButtonPage,
						func(ctx app.Context, e app.Event) { d.commitDelete(ctx) }).
						Attr("data-label-delete-confirm", "true"),
					uiutil.Button("Cancel", "", !d.deleting, "Keep the label", uiutil.ButtonPage,
						func(ctx app.Context, e app.Event) { d.cancelDelete(ctx) }),
				),
			func() app.UI {
				if d.deleteError != "" {
					return errorLine(d.deleteError)
				}
				return app.Div()
			}(),
		)
}

// renderFooter is the new-label row: its name, an optional color and Add.
func (d *Dialog) renderFooter() app.UI {
	swatches := []app.UI{}
	for _, c := range presets {
		swatches = append(swatches, presetSwatch(c, func(ctx app.Context, e app.Event) {
			d.newColor = c
			ctx.Update()
		}))
	}
	return app.Div().
		Style("padding", "10px 14px").
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
						Value(d.newName).
						Placeholder("New label").
						Style("flex", "1").
						Style("padding", "4px 6px").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "4px").
						Style("background", "var(--input-bg)").
						Style("color", "var(--text)").
						OnInput(func(ctx app.Context, _ app.Event) {
							d.newName = ctx.JSSrc().Get("value").String()
						}).
						OnKeyDown(func(ctx app.Context, e app.Event) {
							if e.Get("key").String() == "Enter" {
								d.create(ctx)
							}
						}),
					app.Input().
						Attr("data-label-new-color", "true").
						Type("text").
						Value(d.newColor).
						Placeholder("#RRGGBB").
						Style("width", "90px").
						Style("padding", "4px 6px").
						Style("border", "1px solid var(--border-input)").
						Style("border-radius", "4px").
						Style("background", "var(--input-bg)").
						Style("color", "var(--text)").
						OnInput(func(ctx app.Context, _ app.Event) {
							d.newColor = ctx.JSSrc().Get("value").String()
						}),
					uiutil.Button("Add", "accent", !d.creating, "Add the label", uiutil.ButtonPage,
						func(ctx app.Context, e app.Event) { d.create(ctx) }).
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
						Text("Ctrl/Cmd+Enter adds; Esc closes."),
				)...),
			func() app.UI {
				if d.newError != "" {
					return errorLine(d.newError)
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

// problemText is a refusal of the server as the window shows it: its detail,
// or its title when it carries none.
func problemText(err error) string {
	problem := toast.FromError(err)
	if problem.Detail != "" {
		return problem.Detail
	}
	return problem.Title
}
