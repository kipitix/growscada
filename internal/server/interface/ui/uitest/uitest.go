// Package uitest runs UI components on the host, in go-app's TestEngine, for
// the tests of the UI packages: it mounts a component, runs its async work
// and records the toasts it shows.
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
	"sync"
	"testing"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/toast"
)

// Engine is a TestEngine with one mounted component.
type Engine struct {
	app.TestEngine
	recorder *toastRecorder
}

// Load mounts c (a wrapper as in the package doc) next to a recorder of
// toasts and runs everything its mounting started.
func Load(t *testing.T, c app.Composer) *Engine {
	t.Helper()
	e := &Engine{TestEngine: app.NewTestEngine(), recorder: &toastRecorder{}}
	if err := e.Load(&host{recorder: e.recorder, child: c}); err != nil {
		t.Fatalf("load %T: %v", c, err)
	}
	e.ConsumeAll()
	return e
}

// Toasts returns the toasts shown so far.
func (e *Engine) Toasts() []toast.Problem {
	return e.recorder.all()
}

// TakeToasts returns the toasts shown so far and forgets them.
func (e *Engine) TakeToasts() []toast.Problem {
	return e.recorder.take()
}

// host renders the recorder and the component under test side by side.
type host struct {
	app.Compo
	recorder *toastRecorder
	child    app.Composer
}

func (h *host) Render() app.UI {
	return app.Div().Body(h.recorder, h.child)
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

func (r *toastRecorder) all() []toast.Problem {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]toast.Problem(nil), r.toasts...)
}

func (r *toastRecorder) take() []toast.Problem {
	r.mu.Lock()
	defer r.mu.Unlock()
	taken := r.toasts
	r.toasts = nil
	return taken
}
