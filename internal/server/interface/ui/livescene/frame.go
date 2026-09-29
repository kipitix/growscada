package livescene

import (
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// widgetFrame is a live widget's sandboxed iframe. Its document is loaded once
// per Srcdoc; new inputs are delivered with postMessage, so the widget neither
// flickers nor restarts its animations. Inputs are sent only after the iframe
// reports it is ready, and then whenever they change.
type widgetFrame struct {
	app.Compo
	Srcdoc     string
	InputsJSON string

	ctx          app.Context
	onMessage    app.Func
	loadedSrcdoc string
	ready        bool
	sentInputs   string
}

func (f *widgetFrame) Render() app.UI {
	return app.IFrame().
		Attr("sandbox", "allow-scripts").
		Attr("allowtransparency", "true").
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("background", "transparent").
		Style("pointer-events", "none").
		Style("display", "block")
}

func (f *widgetFrame) OnMount(ctx app.Context) {
	f.ctx = ctx
	f.onMessage = app.FuncOf(func(this app.Value, args []app.Value) any {
		e := args[0]
		if !e.Get("source").Equal(f.JSValue().Get("contentWindow")) {
			return nil
		}
		data := e.Get("data")
		if data.Type() != app.TypeObject || data.Get("type").String() != uiutil.MessageReady {
			return nil
		}
		f.ctx.Dispatch(func(ctx app.Context) {
			f.ready = true
			f.sendInputs()
		})
		return nil
	})
	app.Window().Call("addEventListener", "message", f.onMessage)
	f.load()
}

func (f *widgetFrame) OnUpdate(ctx app.Context) {
	if f.Srcdoc != f.loadedSrcdoc {
		f.load()
		return
	}
	if f.ready && f.InputsJSON != f.sentInputs {
		f.sendInputs()
	}
}

func (f *widgetFrame) OnDismount() {
	if f.onMessage != nil {
		app.Window().Call("removeEventListener", "message", f.onMessage)
		f.onMessage.Release()
		f.onMessage = nil
	}
}

// load (re)loads the iframe document; inputs wait for its ready message.
func (f *widgetFrame) load() {
	f.loadedSrcdoc = f.Srcdoc
	f.ready = false
	f.sentInputs = ""
	f.JSValue().Set("srcdoc", f.Srcdoc)
}

func (f *widgetFrame) sendInputs() {
	win := f.JSValue().Get("contentWindow")
	if win.IsNull() || win.IsUndefined() {
		return
	}
	msg := app.Window().Get("JSON").Call("parse",
		`{"type":"`+uiutil.MessageInputs+`","inputs":`+f.InputsJSON+`}`)
	// A sandboxed srcdoc iframe has an opaque origin, so "*" is the only
	// target origin that reaches it.
	win.Call("postMessage", msg, "*")
	f.sentInputs = f.InputsJSON
}
