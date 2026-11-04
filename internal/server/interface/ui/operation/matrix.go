package operation

import (
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// revisionMatrixCSS derives the CSS matrix() of a Widget of the Revision from
// its geometry (see uiutil.MatrixCSS, which mirrors the domain formula).
func revisionMatrixCSS(widget projectv0.Widget) string {
	return uiutil.MatrixCSS(
		widget.Position.X, widget.Position.Y,
		widget.Origin.X, widget.Origin.Y,
		widget.Rotation.Degrees,
		widget.Size.Width, widget.Size.Height,
	)
}
