package uiutil

import "github.com/maxence-charriere/go-app/v11/pkg/app"

// SceneBackground renders a Scene's static background HTML filling the Scene,
// under its Widgets, in an iframe fully sandboxed (sandbox="": no scripts, an
// opaque origin). Its base is the Widgets' (sceneLayerIFrame): it lets mouse
// events reach Project's canvas, which takes drops and clicks on empty space,
// and keeps the dark theme from painting over what it leaves uncovered.
//
// A Scene renders it always, hidden while HTML is empty: go-app matches
// children by position, so a layer that comes and goes would hand every
// Widget's component to its neighbour and reload all their iframes.
//
// It is decoration: out of the Tab order and hidden from screen readers, so
// a link or control in the background can neither be reached by keyboard nor
// navigate the iframe away from the background.
type SceneBackground struct {
	app.Compo
	HTML string

	srcdocFor string
	srcdoc    string
	doc       IframeSrcdoc
}

func (b *SceneBackground) Render() app.UI {
	display := "block"
	if b.HTML == "" {
		display = "none"
	}
	return sceneLayerIFrame().
		Attr("tabindex", "-1").
		Attr("aria-hidden", "true").
		Style("position", "absolute").
		Style("top", "0").
		Style("left", "0").
		Style("display", display)
}

func (b *SceneBackground) OnMount(ctx app.Context) {
	if b.sandbox() {
		b.doc.Mount(b.JSValue(), b.document())
	}
}

func (b *SceneBackground) OnUpdate(ctx app.Context) {
	if b.sandbox() {
		b.doc.Update(b.JSValue(), b.document())
	}
}

// sandbox gives the iframe sandbox="" before it gets its document and reports
// whether it is an element. Not an Attr in Render: go-app drops an attribute
// whose value is empty, which would leave the iframe without a sandbox; and
// any non-empty value either grants something or is an invalid flag the
// browser complains about on every load. go-app never touches an attribute
// Render does not state, so the one set here stays.
func (b *SceneBackground) sandbox() bool {
	frame := b.JSValue()
	if !isElement(frame) {
		return false
	}
	frame.Call("setAttribute", "sandbox", "")
	return true
}

// document is the iframe's document, rebuilt only when HTML changed: the
// canvas re-renders on every drag move and every Tag update.
func (b *SceneBackground) document() string {
	if b.srcdoc == "" || b.srcdocFor != b.HTML {
		b.srcdocFor, b.srcdoc = b.HTML, SceneBackgroundSrcdoc(b.HTML)
	}
	return b.srcdoc
}

// SceneBackgroundSrcdoc is the document of a Scene's background iframe. Its
// html and body fill the Scene, so a background sized in percent (an
// <svg width="100%" height="100%">) fills it too.
func SceneBackgroundSrcdoc(backgroundHTML string) string {
	return `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0;overflow:hidden;height:100%}</style></head><body>` + backgroundHTML + `</body></html>`
}

// SceneWidgetLayer stacks a Scene's Widgets over its background (and the
// editor's overlays): a stacking context of its own, so a Widget's z-index
// (Position.Z, negative too) orders it among the Widgets only, never under
// the background.
func SceneWidgetLayer(widgets []app.UI) app.UI {
	return app.Div().
		Style("position", "absolute").
		Style("inset", "0").
		Style("isolation", "isolate").
		Body(widgets...)
}
