package project

import (
	"context"
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// canvasHTML is the markup of the selected Scene's canvas.
func (f *fixture) canvasHTML() string {
	return app.HTMLString(f.p.renderSceneCanvas())
}

func TestSceneCanvas_ShowsTheBackgroundUnderTheGrid(t *testing.T) {
	url, api := newServer(t)
	if _, err := api.CreateScene(context.Background(), apiv0.CreateSceneRequest{
		Name: "Main", Width: 800, Height: 600, BackgroundHTML: `<svg id="bg"></svg>`,
	}); err != nil {
		t.Fatalf("create scene: %v", err)
	}
	f := mount(t, url, api)

	html := f.canvasHTML()
	background := strings.Index(html, `<svg id=\"bg\"></svg>`)
	grid := strings.Index(html, "radial-gradient")
	if background < 0 || grid < 0 || background > grid {
		t.Errorf("want the background, then the grid over it: %s", html)
	}
}

func TestSceneCanvas_WithoutBackground_ShowsOnlyTheGrid(t *testing.T) {
	url, api := newServer(t)
	createScene(t, api, "Main")
	f := mount(t, url, api)

	html := f.canvasHTML()
	if strings.Contains(html, "<iframe") || !strings.Contains(html, "radial-gradient") {
		t.Errorf("want the grid and no background: %s", html)
	}
}

func TestSceneCanvas_ShowsTheSavedBackground(t *testing.T) {
	url, api := newServer(t)
	createScene(t, api, "Main")
	f := mount(t, url, api)

	f.p.editingScenePropsBG = `<svg id="new"></svg>`
	f.p.saveSceneProperties(f.p.compoCtx)
	f.run()

	f.noToasts()
	if html := f.canvasHTML(); !strings.Contains(html, `<svg id=\"new\"></svg>`) {
		t.Errorf("the saved background is not on the canvas: %s", html)
	}
}
