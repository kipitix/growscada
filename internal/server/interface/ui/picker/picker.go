// Package picker is the selection page of the Library and Project modes
// (task 73): the list of the server's libraries or projects with Create,
// Open, Edit and Delete, and the Name+Description dialog they share. With no
// entity the page is the mode's whole screen; from an editor it is opened by
// the "← Список" row of Root.
//
// The page reads its list on mount, on every event that may change it
// (project_* or library_*, EventResync) and when it is shown again. A
// description is one line with its full text in the title; an entity without
// one says so quietly. Nothing is filtered or paged: the list is sorted by
// name here, and "no entity" is not a page of its own (task 73).
package picker

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// Item is one entity as the selection page shows it.
type Item struct {
	ID          uuid.UUID
	Name        string
	Description string
	// Version is the aggregate's own Version, stated by an update.
	Version int
	// State is a short badge ("unsaved changes", "published", …); empty for
	// an entity with nothing to say about its Draft.
	State string
	// StateKind says what the badge means for its colour: "unsaved",
	// "unpublished", "published", "legacy" or "new".
	StateKind string
}

// Source is the server side of a selection page: the Projects or the
// Libraries. It is the only place the page knows which entity it lists.
type Source interface {
	// Title heads the page ("Libraries", "Projects").
	Title() string
	// Singular names one entity in the dialog and the messages.
	Singular() string
	// Plural names them in the list's empty text.
	Plural() string
	// List reads every live entity.
	List(context.Context) ([]Item, error)
	// Create creates an entity and returns its ID.
	Create(ctx context.Context, name, description string) (uuid.UUID, error)
	// Update changes an entity's name and description, stating the Version it
	// was read at.
	Update(ctx context.Context, id uuid.UUID, version int, name, description string) error
	// Delete removes an entity, with its content.
	Delete(ctx context.Context, id uuid.UUID) error
	// DeleteQuestion is the confirmation of Delete, naming the entity.
	DeleteQuestion(name string) string
}

// emptyDescription is the quiet text of an entity without a description.
const emptyDescription = "No description"

// Picker is the selection page of one kind of entity.
type Picker struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount; async
	// results are dispatched through it.
	compoCtx app.Context
	// mounted is set in OnMount: a Reload before that finds the list already
	// read by it.
	mounted bool
	source  Source
	// key is the LocalStorage key of the selection, shared with the mode.
	key      string
	onSelect func(uuid.UUID, string)
	onOpen   func(uuid.UUID)

	items    []Item
	loaded   bool
	reloader uiutil.Reloader

	// selected is the entity the mode shows and its name as last read; the
	// page reports the pair to onSelect whenever either moves.
	selected     uuid.UUID
	selectedName string

	// dialog is the Name+Description dialog: editingID is the entity edited
	// (uuid.Nil: a new one), its fields, and the server's refusal shown inside
	// it, with the input kept (task 73).
	dialog            bool
	editingID         uuid.UUID
	dialogName        string
	dialogDescription string
	dialogError       string
	dialogBusy        bool
}

// New returns the selection page of source, keeping the selection under the
// LocalStorage key and reporting it to onSelect — the entity the mode shows,
// uuid.Nil for none — and to onOpen when the user opens one.
func New(source Source, key string, onSelect func(uuid.UUID, string), onOpen func(uuid.UUID)) *Picker {
	return &Picker{source: source, key: key, onSelect: onSelect, onOpen: onOpen}
}

func (p *Picker) OnMount(ctx app.Context) {
	p.compoCtx = ctx
	p.mounted = true
	p.selected = uiutil.LoadID(ctx, p.key)
	p.load(ctx)
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok {
			return
		}
		// A row reads the entity's name, its description and whether its Draft
		// moved since its last Commit. A create, an update or a delete of an
		// entity changes the list itself; a Save fixes the Draft of one, and
		// any write of a Draft's content makes one unsaved (task 73;
		// ReloadTargetsFor).
		if targets := eventlog.ReloadTargetsFor(ev); targets.Projects || targets.Libraries ||
			targets.Commits || targets.DraftChanged() {
			p.load(ctx)
		}
	})
}

// Reload rereads the list: the page is shown again. Before OnMount the list
// is not read yet, and OnMount reads it.
func (p *Picker) Reload(ctx app.Context) {
	if !p.mounted {
		return
	}
	p.load(ctx)
}

// Items returns the list as last loaded.
func (p *Picker) Items() []Item { return p.items }

// SelectedID returns the entity the page reports as shown (uuid.Nil: none).
func (p *Picker) SelectedID() uuid.UUID { return p.selected }

// EditingID returns the entity the dialog edits (uuid.Nil: a new one).
func (p *Picker) EditingID() uuid.UUID { return p.editingID }

// Name returns the name of the entity id as last read.
func (p *Picker) Name(id uuid.UUID) string {
	if item, ok := p.find(id); ok {
		return item.Name
	}
	if id == p.selected {
		return p.selectedName
	}
	return ""
}

func (p *Picker) find(id uuid.UUID) (Item, bool) {
	for _, item := range p.items {
		if item.ID == id {
			return item, true
		}
	}
	return Item{}, false
}

// load (re)reads the list and reconciles the selection with it: a client
// reload or a resync is its own, and an entity deleted elsewhere leaves
// nothing to show.
func (p *Picker) load(ctx app.Context) {
	uiutil.Load(p.compoCtx, &p.reloader, p.source.List, func(ctx app.Context, items []Item) {
		slices.SortFunc(items, func(a, b Item) int {
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		})
		p.items, p.loaded = items, true
		p.reconcile(ctx)
		ctx.Update()
	}, p.load)
}

// reconcile announces the selection the reloaded list holds: the same entity
// under a new name, or none when it is gone (root shows the page then).
func (p *Picker) reconcile(ctx app.Context) {
	if p.selected == uuid.Nil {
		return
	}
	item, ok := p.find(p.selected)
	if !ok {
		p.selectNone(ctx)
		return
	}
	p.report(ctx, item)
}

// report is the selection announcement of item.
func (p *Picker) report(ctx app.Context, item Item) {
	if p.selected == item.ID && p.selectedName == item.Name {
		return
	}
	p.selected, p.selectedName = item.ID, item.Name
	uiutil.StoreID(ctx, p.key, item.ID)
	if p.onSelect != nil {
		p.onSelect(item.ID, item.Name)
	}
}

// selectNone reports that no entity is selected, and forgets the key, so the
// next mount does not look for it.
func (p *Picker) selectNone(ctx app.Context) {
	if p.selected == uuid.Nil && p.selectedName == "" {
		return
	}
	p.selected, p.selectedName = uuid.Nil, ""
	uiutil.StoreID(ctx, p.key, uuid.Nil)
	if p.onSelect != nil {
		p.onSelect(uuid.Nil, "")
	}
}

// open shows the editor of id: the Open row, or a created entity.
func (p *Picker) open(ctx app.Context, id uuid.UUID) {
	if item, ok := p.find(id); ok {
		p.report(ctx, item)
	} else if id != uuid.Nil {
		p.selected, p.selectedName = id, ""
		if p.onSelect != nil {
			p.onSelect(id, "")
		}
	}
	uiutil.StoreID(ctx, p.key, id)
	if p.onOpen != nil {
		p.onOpen(id)
	}
}

// openDialog opens the Name+Description dialog of id (uuid.Nil: a new entity).
func (p *Picker) openDialog(ctx app.Context, id uuid.UUID) {
	p.dialog, p.editingID = true, id
	p.dialogError, p.dialogBusy = "", false
	if id == uuid.Nil {
		p.dialogName, p.dialogDescription = "", ""
	} else if item, ok := p.find(id); ok {
		p.dialogName, p.dialogDescription = item.Name, item.Description
	}
	ctx.Update()
}

// closeDialog closes the dialog and drops its state.
func (p *Picker) closeDialog(ctx app.Context) {
	p.dialog, p.editingID, p.dialogBusy = false, uuid.Nil, false
	p.dialogName, p.dialogDescription, p.dialogError = "", "", ""
	ctx.Update()
}

// submitDialog sends the dialog: Create of a new entity or Update of the one
// edited. The server validates — a refusal is shown inside the dialog, with
// the input kept — and a created entity opens at once (task 73).
func (p *Picker) submitDialog(ctx app.Context) {
	if p.dialogBusy {
		return
	}
	p.dialogBusy, p.dialogError = true, ""
	id, name, dialogDescription := p.editingID, p.dialogName, p.dialogDescription
	version := 0
	if item, ok := p.find(id); ok {
		version = item.Version
	}
	uiutil.Do(p.compoCtx, func(c context.Context) (uuid.UUID, error) {
		if id == uuid.Nil {
			return p.source.Create(c, name, dialogDescription)
		}
		return id, p.source.Update(c, id, version, name, dialogDescription)
	}, func(ctx app.Context, saved uuid.UUID, err error) {
		p.dialogBusy = false
		if err != nil {
			p.dialogError = problemText(err)
			ctx.Update()
			return
		}
		p.dialog = false
		p.load(ctx)
		if id == uuid.Nil {
			p.open(ctx, saved)
		}
		ctx.Update()
	})
}

// deleteEntity removes the entity after a confirmation; deleting the shown one
// leaves the page (root shows it then), deleting another one stays on it.
func (p *Picker) deleteEntity(ctx app.Context, id uuid.UUID) {
	item, ok := p.find(id)
	if !ok {
		return
	}
	if !app.Window().Call("confirm", p.source.DeleteQuestion(item.Name)).Bool() {
		return
	}
	p.doDelete(ctx, id)
}

// doDelete is the deletion the confirmation asked for.
func (p *Picker) doDelete(ctx app.Context, id uuid.UUID) {
	wasSelected := id == p.selected
	uiutil.Call(p.compoCtx, func(c context.Context) (struct{}, error) {
		return struct{}{}, p.source.Delete(c, id)
	}, func(ctx app.Context, _ struct{}) {
		if wasSelected {
			p.selectNone(ctx)
		}
		p.load(ctx)
		ctx.Update()
	}, nil)
}

// problemText is a refusal of the server as the dialog shows it: its detail,
// or its title when it carries none.
func problemText(err error) string {
	var se *apiclient.StatusError
	if errors.As(err, &se) {
		if se.Problem.Detail != "" {
			return se.Problem.Detail
		}
		return se.Problem.Title
	}
	problem := toast.FromError(err)
	if problem.Detail != "" {
		return problem.Detail
	}
	return problem.Title
}

// ── Render ────────────────────────────────────────────────────────────────────

func (p *Picker) Render() app.UI {
	rows := make([]app.UI, 0, len(p.items))
	for _, item := range p.items {
		rows = append(rows, p.renderRow(item))
	}
	if len(rows) == 0 {
		text := "No " + p.source.Plural() + " yet."
		if !p.loaded {
			text = "Loading " + p.source.Plural() + "…"
		}
		rows = append(rows, app.Div().
			Style("font-size", "13px").
			Style("color", "var(--text-muted)").
			Style("padding", "8px 0").
			Text(text))
	}
	return app.Div().
		Attr("data-picker", p.source.Singular()).
		Style("display", "flex").
		Style("flex-direction", "column").
		Style("flex", "1").
		Style("min-width", "0").
		Style("min-height", "0").
		Style("padding", "8px 16px").
		Style("box-sizing", "border-box").
		Body(
			app.Div().
				Style("display", "flex").
				Style("align-items", "center").
				Style("gap", "10px").
				Style("padding-bottom", "8px").
				Body(
					app.H2().Style("margin", "0").Style("font-size", "16px").Text(p.source.Title()),
					app.Div().Style("flex", "1"),
					button("Create", "accent", true, "Create a new "+p.source.Singular(), func(ctx app.Context) {
						p.openDialog(ctx, uuid.Nil)
					}),
				),
			app.If(p.dialog, p.renderDialog).Else(func() app.UI { return app.Div() }),
			app.Div().
				Style("flex", "1").
				Style("min-height", "0").
				Style("overflow-y", "auto").
				Body(rows...),
		)
}

// renderRow is one entity: its name, its badges, its description on one line
// and the three things the page does with it.
func (p *Picker) renderRow(item Item) app.UI {
	id := item.ID
	badges := []app.UI{}
	if item.State != "" {
		badges = append(badges, app.Span().
			Attr("data-picker-state", item.StateKind).
			Style("font-size", "10px").
			Style("padding", "1px 6px").
			Style("border-radius", "8px").
			Style("white-space", "nowrap").
			Style("color", stateColor(item.StateKind)).
			Style("border", "1px solid "+stateBorder(item.StateKind)).
			Text(item.State))
	}
	description := app.Div().
		Attr("data-picker-description", "true").
		Style("font-size", "12px").
		Style("overflow", "hidden").
		Style("text-overflow", "ellipsis").
		Style("white-space", "nowrap")
	if item.Description == "" {
		description = description.Style("color", "var(--text-muted)").Text(emptyDescription)
	} else {
		description = description.Style("color", "var(--text-2)").Title(item.Description).Text(item.Description)
	}
	row := app.Div().
		Attr("data-picker-row", id.String()).
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "10px").
		Style("padding", "8px 4px").
		Style("border-bottom", "1px solid var(--border)").
		Body(
			app.Div().
				Style("flex", "1").
				Style("min-width", "0").
				Body(
					app.Div().
						Style("display", "flex").
						Style("align-items", "center").
						Style("gap", "6px").
						Body(
							app.Span().Style("font-size", "14px").Style("font-weight", "600").Text(item.Name),
							app.Span().Body(badges...),
						),
					description,
				),
			button("Open", "accent", true, "Open the "+p.source.Singular(), func(ctx app.Context) {
				p.open(ctx, id)
			}, app.EventScope("open/"+id.String())),
			button("Edit", "", true, "Change the name and description", func(ctx app.Context) {
				p.openDialog(ctx, id)
			}, app.EventScope("edit/"+id.String())),
			button("Delete", "error", true, "Delete the "+p.source.Singular(), func(ctx app.Context) {
				p.deleteEntity(ctx, id)
			}, app.EventScope("delete/"+id.String())),
		).
		OnDblClick(func(ctx app.Context, e app.Event) { p.open(ctx, id) }, app.EventScope("open/"+id.String()))
	return row
}

// renderDialog is the Name+Description form of a new or an edited entity.
func (p *Picker) renderDialog() app.UI {
	title := "New " + p.source.Singular()
	if p.editingID != uuid.Nil {
		title = "Edit " + p.source.Singular()
	}
	var problem app.UI = app.Div()
	if p.dialogError != "" {
		problem = app.Div().
			Attr("role", "alert").
			Attr("data-picker-error", "true").
			Style("margin", "0 0 8px").
			Style("padding", "4px 8px").
			Style("font-size", "12px").
			Style("border", "1px solid var(--error-border)").
			Style("border-radius", "4px").
			Style("background", "var(--error-bg)").
			Style("color", "var(--error)").
			Text(p.dialogError)
	}
	return app.Div().
		Attr("data-picker-dialog", "true").
		Style("margin-bottom", "8px").
		Style("padding", "10px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg-elevated)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Style("font-size", "14px").Text(title),
			problem,
			app.Div().Style("margin", "0 0 4px").Text("Name"),
			app.Input().
				Attr("data-picker-name", "true").
				Type("text").
				Value(p.dialogName).
				Placeholder("Name").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("padding", "4px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				OnInput(func(ctx app.Context, e app.Event) {
					p.dialogName = ctx.JSSrc().Get("value").String()
				}),
			app.Div().Style("margin", "8px 0 4px").Text("Description"),
			app.Textarea().
				Attr("data-picker-description-input", "true").
				Rows(2).
				Placeholder("Description (optional)").
				Style("width", "100%").
				Style("box-sizing", "border-box").
				Style("padding", "4px 6px").
				Style("border", "1px solid var(--border-input)").
				Style("border-radius", "4px").
				Style("background", "var(--input-bg)").
				Style("color", "var(--text)").
				Style("resize", "vertical").
				Style("font-family", "inherit").
				Text(p.dialogDescription).
				OnInput(func(ctx app.Context, e app.Event) {
					p.dialogDescription = ctx.JSSrc().Get("value").String()
				}),
			app.Div().
				Style("display", "flex").
				Style("gap", "6px").
				Style("margin-top", "10px").
				Body(
					button("Save", "accent", !p.dialogBusy, "Save the "+p.source.Singular(), func(ctx app.Context) {
						p.submitDialog(ctx)
					}),
					button("Cancel", "", true, "Close without saving", func(ctx app.Context) {
						p.closeDialog(ctx)
					}),
				),
		)
}

// button renders one action of the page; kind "accent", "error" or "" picks
// its colours. options scope the handler to the row it belongs to.
func button(label, kind string, enabled bool, title string, onClick func(app.Context), options ...app.EventOption) app.UI {
	border, bg, fg := "var(--border-input)", "var(--bg-hover)", "var(--text)"
	switch kind {
	case "accent":
		border, bg, fg = "var(--accent-border)", "var(--accent-bg)", "var(--accent)"
	case "error":
		border, bg, fg = "var(--error-border)", "var(--error-bg)", "var(--error)"
	}
	b := app.Button().
		Title(title).
		Style("padding", "3px 10px").
		Style("font-size", "12px").
		Style("border", "1px solid "+border).
		Style("border-radius", "4px").
		Style("background", bg).
		Style("color", fg).
		Text(label)
	if !enabled {
		return b.Style("opacity", "0.5").Style("cursor", "default").Disabled(true)
	}
	return b.Style("cursor", "pointer").OnClick(func(ctx app.Context, e app.Event) { onClick(ctx) }, options...)
}

// stateColor is the text colour of a badge.
func stateColor(kind string) string {
	switch kind {
	case "unsaved", "unpublished":
		return "var(--toast-warn-title)"
	default:
		return "var(--text-muted)"
	}
}

// stateBorder is the border colour of a badge.
func stateBorder(kind string) string {
	switch kind {
	case "unsaved", "unpublished":
		return "var(--toast-warn-border)"
	default:
		return "var(--border)"
	}
}

// String implements [fmt.Stringer], for the tests' messages.
func (p *Picker) String() string {
	return fmt.Sprintf("Picker(%s): %d items, selected %s", p.source.Title(), len(p.items), p.selected)
}
