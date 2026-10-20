package livescene

import (
	"testing"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

func TestView_RendersTheSceneBackgroundUnderTheWidgets(t *testing.T) {
	for _, bg := range []string{`<svg id="bg"></svg>`, ""} {
		v := &View{
			Scene:     apiv0.SceneResponse{Width: 800, Height: 600, BackgroundHTML: bg},
			Connected: true,
		}
		layers := v.layers("")
		if b, ok := layers[0].(*uiutil.SceneBackground); !ok || b.HTML != bg {
			t.Errorf("bottom layer = %#v, want the background %q", layers[0], bg)
		}
	}
}
