// Package picker is the selection page of the Library and Project modes
// (task 73, task 82): the list of the server's libraries or projects beside
// the details of the selected one, two panes on a desktop screen and two
// screens on a phone.
//
// The list is for choosing an entity and, for the Libraries, for arranging it
// (task 81): a row carries a name and a dot for the entity's state, nothing
// else. Everything that acts on the entity — its name and description, opening
// the editor, deleting it — lives in the details pane. Create puts a draft row
// at the top of the list and opens the details in its "new" mode, so the page
// has no dialog and no form of its own.
//
// The page reads its list on mount, on every event that may change it
// (project_* or library_*, EventResync) and when it is shown again. A
// description is one line with its full text in the title; an entity without
// one says so quietly. Nothing is filtered or paged: the Libraries come in the
// order the person arranged (ADR 0024), the rest, and the Projects, by name;
// "no entity" is not a page of its own (task 73).
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
	// Singular names one entity in the messages.
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

// OrderSource is a Source whose entities carry a per-user order (task 81,
// ADR 0024): the Library list. A Source that does not implement it lists its
// entities by name and offers no drag.
type OrderSource interface {
	// Order returns the arrangement the person placed and the Version their
	// next write states.
	Order(ctx context.Context) (Order, error)
	// SetOrder stores the whole order and returns the Version stored.
	SetOrder(ctx context.Context, version int, ids []uuid.UUID) (int, error)
}

// emptyDescription is the quiet text of an entity without a description.
const emptyDescription = "No description"

// The mobile screens of the page: the list, or the details of one entity. On a
// desktop screen both are shown and the state only says which one a phone
// would show.
const (
	screenList    = "list"
	screenDetails = "details"
)

// editField is the field of the selected entity the details pane is editing,
// empty for none: the name or the description is edited in place (task 82).
type editField string

const (
	fieldNone        editField = ""
	fieldName        editField = "name"
	fieldDescription editField = "description"
)

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

	// order is the arrangement the person stored (task 81), the IDs the list
	// puts first; orderSeq drops a response of a read a newer one overtook.
	order       Order
	orderSeq    int
	savingOrder bool
	// orderUnavailable says the server has no per-user settings (an API older
	// than 0.34): the list is by name and offers no drag, rather than failing
	// a write and a toast on every load (task 82).
	orderUnavailable bool

	// selected is the entity the details pane shows and its name as last read;
	// the page reports the pair to onSelect whenever either moves.
	selected     uuid.UUID
	selectedName string
	// screen is the mobile pane shown: the list unless a row was chosen.
	screen string

	// field and value are the inline edit in progress of the selected entity;
	// editError is the server's refusal, shown under the fields, and saving
	// says a write is in flight.
	field     editField
	value     string
	editError string
	saving    bool

	// creating is a new entity whose name is not committed yet: the list shows
	// a draft row and the details pane its name field.
	creating    bool
	createName  string
	createError string
	createBusy  bool

	// dragFrom is the row a desktop drag started on, -1 for none; dragging
	// says one is in flight, so the rows can show where a drop would land.
	dragFrom int
	dragging bool

	// releaseTouch ends the pointer-drag listeners of a phone (task 82).
	releaseTouch func()
}

// New returns the selection page of source, keeping the selection under the
// LocalStorage key and reporting it to onSelect — the entity the mode shows,
// uuid.Nil for none — and to onOpen when the user opens one.
func New(source Source, key string, onSelect func(uuid.UUID, string), onOpen func(uuid.UUID)) *Picker {
	return &Picker{source: source, key: key, onSelect: onSelect, onOpen: onOpen, dragFrom: -1}
}

func (p *Picker) OnMount(ctx app.Context) {
	p.compoCtx = ctx
	p.mounted = true
	p.screen = screenList
	p.selected = uiutil.LoadID(ctx, p.key)
	p.installTouch()
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

// OnDismount releases what a phone's drag holds in the browser.
func (p *Picker) OnDismount() {
	p.uninstallTouch()
}

// Reload rereads the list: the page is shown again. Before OnMount the list
// is not read yet, and OnMount reads it.
func (p *Picker) Reload(ctx app.Context) {
	if !p.mounted {
		return
	}
	p.load(ctx)
}

// Items returns the list as last loaded, in the server's order.
func (p *Picker) Items() []Item { return p.items }

// SelectedID returns the entity the page reports as shown (uuid.Nil: none).
func (p *Picker) SelectedID() uuid.UUID { return p.selected }

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

// Displayed returns the entities as the page shows them: the IDs the person
// placed first, in that order, then the rest by name (task 81, ADR 0024). An
// ID of an entity this list does not have is ignored; the client is what
// merges, the server never reads the entities.
func (p *Picker) Displayed() []Item {
	shown := make([]Item, 0, len(p.items))
	placed := make(map[uuid.UUID]struct{}, len(p.order.IDs))
	byID := make(map[uuid.UUID]Item, len(p.items))
	for _, item := range p.items {
		byID[item.ID] = item
	}
	for _, anID := range p.order.IDs {
		item, ok := byID[anID]
		if !ok {
			continue
		}
		if _, twice := placed[anID]; twice {
			continue
		}
		placed[anID] = struct{}{}
		shown = append(shown, item)
	}
	rest := make([]Item, 0, len(p.items)-len(shown))
	for _, item := range p.items {
		if _, ok := placed[item.ID]; !ok {
			rest = append(rest, item)
		}
	}
	slices.SortFunc(rest, byName)
	return append(shown, rest...)
}

// byName orders two entities alphabetically, whatever their case.
func byName(a, b Item) int {
	return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
}

// itemID is the ID of an Item for uiutil's lookups.
func itemID(item Item) uuid.UUID { return item.ID }

func (p *Picker) find(id uuid.UUID) (Item, bool) {
	return uiutil.FindByID(p.items, id, itemID)
}

// load (re)reads the list and the arrangement, and reconciles the selection
// with them: a client reload or a resync is its own, and an entity deleted
// elsewhere leaves nothing to show.
func (p *Picker) load(ctx app.Context) {
	uiutil.Load(p.compoCtx, &p.reloader, p.source.List, func(ctx app.Context, items []Item) {
		p.items, p.loaded = items, true
		p.reconcile(ctx)
		p.loadOrder(ctx)
		ctx.Update()
	}, p.load)
}

// loadOrder rereads the arrangement of the entities, when the source has one
// (task 81). A response of a read a newer one overtook is dropped.
func (p *Picker) loadOrder(ctx app.Context) {
	orderSource, ok := p.source.(OrderSource)
	if !ok {
		return
	}
	p.orderSeq++
	seq := p.orderSeq
	uiutil.Call(p.compoCtx, orderSource.Order, func(ctx app.Context, order Order) {
		if seq != p.orderSeq {
			return
		}
		p.order = order
		ctx.Update()
	}, func(ctx app.Context, err error) {
		if errors.Is(err, apiclient.ErrNotFound) {
			// A server without the setting is not an error worth a toast on
			// every load: the list is by name and offers no drag.
			p.orderUnavailable = true
			ctx.Update()
			return
		}
		uiutil.ShowError(ctx, err)
	})
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
	p.screen = screenList
	if p.selected == uuid.Nil && p.selectedName == "" {
		return
	}
	p.selected, p.selectedName = uuid.Nil, ""
	uiutil.StoreID(ctx, p.key, uuid.Nil)
	if p.onSelect != nil {
		p.onSelect(uuid.Nil, "")
	}
}

// selectByID shows the details of the entity id, read as it is now: go-app
// reuses a row whose handler it thinks is the same one, so a handler never
// carries an entity's data from an older render (task 82).
func (p *Picker) selectByID(ctx app.Context, id uuid.UUID) {
	item, ok := p.find(id)
	if !ok {
		return
	}
	p.selectItem(ctx, item)
}

// selectItem shows the details of item: a click on its row, or the arrows of
// the list. Choosing an entity abandons the draft of a new one, which was never
// sent anywhere, and closes the inline edit of the one shown: the field belongs
// to that entity, not to the one picked now.
func (p *Picker) selectItem(ctx app.Context, item Item) {
	p.creating, p.createName, p.createError = false, "", ""
	p.field, p.value, p.editError = fieldNone, "", ""
	p.report(ctx, item)
	p.screen = screenDetails
	ctx.Update()
}

// open shows the editor of id: the Open button of the details pane, or a
// created entity.
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

// startEdit opens the inline edit of the selected entity's field, with the
// value it holds now.
func (p *Picker) startEdit(ctx app.Context, field editField) {
	item, ok := p.find(p.selected)
	if !ok || p.busy() {
		return
	}
	p.field = field
	p.editError = ""
	switch field {
	case fieldName:
		p.value = item.Name
	case fieldDescription:
		p.value = item.Description
	}
	ctx.Update()
}

// cancelEdit drops the inline edit and the value typed into it.
func (p *Picker) cancelEdit(ctx app.Context) {
	p.field, p.value, p.editError = fieldNone, "", ""
	ctx.Update()
}

// commitEdit writes the edited field: Enter or a blur of the name, Ctrl+Enter
// or a blur of the description. The server is the only validator; a refusal or
// a conflict keeps the field open with the text typed in it (task 82).
func (p *Picker) commitEdit(ctx app.Context) {
	if p.saving || p.field == fieldNone {
		return
	}
	item, ok := p.find(p.selected)
	if !ok {
		p.cancelEdit(ctx)
		return
	}
	name, description := item.Name, item.Description
	if p.field == fieldName {
		name = strings.TrimSpace(p.value)
	} else {
		description = strings.TrimSpace(p.value)
	}
	// Nothing changed: the page leaves the field without a request, so an
	// edit that changes nothing neither bumps the Version nor publishes.
	if name == item.Name && description == item.Description {
		p.field, p.value, p.editError = fieldNone, "", ""
		ctx.Update()
		return
	}
	if name == "" {
		p.editError = "A name is required"
		ctx.Update()
		return
	}
	p.saving = true
	edited := item.ID
	uiutil.Do(p.compoCtx, func(c context.Context) (struct{}, error) {
		return struct{}{}, p.source.Update(c, edited, item.Version, name, description)
	}, func(ctx app.Context, _ struct{}, err error) {
		p.saving = false
		// The user may have chosen another entity while the write was in
		// flight: its refusal belongs to the entity edited, never to the one
		// shown now, and the field of that one is not this write's to close.
		if p.selected != edited {
			if err != nil {
				uiutil.ShowError(ctx, err)
			}
			p.load(ctx)
			ctx.Update()
			return
		}
		if err != nil {
			p.editError = problemText(err)
			if errors.Is(err, apiclient.ErrConflict) {
				// The entity moved on: reread it so the next attempt states
				// its current Version, and keep the text typed in.
				p.load(ctx)
			}
			ctx.Update()
			return
		}
		p.field, p.value, p.editError = fieldNone, "", ""
		p.load(ctx)
		ctx.Update()
	})
}

// startCreate puts a new entity at the top of the list and opens the details
// in its "new" mode, with the name field focused (task 82).
func (p *Picker) startCreate(ctx app.Context) {
	if p.busy() {
		return
	}
	p.field, p.editError = fieldNone, ""
	p.creating, p.createName, p.createError = true, "", ""
	p.screen = screenDetails
	// The draft row is the list's selection while it is open, and it sits at
	// the top: a list scrolled down has to show it.
	app.Window().GetElementByID(p.rowsID()).Set("scrollTop", 0)
	ctx.Update()
}

// cancelCreate drops the draft without a request.
func (p *Picker) cancelCreate(ctx app.Context) {
	p.creating, p.createName, p.createError = false, "", ""
	p.screen = screenList
	ctx.Update()
}

// commitCreate creates the entity of the draft row. An empty name drops it
// without a request; a refusal of the server keeps the draft with the text and
// the message under it (task 82).
func (p *Picker) commitCreate(ctx app.Context) {
	if p.createBusy || !p.creating {
		return
	}
	name := strings.TrimSpace(p.createName)
	if name == "" {
		p.cancelCreate(ctx)
		return
	}
	p.createBusy = true
	p.createError = ""
	uiutil.Do(p.compoCtx, func(c context.Context) (uuid.UUID, error) {
		return p.source.Create(c, name, "")
	}, func(ctx app.Context, id uuid.UUID, err error) {
		p.createBusy = false
		if err != nil {
			p.createError = problemText(err)
			ctx.Update()
			return
		}
		p.creating, p.createName, p.createError = false, "", ""
		p.load(ctx)
		p.open(ctx, id)
	})
}

// deleteEntity removes the entity after a confirmation; the details pane then
// shows the next one, so deleting never leaves an empty pane on a desktop
// screen and a dead screen on a phone.
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
	shown := p.Displayed()
	index := uiutil.IdxByID(shown, id, itemID)
	wasSelected := id == p.selected
	uiutil.Call(p.compoCtx, func(c context.Context) (struct{}, error) {
		return struct{}{}, p.source.Delete(c, id)
	}, func(ctx app.Context, _ struct{}) {
		if wasSelected {
			p.selectAfterDelete(ctx, shown, id, index)
		}
		p.load(ctx)
		ctx.Update()
	}, nil)
}

// selectAfterDelete selects the entity that took the deleted one's place, the
// last one when it was the last, or none.
func (p *Picker) selectAfterDelete(ctx app.Context, shown []Item, deleted uuid.UUID, index int) {
	rest := make([]Item, 0, len(shown)-1)
	for _, item := range shown {
		if item.ID != deleted {
			rest = append(rest, item)
		}
	}
	switch {
	case len(rest) == 0:
		p.selectNone(ctx)
	case index >= len(rest):
		p.report(ctx, rest[len(rest)-1])
	default:
		p.report(ctx, rest[index])
	}
}

// move puts the entity at position from at position to and stores the whole
// order at once, optimistically: a refused write puts the previous
// arrangement back and shows the refusal (task 81, task 82).
func (p *Picker) move(ctx app.Context, from, to int) {
	orderSource, ok := p.source.(OrderSource)
	if !ok || p.orderUnavailable || p.savingOrder {
		return
	}
	ids := reordered(p.Displayed(), from, to)
	if ids == nil {
		return
	}
	previous := p.order
	p.order = Order{IDs: ids, Version: previous.Version}
	// An order read started before the drop states the arrangement before it:
	// this bump makes its response stale, so it cannot undo the drop on
	// screen and leave the next write stating an older Version.
	p.orderSeq++
	p.savingOrder = true
	ctx.Update()
	uiutil.Do(p.compoCtx, func(c context.Context) (int, error) {
		return orderSource.SetOrder(c, previous.Version, ids)
	}, func(ctx app.Context, saved int, err error) {
		p.savingOrder = false
		if err != nil {
			p.order = previous
			uiutil.ShowError(ctx, err)
			if errors.Is(err, apiclient.ErrConflict) {
				p.loadOrder(ctx)
			}
			ctx.Update()
			return
		}
		p.order.Version = saved
		ctx.Update()
	})
}

// moveSelected moves the selected entity one place, for the keyboard's
// Alt+Arrow (task 82).
func (p *Picker) moveSelected(ctx app.Context, delta int) {
	shown := p.Displayed()
	index := uiutil.IdxByID(shown, p.selected, itemID)
	if index < 0 {
		return
	}
	p.move(ctx, index, index+delta)
}

// stepSelection moves the selection through the shown list, clamped at its
// ends.
func (p *Picker) stepSelection(ctx app.Context, delta int) {
	shown := p.Displayed()
	if len(shown) == 0 {
		return
	}
	index := uiutil.IdxByID(shown, p.selected, itemID)
	next := index + delta
	switch {
	case index < 0:
		next = 0
	case next < 0:
		next = 0
	case next >= len(shown):
		next = len(shown) - 1
	}
	p.selectItem(ctx, shown[next])
}

// onRowsKey is the keyboard of the list: the arrows move the selection, Alt
// with an arrow moves the selected entity, Enter opens it (task 82).
func (p *Picker) onRowsKey(ctx app.Context, e app.Event) {
	if p.creating || p.busy() {
		return
	}
	alt := e.Get("altKey").Bool()
	switch e.Get("key").String() {
	case "ArrowDown":
		e.PreventDefault()
		if alt {
			p.moveSelected(ctx, 1)
			return
		}
		p.stepSelection(ctx, 1)
	case "ArrowUp":
		e.PreventDefault()
		if alt {
			p.moveSelected(ctx, -1)
			return
		}
		p.stepSelection(ctx, -1)
	case "Enter":
		if p.selected != uuid.Nil {
			p.open(ctx, p.selected)
		}
	}
}

// busy reports whether a write of the page is in flight: the actions of the
// details pane wait for it, so a click never states a Version the write about
// to land makes stale (task 82).
func (p *Picker) busy() bool { return p.saving || p.savingOrder || p.createBusy }

// problemText is a refusal of the server as the page shows it: its detail, or
// its title when it carries none.
func problemText(err error) string {
	problem := toast.FromError(err)
	if problem.Detail != "" {
		return problem.Detail
	}
	return problem.Title
}

// boolAttr renders a boolean attribute value the way HTML reads it.
func boolAttr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// String implements [fmt.Stringer], for the tests' messages.
func (p *Picker) String() string {
	return fmt.Sprintf("Picker(%s): %d items, selected %s", p.source.Title(), len(p.items), p.selected)
}
