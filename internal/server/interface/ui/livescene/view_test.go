package livescene

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

func TestView_RendersTheSceneBackground(t *testing.T) {
	html := app.HTMLString(&View{
		Scene:     apiv0.SceneResponse{Width: 800, Height: 600, BackgroundHTML: `<svg id="bg"></svg>`},
		Connected: true,
	})
	if !strings.Contains(html, `sandbox="`+uiutil.SceneBackgroundSandbox+`"`) || !strings.Contains(html, `<svg id=\"bg\"></svg>`) {
		t.Errorf("the background is missing: %s", html)
	}
}
