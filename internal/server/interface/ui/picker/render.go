package picker

import (
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Render ────────────────────────────────────────────────────────────────────

// Render draws the two panes of the page: the list of the entities and the
// details of the selected one. Which of them a phone shows is the attribute
// data-screen; a desktop screen shows both.
func (p *Picker) Render() app.UI {
	return app.Div().
		Attr("data-picker", p.source.Singular()).
		Attr("data-screen", p.screenName()).
		Attr("data-sortable", boolAttr(p.sortable())).
		Attr("data-dragging", boolAttr(p.dragging)).
		Body(
			app.Style().Text(pickerCSS),
			p.renderListPane(),
			p.renderDetailsPane(),
		)
}

// screenName is the pane a phone shows: the details whenever there is
// something to show there.
func (p *Picker) screenName() string {
	if p.screen == screenDetails && (p.creating || p.selected != uuid.Nil) {
		return screenDetails
	}
	return screenList
}

// sortable reports whether the entities can be arranged: the source carries a
// per-user order, and a draft row cannot be dragged.
func (p *Picker) sortable() bool {
	if p.creating {
		return false
	}
	_, ok := p.source.(OrderSource)
	return ok
}

// renderListPane is the left pane: the header with Create and the rows.
func (p *Picker) renderListPane() app.UI {
	return app.Div().
		Attr("data-picker-list", "true").
		Body(
			app.Div().
				Attr("data-picker-list-header", "true").
				Body(
					app.H2().
						Style("margin", "0").
						Style("font-size", "16px").
						Style("white-space", "nowrap").
						Style("overflow", "hidden").
						Style("text-overflow", "ellipsis").
						Text(p.source.Title()),
					app.Div().Style("flex", "1"),
					uiutil.IconButton(iconPlus(), "accent", !p.busy(),
						"Create a new "+p.source.Singular(), uiutil.ButtonPage,
						func(ctx app.Context, _ app.Event) { p.startCreate(ctx) }),
				),
			app.Div().
				Attr("data-picker-rows", "true").
				Attr("id", p.rowsID()).
				Attr("role", "listbox").
				Attr("aria-label", p.source.Title()).
				TabIndex(0).
				OnKeyDown(p.onRowsKey).
				Body(p.renderRows()...),
		)
}

// renderRows draws the draft row of a new entity, the entities in the order
// the page shows them, or the placeholder of an empty list.
func (p *Picker) renderRows() []app.UI {
	shown := p.Displayed()
	rows := make([]app.UI, 0, len(shown)+1)
	if p.creating {
		rows = append(rows, p.renderDraftRow())
	}
	if len(shown) == 0 {
		if !p.creating {
			rows = append(rows, p.renderListPlaceholder())
		}
		return rows
	}
	for i, item := range shown {
		rows = append(rows, p.renderRow(i, item))
	}
	return rows
}

// renderListPlaceholder is the quiet text of an empty list.
func (p *Picker) renderListPlaceholder() app.UI {
	text := "No " + p.source.Plural() + " yet."
	if !p.loaded {
		text = "Loading " + p.source.Plural() + "…"
	}
	return app.Div().
		Style("font-size", "13px").
		Style("color", "var(--text-muted)").
		Style("padding", "8px 0").
		Text(text)
}

// renderDraftRow is the row of a new entity whose name is not committed yet:
// it holds the draft in the list and brings the details pane back.
func (p *Picker) renderDraftRow() app.UI {
	text := strings.TrimSpace(p.createName)
	if text == "" {
		text = "New " + p.source.Singular()
	}
	return app.Div().
		Attr("data-picker-row", "draft").
		Attr("data-picker-draft", "1").
		Attr("role", "option").
		Attr("aria-selected", "true").
		Attr("data-picker-selected", "1").
		Style("font-style", "italic").
		Body(app.Span().Text(text)).
		OnClick(func(ctx app.Context, _ app.Event) {
			p.screen = screenDetails
			ctx.Update()
		})
}

// renderRow is one entity: its name, the dot of its state and, where the
// source has an order, the handle the drag starts on (task 82).
func (p *Picker) renderRow(index int, item Item) app.UI {
	id := item.ID
	body := []app.UI{}
	if p.sortable() {
		body = append(body, p.renderGrip(index, item))
	}
	body = append(body,
		app.Span().
			Style("flex", "1").
			Style("min-width", "0").
			Style("font-size", "14px").
			Style("font-weight", "600").
			Style("overflow", "hidden").
			Style("text-overflow", "ellipsis").
			Style("white-space", "nowrap").
			Text(item.Name),
	)
	if item.State != "" {
		body = append(body, p.renderStateDot(item))
	}
	selected := id == p.selected && !p.creating
	return app.Div().
		Attr("data-picker-row", id.String()).
		Attr("data-picker-index", strconv.Itoa(index)).
		Attr("id", p.rowID(id)).
		Attr("role", "option").
		Attr("aria-selected", boolAttr(selected)).
		Attr("data-picker-selected", selectedAttr(selected)).
		Body(body...).
		OnClick(func(ctx app.Context, _ app.Event) {
			p.selectByID(ctx, id)
		}, app.EventScope("select/"+id.String())).
		OnDragOver(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
		}).
		OnDrop(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
			from := p.dragFrom
			p.dragFrom, p.dragging = -1, false
			if from >= 0 {
				p.move(ctx, from, index)
			}
			ctx.Update()
			// The position is part of the identity: go-app reuses an element
			// whose handler it thinks is the same one.
		}, app.EventScope("drop/"+id.String()+"/"+strconv.Itoa(index)))
}

// renderGrip is the handle a drag starts on: the browser's own on a desktop
// screen, a pointer on a phone (task 82).
func (p *Picker) renderGrip(index int, item Item) app.UI {
	id := item.ID
	return app.Span().
		Attr("data-picker-grip", "1").
		Attr("title", "Drag to reorder").
		Draggable(true).
		OnDragStart(func(ctx app.Context, e app.Event) {
			p.dragFrom, p.dragging = index, true
			data := e.Get("dataTransfer")
			data.Set("effectAllowed", "move")
			data.Call("setData", "text/plain", id.String())
			ctx.Update()
		}, app.EventScope("movestart/"+id.String()+"/"+strconv.Itoa(index))).
		OnDragEnd(func(ctx app.Context, _ app.Event) {
			p.dragFrom, p.dragging = -1, false
			ctx.Update()
		}, app.EventScope("moveend/"+id.String()+"/"+strconv.Itoa(index))).
		Body(iconGrip())
}

// renderStateDot is the dot a row carries for the entity's state: the state
// in words is in the details pane, so the list stays names (task 82).
func (p *Picker) renderStateDot(item Item) app.UI {
	color, _ := stateBadge(item.StateKind)
	return app.Span().
		Attr("data-picker-dot", item.StateKind).
		Attr("title", item.State).
		Style("width", "7px").
		Style("height", "7px").
		Style("border-radius", "50%").
		Style("flex-shrink", "0").
		Style("background", color)
}

// renderDetailsPane is the right pane: the selected entity, the draft of a new
// one, or what to do when there is nothing to show.
func (p *Picker) renderDetailsPane() app.UI {
	return app.Div().
		Attr("data-picker-details", "true").
		Body(p.renderDetails())
}

func (p *Picker) renderDetails() app.UI {
	if p.creating {
		return p.renderCreate()
	}
	item, ok := p.find(p.selected)
	if !ok {
		if !p.loaded {
			return p.renderHint("Loading " + p.source.Plural() + "…")
		}
		if len(p.items) == 0 {
			return p.renderFirstCreate()
		}
		return p.renderHint("Select a " + p.source.Singular() + " to see its details.")
	}
	return app.Div().Body(
		p.renderBackRow(),
		p.renderNameBlock(item),
		p.renderDescriptionBlock(item),
		p.renderStateLine(item),
		p.renderErrorLine(p.editError),
		p.renderActions(item),
	)
}

// renderBackRow is the phone's way back to the list; a desktop screen does not
// show it.
func (p *Picker) renderBackRow() app.UI {
	return app.Div().
		Attr("data-picker-back", "true").
		Body(app.Span().Text("← " + p.source.Title())).
		OnClick(func(ctx app.Context, _ app.Event) {
			p.screen = screenList
			ctx.Update()
		})
}

// renderNameBlock is the name of the entity, edited in place by a double click
// (a desktop screen) or the pencil (a phone).
func (p *Picker) renderNameBlock(item Item) app.UI {
	var name app.UI
	if p.field == fieldName {
		name = app.Input().
			Attr("data-picker-name-input", "true").
			Attr("id", "picker-"+p.source.Singular()+"-name").
			Attr("name", "name").
			Type("text").
			Value(p.value).
			Placeholder("Name").
			AutoFocus(true).
			OnInput(func(ctx app.Context, _ app.Event) {
				p.value = ctx.JSSrc().Get("value").String()
			}).
			OnBlur(func(ctx app.Context, _ app.Event) {
				p.commitEdit(ctx)
			}).
			OnKeyDown(func(ctx app.Context, e app.Event) {
				switch e.Get("key").String() {
				case "Enter":
					p.commitEdit(ctx)
				case "Escape":
					p.cancelEdit(ctx)
				}
			})
	} else {
		name = app.H2().
			Attr("data-picker-name", "true").
			Style("margin", "0").
			Style("font-size", "20px").
			Style("word-break", "break-word").
			Text(item.Name).
			Title("Double-click to edit").
			OnDblClick(func(ctx app.Context, _ app.Event) {
				p.startEdit(ctx, fieldName)
			}, app.EventScope("edit-name/"+item.ID.String()))
	}
	var trailing app.UI
	if p.field == fieldName {
		trailing = p.renderCancelEdit()
	} else {
		trailing = uiutil.IconButton(iconPencil(), "", !p.busy(),
			"Change the name", uiutil.ButtonPage,
			func(ctx app.Context, _ app.Event) { p.startEdit(ctx, fieldName) })
	}
	return app.Div().
		Attr("data-picker-name-row", "true").
		Body(name, trailing)
}

// renderDescriptionBlock is the description, edited in place: Enter breaks the
// line, Ctrl+Enter or leaving the field saves it (task 82).
func (p *Picker) renderDescriptionBlock(item Item) app.UI {
	var description app.UI
	if p.field == fieldDescription {
		description = app.Textarea().
			Attr("data-picker-description-input", "true").
			Attr("id", "picker-"+p.source.Singular()+"-description").
			Attr("name", "description").
			Rows(3).
			Text(p.value).
			Placeholder(emptyDescription).
			AutoFocus(true).
			OnInput(func(ctx app.Context, _ app.Event) {
				p.value = ctx.JSSrc().Get("value").String()
			}).
			OnBlur(func(ctx app.Context, _ app.Event) {
				p.commitEdit(ctx)
			}).
			OnKeyDown(func(ctx app.Context, e app.Event) {
				switch {
				case e.Get("key").String() == "Escape":
					p.cancelEdit(ctx)
				case e.Get("key").String() == "Enter" && (e.Get("ctrlKey").Bool() || e.Get("metaKey").Bool()):
					e.PreventDefault()
					p.commitEdit(ctx)
				}
			})
	} else {
		empty := item.Description == ""
		text := item.Description
		if empty {
			text = emptyDescription
		}
		description = app.Div().
			Attr("data-picker-description", "true").
			Attr("data-empty", boolAttr(empty)).
			Style("white-space", "pre-wrap").
			Style("word-break", "break-word").
			Style("font-size", "13px").
			Text(text).
			Title("Double-click to edit").
			OnDblClick(func(ctx app.Context, _ app.Event) {
				p.startEdit(ctx, fieldDescription)
			}, app.EventScope("edit-description/"+item.ID.String()))
	}
	var trailing app.UI
	if p.field == fieldDescription {
		trailing = p.renderCancelEdit()
	} else {
		trailing = uiutil.IconButton(iconPencil(), "", !p.busy(),
			"Change the description", uiutil.ButtonPage,
			func(ctx app.Context, _ app.Event) { p.startEdit(ctx, fieldDescription) })
	}
	return app.Div().
		Attr("data-picker-description-row", "true").
		Body(description, trailing)
}

// renderCancelEdit is the visible way out of an inline edit: a phone has no
// Escape, and every tap outside the field saves instead of cancelling. The
// button keeps the field from blurring, so a tap on it never commits the edit
// it is cancelling (task 82).
func (p *Picker) renderCancelEdit() app.UI {
	return app.Div().
		Attr("data-picker-cancel", "1").
		OnMouseDown(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
		}).
		Body(uiutil.Button("Cancel", "", true, "Discard the change", uiutil.ButtonPage,
			func(ctx app.Context, _ app.Event) { p.cancelEdit(ctx) }))
}

// renderStateLine is the state of the entity in words, beside its dot.
func (p *Picker) renderStateLine(item Item) app.UI {
	if item.State == "" {
		return app.Div()
	}
	color, border := stateBadge(item.StateKind)
	return app.Div().
		Style("margin-top", "12px").
		Body(app.Span().
			Attr("data-picker-state", item.StateKind).
			Style("font-size", "10px").
			Style("padding", "1px 6px").
			Style("border-radius", "8px").
			Style("white-space", "nowrap").
			Style("color", color).
			Style("border", "1px solid "+border).
			Text(item.State))
}

// renderErrorLine is the server's refusal of the field being edited, or of the
// draft being created.
func (p *Picker) renderErrorLine(text string) app.UI {
	if text == "" {
		return app.Div()
	}
	return app.Div().
		Attr("role", "alert").
		Attr("data-picker-error", "true").
		Style("margin-top", "10px").
		Style("padding", "4px 8px").
		Style("font-size", "12px").
		Style("border", "1px solid var(--error-border)").
		Style("border-radius", "4px").
		Style("background", "var(--error-bg)").
		Style("color", "var(--error)").
		Text(text)
}

// renderActions is Open and Delete of the selected entity: icons alone, with
// their titles, on every screen (task 82).
func (p *Picker) renderActions(item Item) app.UI {
	id := item.ID
	return app.Div().
		Attr("data-picker-details-actions", "true").
		Body(
			uiutil.IconButton(iconOpen(), "accent", !p.busy(),
				"Open the "+p.source.Singular(), uiutil.ButtonPage,
				func(ctx app.Context, _ app.Event) {
					if p.busy() {
						return
					}
					p.open(ctx, id)
				}, app.EventScope("open/"+id.String())),
			uiutil.IconButton(iconTrash(), "error", !p.busy(),
				"Delete the "+p.source.Singular(), uiutil.ButtonPage,
				func(ctx app.Context, _ app.Event) {
					if p.busy() {
						return
					}
					p.deleteEntity(ctx, id)
				}, app.EventScope("delete/"+id.String())),
		)
}

// renderHint is the quiet text of a details pane with nothing to show.
func (p *Picker) renderHint(text string) app.UI {
	return app.Div().
		Attr("data-picker-hint", "true").
		Style("font-size", "13px").
		Style("color", "var(--text-muted)").
		Style("padding", "8px 0").
		Text(text)
}

// renderFirstCreate is the empty page: the way to create the first entity.
func (p *Picker) renderFirstCreate() app.UI {
	return app.Div().Body(
		p.renderHint("No "+p.source.Plural()+" yet."),
		app.Div().
			Style("margin-top", "10px").
			Body(uiutil.IconButton(iconPlus(), "accent", !p.busy(),
				"Create the first "+p.source.Singular(), uiutil.ButtonPage,
				func(ctx app.Context, _ app.Event) { p.startCreate(ctx) })),
	)
}

// renderCreate is the details pane in its "new" mode: the name is typed here,
// nothing else, and the draft row of the list waits for it (task 82).
func (p *Picker) renderCreate() app.UI {
	// Cancel runs before the field loses the focus, so a click on it never
	// commits the draft it is cancelling.
	cancel := app.Div().
		OnMouseDown(func(ctx app.Context, e app.Event) {
			e.PreventDefault()
		}).
		Body(uiutil.Button("Cancel", "", true, "Close without creating", uiutil.ButtonPage,
			func(ctx app.Context, _ app.Event) { p.cancelCreate(ctx) }))
	return app.Div().Body(
		p.renderBackRow(),
		app.H2().
			Style("margin", "0").
			Style("font-size", "20px").
			Text("New "+p.source.Singular()),
		app.Div().
			Style("margin-top", "12px").
			Body(app.Input().
				Attr("data-picker-create-name", "true").
				Attr("id", "picker-"+p.source.Singular()+"-create-name").
				Attr("name", "name").
				Type("text").
				Placeholder("Name").
				Value(p.createName).
				AutoFocus(true).
				OnInput(func(ctx app.Context, _ app.Event) {
					p.createName = ctx.JSSrc().Get("value").String()
				}).
				OnBlur(func(ctx app.Context, _ app.Event) {
					p.commitCreate(ctx)
				}).
				OnKeyDown(func(ctx app.Context, e app.Event) {
					switch e.Get("key").String() {
					case "Enter":
						p.commitCreate(ctx)
					case "Escape":
						p.cancelCreate(ctx)
					}
				})),
		p.renderErrorLine(p.createError),
		app.Div().
			Style("display", "flex").
			Style("gap", "6px").
			Style("margin-top", "12px").
			Body(
				uiutil.Button("Create", "accent", !p.createBusy, "Create the "+p.source.Singular(), uiutil.ButtonPage,
					func(ctx app.Context, _ app.Event) { p.commitCreate(ctx) }),
				cancel,
			),
	)
}

// stateBadge is the text colour and the border colour of a state's badge: the
// one place its palette lives.
func stateBadge(kind string) (color, border string) {
	switch kind {
	case "unsaved", "unpublished":
		return "var(--toast-warn-title)", "var(--toast-warn-border)"
	default:
		return "var(--text-muted)", "var(--border)"
	}
}

// rowsID and rowID name the list and its rows for the browser: the list is the
// active descendant owner of the keyboard, and a drag reads the row under the
// pointer.
func (p *Picker) rowsID() string { return "picker-" + p.source.Singular() + "-rows" }

func (p *Picker) rowID(id uuid.UUID) string {
	return "picker-" + p.source.Singular() + "-row-" + id.String()
}

// selectedAttr marks the row of the selected entity: "true" would be encoded as
// a bare attribute, which a selector cannot match.
func selectedAttr(selected bool) string {
	if selected {
		return "1"
	}
	return "0"
}
