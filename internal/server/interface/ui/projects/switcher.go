// Package projects is the project switcher in the header of the UI: the
// server's projects, the one selected — which Project and Operation show —
// and creating, renaming and deleting them (ADR 0006). Library does not
// depend on it: WidgetTypes are the server's.
package projects

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/eventlog"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// selectedKey is the LocalStorage key of the selected project.
const selectedKey = "root:projectID"

// Switcher lists the server's projects and keeps one selected. The
// selection is announced to onSelect once the list is known, and again each
// time it changes: the user picks another project, or the selected one is
// gone (deleted here or elsewhere) and the first one is taken instead —
// uuid.Nil when there are none.
type Switcher struct {
	app.Compo
	// compoCtx is the component's own context, captured in OnMount; async
	// results are dispatched through it (see project.Project.compoCtx).
	compoCtx app.Context
	client   *apiclient.Client
	onSelect func(uuid.UUID)

	projects []apiv0.ProjectResponse
	reloader uiutil.Reloader
	// selected is the project announced, uuid.Nil until the list is known;
	// preferred the one the user picked last (or LocalStorage held), taken
	// whenever it is in the list.
	selected  uuid.UUID
	preferred uuid.UUID
}

// NewSwitcher returns the switcher of the projects of the server client
// talks to, announcing the selection to onSelect.
func NewSwitcher(client *apiclient.Client, onSelect func(uuid.UUID)) *Switcher {
	return &Switcher{client: client, onSelect: onSelect}
}

// Selected returns the project selected, uuid.Nil if none.
func (s *Switcher) Selected() uuid.UUID { return s.selected }

// Projects returns the server's projects as last loaded, for reading only.
func (s *Switcher) Projects() []apiv0.ProjectResponse { return s.projects }

func (s *Switcher) OnMount(ctx app.Context) {
	s.compoCtx = ctx
	s.preferred = uiutil.LoadID(ctx, selectedKey)
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
	uiutil.StoreID(ctx, selectedKey, want)
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

// Create creates a project named name and selects it.
func (s *Switcher) Create(ctx app.Context, name string) {
	req := apiv0.CreateProjectRequest{Name: name}
	uiutil.Call(s.compoCtx, func(c context.Context) (apiv0.CreateProjectResponse, error) {
		return s.client.CreateProject(c, req)
	}, func(ctx app.Context, created apiv0.CreateProjectResponse) {
		s.preferred = created.ID
		s.load(ctx)
	}, nil)
}

// Rename renames the selected project to name, at the version last loaded.
// A conflict (renamed elsewhere, or the name taken) is shown and the list
// reloaded.
func (s *Switcher) Rename(ctx app.Context, name string) {
	p, ok := s.find(s.selected)
	if !ok {
		return
	}
	req := apiv0.UpdateProjectRequest{Name: name, Version: p.Version}
	uiutil.Call(s.compoCtx, func(c context.Context) (apiv0.UpdateProjectResponse, error) {
		return s.client.UpdateProject(c, p.ID, req)
	}, func(ctx app.Context, _ apiv0.UpdateProjectResponse) {
		s.load(ctx)
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		s.load(ctx)
	})
}

// Delete deletes the selected project with all its content; the first
// remaining project is selected as soon as the server confirms.
func (s *Switcher) Delete(ctx app.Context) {
	id := s.selected
	if id == uuid.Nil {
		return
	}
	uiutil.Call(s.compoCtx, func(c context.Context) (apiv0.ProjectResponse, error) {
		return s.client.DeleteProject(c, id)
	}, func(ctx app.Context, _ apiv0.ProjectResponse) {
		s.forget(ctx, id)
		s.load(ctx)
	}, func(ctx app.Context, err error) {
		uiutil.ShowError(ctx, err)
		s.load(ctx)
	})
}

// prompt asks the user for a text in the browser's dialog; false if they
// cancel or leave it empty.
func prompt(message, value string) (string, bool) {
	answer := app.Window().Call("prompt", message, value)
	if !answer.Truthy() {
		return "", false
	}
	return answer.String(), answer.String() != ""
}

func (s *Switcher) Render() app.UI {
	options := make([]app.UI, 0, len(s.projects)+1)
	if s.selected == uuid.Nil {
		options = append(options, app.Option().Value("").Selected(true).Text("— no project —"))
	}
	for _, p := range s.projects {
		options = append(options, app.Option().Value(p.ID.String()).Selected(p.ID == s.selected).Text(p.Name))
	}
	current, hasCurrent := s.find(s.selected)

	return app.Div().
		Attr("data-project-switcher", "true").
		Style("display", "flex").
		Style("align-items", "center").
		Style("gap", "4px").
		Style("margin", "0 12px").
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
			s.button("+", "Create a project", true, func(ctx app.Context) {
				if name, ok := prompt("Name of the new project:", ""); ok {
					s.Create(ctx, name)
				}
			}),
			s.button("✎", "Rename the project", hasCurrent, func(ctx app.Context) {
				if name, ok := prompt("New name of the project:", current.Name); ok && name != current.Name {
					s.Rename(ctx, name)
				}
			}),
			s.button("🗑", "Delete the project", hasCurrent, func(ctx app.Context) {
				message := fmt.Sprintf("Delete project %q with all its scenes, widgets and tags?", current.Name)
				if app.Window().Call("confirm", message).Bool() {
					s.Delete(ctx)
				}
			}),
		)
}

func (s *Switcher) button(icon, title string, enabled bool, onClick func(app.Context)) app.UI {
	return app.Button().
		Title(title).
		Attr("aria-label", title).
		Disabled(!enabled).
		Style("padding", "2px 7px").
		Style("font-size", "13px").
		Style("line-height", "1").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg-elevated)").
		Style("color", "var(--text-2)").
		Text(icon).
		OnClick(func(ctx app.Context, e app.Event) { onClick(ctx) })
}
