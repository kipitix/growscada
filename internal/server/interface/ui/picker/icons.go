package picker

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// The icons of the page (task 82): inline SVG, so no icon font and no image
// asset, the stroke follows the button's colour, and data-icon lets a test
// find one. Emoji were rejected: their colour and metrics differ across
// platforms.
const (
	iconPlusSVG   = `<svg data-icon="plus" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 5v14M5 12h14"/></svg>`
	iconPencilSVG = `<svg data-icon="pencil" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 20h4L19 9l-4-4L4 16v4z"/><path d="M14 6l4 4"/></svg>`
	iconTrashSVG  = `<svg data-icon="trash" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 7h16M9 7V5h6v2"/><path d="M6 7l1 13h10l1-13"/></svg>`
	iconOpenSVG   = `<svg data-icon="open" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14M13 6l6 6-6 6"/></svg>`
	iconCheckSVG  = `<svg data-icon="check" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 13l4 4L19 7"/></svg>`
	iconCloseSVG  = `<svg data-icon="close" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>`
	iconGripSVG   = `<svg data-icon="grip" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><path d="M9 6h.01M9 12h.01M9 18h.01M15 6h.01M15 12h.01M15 18h.01"/></svg>`
)

// iconPlus is Create, the empty page's call to action and the draft's row.
func iconPlus() app.UI { return app.Raw(iconPlusSVG) }

// iconPencil edits a field in place: the way in where a double click is not
// available (task 82).
func iconPencil() app.UI { return app.Raw(iconPencilSVG) }

// iconTrash deletes the selected entity.
func iconTrash() app.UI { return app.Raw(iconTrashSVG) }

// iconOpen opens the editor of the selected entity: selection is not opening.
func iconOpen() app.UI { return app.Raw(iconOpenSVG) }

// iconCheck saves the edit of a field.
func iconCheck() app.UI { return app.Raw(iconCheckSVG) }

// iconClose drops the edit of a field: the way out of it beside the pencil.
func iconClose() app.UI { return app.Raw(iconCloseSVG) }

// iconGrip is the handle a drag starts on.
func iconGrip() app.UI { return app.Raw(iconGripSVG) }
