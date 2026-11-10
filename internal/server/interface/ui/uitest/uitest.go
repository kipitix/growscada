// Package uitest runs UI components on the host, in go-app's TestEngine, for
// the tests of the UI packages: it mounts a component, runs its async work
// and records the toasts it shows and, when asked, the actions it fires.
//
// go-app calls OnMount only in the browser; on the host it calls OnPreRender
// instead, with the component's own context. A test therefore mounts its
// component through a wrapper that forwards one to the other, e.g.
//
//	type mountOnHost struct{ *Project }
//
//	func (m mountOnHost) OnPreRender(ctx app.Context) { m.OnMount(ctx) }
package uitest

import (
	"slices"
	"sync"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// Engine is a TestEngine with one mounted component.
type Engine struct {
	app.TestEngine
	recorder *toastRecorder
	watcher  *actionRecorder
}

// Load mounts c (a wrapper as in the package doc) next to a recorder of
// toasts and runs everything its mounting started.
func Load(t *testing.T, c app.Composer) *Engine {
	t.Helper()
	return load(t, c, nil)
}

// LoadWatching is Load recording the actions named watch as they are handled
// (TakeActions): a component that reports a fact to its parent — an entity
// found gone, say — is checked by the action it fires, the parent itself
// being another package's.
func LoadWatching(t *testing.T, c app.Composer, watch ...string) *Engine {
	t.Helper()
	return load(t, c, &actionRecorder{watch: slices.Clone(watch)})
}

func load(t *testing.T, c app.Composer, watcher *actionRecorder) *Engine {
	t.Helper()
	e := &Engine{TestEngine: app.NewTestEngine(), recorder: &toastRecorder{}, watcher: watcher}
	if err := e.Load(&host{recorder: e.recorder, watcher: e.watcher, child: c}); err != nil {
		t.Fatalf("load %T: %v", c, err)
	}
	e.ConsumeAll()
	return e
}

// TakeToasts returns the toasts shown so far and forgets them.
func (e *Engine) TakeToasts() []toast.Problem {
	return e.recorder.take()
}

// Action is an action the handlers of the host saw, with the name it was
// handled under and its value.
type Action struct {
	Name  string
	Value any
}

// TakeActions returns the actions recorded so far and forgets them; none
// without LoadWatching.
func (e *Engine) TakeActions() []Action {
	if e.watcher == nil {
		return nil
	}
	return e.watcher.take()
}

// host renders the recorders and the component under test side by side.
type host struct {
	app.Compo
	recorder *toastRecorder
	watcher  *actionRecorder
	child    app.Composer
}

func (h *host) Render() app.UI {
	body := []app.UI{h.recorder}
	if h.watcher != nil {
		body = append(body, h.watcher)
	}
	return app.Div().Body(append(body, h.child)...)
}

// toastRecorder takes the place of toast.Container: it handles
// toast.ActionAdd and keeps the problems instead of showing them.
type toastRecorder struct {
	app.Compo

	mu     sync.Mutex
	toasts []toast.Problem
}

func (r *toastRecorder) OnPreRender(ctx app.Context) {
	ctx.Handle(toast.ActionAdd, func(ctx app.Context, a app.Action) {
		if p, ok := a.Value.(toast.Problem); ok {
			r.mu.Lock()
			r.toasts = append(r.toasts, p)
			r.mu.Unlock()
		}
	})
}

func (r *toastRecorder) Render() app.UI { return app.Div() }

func (r *toastRecorder) take() []toast.Problem {
	r.mu.Lock()
	defer r.mu.Unlock()
	taken := r.toasts
	r.toasts = nil
	return taken
}

// actionRecorder keeps the actions named watch as their handlers see them.
type actionRecorder struct {
	app.Compo

	watch []string

	mu   sync.Mutex
	seen []Action
}

func (r *actionRecorder) OnPreRender(ctx app.Context) {
	for _, name := range r.watch {
		ctx.Handle(name, func(ctx app.Context, a app.Action) {
			r.mu.Lock()
			r.seen = append(r.seen, Action{Name: name, Value: a.Value})
			r.mu.Unlock()
		})
	}
}

func (r *actionRecorder) Render() app.UI { return app.Div() }

func (r *actionRecorder) take() []Action {
	r.mu.Lock()
	defer r.mu.Unlock()
	taken := r.seen
	r.seen = nil
	return taken
}
