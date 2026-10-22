package uiutil

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

func TestSceneBackgroundSrcdoc(t *testing.T) {
	if doc := SceneBackgroundSrcdoc(`<svg id="bg"></svg>`); !strings.Contains(doc, `<body><svg id="bg"></svg></body>`) {
		t.Errorf("the background HTML is not the document's body: %s", doc)
	}
	if doc := SceneBackgroundSrcdoc(""); !strings.Contains(doc, `html,body{margin:0;padding:0;overflow:hidden;height:100%}`) {
		t.Errorf("html and body must fill the Scene: %s", doc)
	}
}

func TestSceneBackground(t *testing.T) {
	html := app.HTMLString(&SceneBackground{HTML: `<svg id="bg"></svg>`})
	for _, want := range []string{
		`<iframe `,
		`color-scheme:light`,
		`pointer-events:none`,
		`display:block`,
		`tabindex="-1"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in %s", want, html)
		}
	}
	// HTMLString writes the value "true" as a bare attribute, in no fixed
	// order (the last one is followed by ">"); in the browser go-app sets
	// aria-hidden="true".
	if !strings.Contains(html, ` aria-hidden `) && !strings.Contains(html, ` aria-hidden>`) {
		t.Errorf("missing aria-hidden in %s", html)
	}
	// Both are set on the element on mount: sandbox="" first, then srcdoc.
	for _, unwanted := range []string{`sandbox=`, `srcdoc=`} {
		if strings.Contains(html, unwanted) {
			t.Errorf("unexpected %s in %s", unwanted, html)
		}
	}
}

func TestSceneBackground_WithoutHTML_IsHidden(t *testing.T) {
	if html := app.HTMLString(&SceneBackground{}); !strings.Contains(html, `display:none`) {
		t.Errorf("an empty background must be hidden: %s", html)
	}
}
