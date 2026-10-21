package project

import (
	"context"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// canvasBackground is the background layer of the selected Scene's canvas.
func (f *fixture) canvasBackground(t *testing.T) string {
	t.Helper()
	sc, _ := f.p.findScene(f.p.selectedSceneID)
	layers := f.p.canvasLayers(sc.BackgroundHTML)
	bg, ok := layers[0].(*uiutil.SceneBackground)
	if !ok {
		t.Fatalf("the bottom layer is %T, want the background", layers[0])
	}
	if !strings.Contains(app.HTMLString(layers[1]), "radial-gradient") {
		t.Errorf("want the grid right over the background: %s", app.HTMLString(layers[1]))
	}
	return bg.HTML
}

func TestSceneCanvas_ShowsTheBackgroundUnderTheGrid(t *testing.T) {
	url, api := newServer(t)
	if _, err := api.CreateScene(context.Background(), apiv0.CreateSceneRequest{
		Name: "Main", Width: 800, Height: 600, BackgroundHTML: `<svg id="bg"></svg>`,
	}); err != nil {
		t.Fatalf("create scene: %v", err)
	}
	f := mount(t, url, api)

	if got := f.canvasBackground(t); got != `<svg id="bg"></svg>` {
		t.Errorf("background = %q", got)
	}
}

// The background keeps its place without HTML, so the Widgets' layer does not
// move when a background is added or removed.
func TestSceneCanvas_WithoutBackground_KeepsAnEmptyBackgroundLayer(t *testing.T) {
	url, api := newServer(t)
	createScene(t, api, "Main")
	f := mount(t, url, api)

	if got := f.canvasBackground(t); got != "" {
		t.Errorf("background = %q, want none", got)
	}
}

func TestSceneCanvas_ShowsTheSavedBackground(t *testing.T) {
	url, api := newServer(t)
	createScene(t, api, "Main")
	f := mount(t, url, api)

	f.p.sceneEdit.background = `<svg id="new"></svg>`
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	if got := f.canvasBackground(t); got != `<svg id="new"></svg>` {
		t.Errorf("the saved background is not on the canvas: %q", got)
	}
}
