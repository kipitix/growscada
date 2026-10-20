package uiutil

import "github.com/maxence-charriere/go-app/v10/pkg/app"

// SceneBackgroundSandbox is the sandbox of a Scene's background iframe: no
// scripts. It cannot be the empty sandbox="": go-app drops an attribute whose
// value is empty, which would leave the iframe without a sandbox at all.
// allow-same-origin alone lets nothing run in it.
const SceneBackgroundSandbox = "allow-same-origin"

// SceneBackground renders a Scene's static background HTML filling the Scene,
// under its Widgets, in an iframe sandboxed without scripts. pointer-events:none
// lets mouse events reach whatever lies under it (Project's canvas, which
// takes drops and clicks on empty space).
func SceneBackground(backgroundHTML string) app.UI {
	return app.IFrame().
		Attr("sandbox", SceneBackgroundSandbox).
		Attr("srcdoc", `<!DOCTYPE html><html><head><style>html,body{margin:0;padding:0;overflow:hidden}</style></head><body>`+backgroundHTML+`</body></html>`).
		Style("position", "absolute").
		Style("inset", "0").
		Style("width", "100%").
		Style("height", "100%").
		Style("border", "none").
		Style("pointer-events", "none").
		Style("display", "block")
}
