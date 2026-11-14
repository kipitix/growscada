package uiutil

import (
	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// The icons of the modes' panels (task 84): inline SVG, so no icon font and no
// image asset, the stroke follows the button's colour, and data-icon lets a test
// find one. Emoji were rejected for the picker's actions for the same reason
// they are rejected here: their colour and metrics differ across platforms.
const (
	iconBackSVG    = `<svg data-icon="back" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 12H5"/><path d="M11 18l-6-6 6-6"/></svg>`
	iconSaveSVG    = `<svg data-icon="save" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><path d="M17 21v-8H7v8"/><path d="M7 3v5h8"/></svg>`
	iconUndoSVG    = `<svg data-icon="undo" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 14L4 9l5-5"/><path d="M4 9h11a5 5 0 0 1 0 10h-3"/></svg>`
	iconRedoSVG    = `<svg data-icon="redo" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M15 14l5-5-5-5"/><path d="M20 9H9a5 5 0 0 0 0 10h3"/></svg>`
	iconHistorySVG = `<svg data-icon="history" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 12a9 9 0 1 0 3-6.7"/><path d="M3 4v5h5"/><path d="M12 8v5l3 2"/></svg>`
	iconDeploySVG  = `<svg data-icon="deploy" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 16V4"/><path d="M6 10l6-6 6 6"/><path d="M4 20h16"/></svg>`
	iconLayersSVG  = `<svg data-icon="layers" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3l9 5-9 5-9-5 9-5z"/><path d="M3 13l9 5 9-5"/></svg>`
	iconFileSVG    = `<svg data-icon="file" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8l-5-5z"/><path d="M14 3v5h5"/></svg>`
	iconCheckSVG   = `<svg data-icon="check" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 13l4 4L19 7"/></svg>`
	iconCloseSVG   = `<svg data-icon="close" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 6l12 12M18 6L6 18"/></svg>`
	iconPencilSVG  = `<svg data-icon="pencil" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M4 20h4L19 9l-4-4L4 16v4z"/><path d="M14 6l4 4"/></svg>`
	iconTagSVG     = `<svg data-icon="tag" aria-hidden="true" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 13l-7 7-9-9V4h7l9 9z"/><circle cx="8" cy="8" r="1.5"/></svg>`
)

// IconBack is the way back to a mode's selection page.
func IconBack() app.UI { return app.Raw(iconBackSVG) }

// IconSave is the Save of a Draft as the next saved state.
func IconSave() app.UI { return app.Raw(iconSaveSVG) }

// IconUndo and IconRedo step the DraftChange stack.
func IconUndo() app.UI { return app.Raw(iconUndoSVG) }
func IconRedo() app.UI { return app.Raw(iconRedoSVG) }

// IconHistory opens the saved states of the Draft.
func IconHistory() app.UI { return app.Raw(iconHistorySVG) }

// IconDeploy makes a Release the running Deployment.
func IconDeploy() app.UI { return app.Raw(iconDeploySVG) }

// IconLayers opens the Deployments of a Project.
func IconLayers() app.UI { return app.Raw(iconLayersSVG) }

// IconFile opens the project file of a Project.
func IconFile() app.UI { return app.Raw(iconFileSVG) }

// IconLoad loads a saved state into the Draft.

// IconCheck saves the edit of a field in place.
func IconCheck() app.UI { return app.Raw(iconCheckSVG) }

// IconClose drops the edit of a field in place.
func IconClose() app.UI { return app.Raw(iconCloseSVG) }

// IconPencil edits a field in place.
func IconPencil() app.UI { return app.Raw(iconPencilSVG) }

// IconTag opens the labels of the server (task 85_2).
func IconTag() app.UI { return app.Raw(iconTagSVG) }
