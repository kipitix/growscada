// Package projects is the small project selector at the top of Operation and
// History, one in each: the server's projects and the one the mode shows
// (ADR 0006). Project and Library list their entities on a selection page of
// their own (ui/picker, task 73), which also creates, renames and deletes
// them; Library does not depend on a project at all.
package projects

import (
	"slices"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// legacyKey is the LocalStorage key of the project selected when one
// switcher served every mode: a switcher whose own key holds none starts
// from it.
const legacyKey = "root:projectID"

// Switcher lists the server's projects and keeps one selected. The
// selection is announced to onSelect once the list is known, and again each
// time it changes: the user picks another project, or the selected one is
// gone (deleted elsewhere) and the first one is taken instead — uuid.Nil when
// there are none.
type Switcher struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount; async
	// results are dispatched through it (see project.Project.compoCtx).
	compoCtx app.Context
	client   *apiclient.Client
	// key is the LocalStorage key of the selection.
	key      string
	onSelect func(uuid.UUID)

	projects []apiv0.ProjectResponse
	reloader uiutil.Reloader
	// selected is the project announced, uuid.Nil until the list is known;
	// preferred the one the user picked last (or LocalStorage held), taken
	// whenever it is in the list.
	selected  uuid.UUID
	preferred uuid.UUID
}

// NewSelector returns the selector of the projects of the server client talks
// to, keeping the selection under the LocalStorage key and announcing it to
// onSelect.
func NewSelector(client *apiclient.Client, key string, onSelect func(uuid.UUID)) *Switcher {
	return &Switcher{client: client, key: key, onSelect: onSelect}
}

// Selected returns the project selected, uuid.Nil if none.
func (s *Switcher) Selected() uuid.UUID { return s.selected }

// Projects returns the server's projects as last loaded, for reading only.
func (s *Switcher) Projects() []apiv0.ProjectResponse { return s.projects }

func (s *Switcher) OnMount(ctx app.Context) {
	s.compoCtx = ctx
	s.preferred = uiutil.LoadID(ctx, s.key)
	if s.preferred == uuid.Nil {
		s.preferred = uiutil.LoadID(ctx, legacyKey)
	}
	s.load(ctx)
	ctx.Handle(eventlog.ActionServerEvent, func(ctx app.Context, a app.Action) {
		ev, ok := a.Value.(apiv0.Event)
		if !ok || !eventlog.ReloadTargetsFor(ev).Projects {
			return
		}
		if ev.Type == apiv0.EventProjectDeleted {
			s.forget(ctx, uiutil.ParseID(ev.ID))
		}
		s.load(ctx)
	})
	ctx.Handle(uiutil.ActionProjectGone, func(ctx app.Context, a app.Action) {
		if id, ok := a.Value.(uuid.UUID); ok {
			s.forget(ctx, id)
			s.load(ctx)
		}
	})
}

// forget drops the project id, known to be gone, from the list at once,
// without waiting for the list to be reloaded: if it is the one selected,
// another is selected — and announced — right away, so the modes and the
// event stream leave it before more of its content (deleted by the server
// after it) is loaded or shown.
func (s *Switcher) forget(ctx app.Context, id uuid.UUID) {
	if id == uuid.Nil {
		return
	}
	if s.preferred == id {
		s.preferred = uuid.Nil
	}
	if i := uiutil.IdxByID(s.projects, id, uiutil.ProjectID); i >= 0 {
		s.projects = slices.Delete(slices.Clone(s.projects), i, i+1)
	}
	if s.selected == id {
		s.reselect(ctx)
	}
}

// load (re)loads the list of projects.
func (s *Switcher) load(ctx app.Context) {
	uiutil.Load(s.compoCtx, &s.reloader, s.client.ListProjects, func(ctx app.Context, list []apiv0.ProjectResponse) {
		s.projects = list
		s.reselect(ctx)
	}, s.load)
}

// reselect selects, from the list, the preferred project, else the one
// selected, else the first, and announces it if it changed.
func (s *Switcher) reselect(ctx app.Context) {
	want := uuid.Nil
	for _, id := range []uuid.UUID{s.preferred, s.selected} {
		if _, ok := s.find(id); ok {
			want = id
			break
		}
	}
	if want == uuid.Nil && len(s.projects) > 0 {
		want = s.projects[0].ID
	}
	if want == s.selected {
		return
	}
	s.selected = want
	uiutil.StoreID(ctx, s.key, want)
	s.onSelect(want)
}

func (s *Switcher) find(id uuid.UUID) (apiv0.ProjectResponse, bool) {
	return uiutil.FindByID(s.projects, id, uiutil.ProjectID)
}

// Select selects the project id, as picking it in the list does.
func (s *Switcher) Select(ctx app.Context, id uuid.UUID) {
	s.preferred = id
	s.reselect(ctx)
}

func (s *Switcher) Render() app.UI {
	options := make([]app.UI, 0, len(s.projects)+1)
	if s.selected == uuid.Nil {
		options = append(options, app.Option().Value("").Selected(true).Text("— no project —"))
	}
	for _, p := range s.projects {
		options = append(options, app.Option().Value(p.ID.String()).Selected(p.ID == s.selected).Text(p.Name))
	}
	return app.Div().
		Attr("data-project-switcher", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Style("margin-bottom", "8px").
		Body(
			app.Span().
				Style("font-size", "12px").
				Style("color", "var(--text-3)").
				Text("Project"),
			app.Select().
				Attr("aria-label", "Project").
				Style("font-size", "13px").
				Style("padding", "2px 4px").
				Style("max-width", "220px").
				OnChange(func(ctx app.Context, e app.Event) {
					s.Select(ctx, uiutil.ParseID(ctx.JSSrc().Get("value").String()))
				}).
				Body(options...),
		)
}
