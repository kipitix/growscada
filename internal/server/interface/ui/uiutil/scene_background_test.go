package uiutil

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v10/pkg/app"
)

func TestSceneBackground(t *testing.T) {
	html := app.HTMLString(SceneBackground(`<svg id="bg"></svg>`))
	for _, want := range []string{
		`<iframe `,
		`sandbox="` + SceneBackgroundSandbox + `"`,
		`<body><svg id=\"bg\"></svg></body>`,
		`pointer-events:none`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in %s", want, html)
		}
	}
	if strings.Contains(html, "allow-scripts") {
		t.Errorf("the background must not run scripts: %s", html)
	}
}
