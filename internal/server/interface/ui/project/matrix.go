package project

import (
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Matrix helper ─────────────────────────────────────────────────────────────

// widgetMatrixCSS is the CSS matrix() of a Widget of the Draft, from its
// geometric properties (see uiutil.MatrixCSS, which mirrors the domain
// TransformationMatrix formula).
func widgetMatrixCSS(w apiv0.WidgetResponse) string {
	return uiutil.MatrixCSS(
		w.Position.X, w.Position.Y,
		w.Origin.X, w.Origin.Y,
		w.Rotation.Degrees,
		w.Size.Width, w.Size.Height,
	)
}
