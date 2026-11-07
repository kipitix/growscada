package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	projectv0 "github.com/kipitix/growscada/contract/project/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// The Project file dialog (task 39, ADR 0018): one button in the header opens
// it, and from there the Draft or a Revision is downloaded, a file is opened
// as the whole Draft, or a file is deployed as a new Revision. A file is read
// once, confirmed with its counts, and does not survive a reload.

// pendingProjectFile is a picked file waiting for confirmation.
type pendingProjectFile struct {
	// action is "open" (replace the Draft) or "deploy" (make a Revision).
	action string
	name   string
	data   []byte
	file   projectv0.ProjectFile
}

func (p *Project) openProjectFileDialog(ctx app.Context) {
	if p.content == nil {
		return
	}
	p.projectFileDialog = true
	p.pendingProjectFile = nil
	p.projectFileIssues = nil
	p.projectFileDetail = ""
	ctx.Update()
}

func (p *Project) closeProjectFileDialog() {
	p.projectFileDialog = false
	p.pendingProjectFile = nil
	p.projectFileIssues = nil
	p.projectFileDetail = ""
}

// downloadDraftProjectFile saves the Draft as a ProjectFile. A Draft the
// builder refuses is the toast, as any other server error.
func (p *Project) downloadDraftProjectFile(ctx app.Context) {
	if p.content == nil || p.projectFileBusy {
		return
	}
	p.projectFileBusy = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiclient.ProjectFileDownload, error) {
		return pc.DraftProjectFile(c)
	}, func(ctx app.Context, download apiclient.ProjectFileDownload) {
		p.projectFileBusy = false
		saveProjectFile(download.Filename, download.Data)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.projectFileBusy = false
		uiutil.ShowError(ctx, err)
		ctx.Update()
	})
}

// downloadRevisionProjectFile saves the document of a Revision.
func (p *Project) downloadRevisionProjectFile(ctx app.Context, number int) {
	if p.content == nil {
		return
	}
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiclient.ProjectFileDownload, error) {
		return pc.RevisionProjectFile(c, number)
	}, func(ctx app.Context, download apiclient.ProjectFileDownload) {
		saveProjectFile(download.Filename, download.Data)
		ctx.Update()
	}, nil)
}

// pickProjectFile shows the confirmation of a picked file: its name, schema
// version, project and counts. A document that is not JSON is a problem of
// the file, listed in the dialog instead of a toast.
func (p *Project) pickProjectFile(ctx app.Context, action, name string, data []byte) {
	var file projectv0.ProjectFile
	if err := json.Unmarshal(data, &file); err != nil {
		p.pendingProjectFile = nil
		p.projectFileIssues = []apiv0.FileIssue{{Path: "", Message: err.Error()}}
		p.projectFileDetail = "the file is not a project file"
		ctx.Update()
		return
	}
	p.pendingProjectFile = &pendingProjectFile{action: action, name: name, data: data, file: file}
	p.projectFileIssues = nil
	p.projectFileDetail = ""
	ctx.Update()
}

// confirmPendingProjectFile runs the confirmed action.
func (p *Project) confirmPendingProjectFile(ctx app.Context) {
	pending := p.pendingProjectFile
	if pending == nil || p.projectFileBusy {
		return
	}
	if pending.action == "open" {
		p.doOpenProjectFile(ctx, pending)
		return
	}
	p.doDeployProjectFile(ctx, pending)
}

// doOpenProjectFile opens the file as the whole Draft: one DraftChange, and
// the content and the stack are reread afterwards.
func (p *Project) doOpenProjectFile(ctx app.Context, pending *pendingProjectFile) {
	if p.waitMoves(ctx, func(ctx app.Context) { p.doOpenProjectFile(ctx, pending) }) {
		return
	}
	p.projectFileBusy = true
	mark := p.markDraft()
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DraftStateResponse, error) {
		return pc.OpenProjectFile(c, p.draftVersion(), pending.data)
	}, func(ctx app.Context, opened apiv0.DraftStateResponse) {
		p.projectFileBusy = false
		p.observeDraftVersion(mark, opened.Version)
		p.closeProjectFileDialog()
		p.loadScenes(ctx)
		p.loadTags(ctx)
		p.loadWidgetTypes(ctx)
		p.loadPins(ctx)
		p.loadDeploy(ctx)
		p.loadDraftChanges(ctx)
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.projectFileBusy = false
		p.showProjectFileError(ctx, err)
	})
}

// doDeployProjectFile makes the file a Revision without the Draft: Operation
// switches to it and the Draft stays as it is.
func (p *Project) doDeployProjectFile(ctx app.Context, pending *pendingProjectFile) {
	if p.waitMoves(ctx, func(ctx app.Context) { p.doDeployProjectFile(ctx, pending) }) {
		return
	}
	p.projectFileBusy = true
	callContent(p, func(c context.Context, pc *apiclient.ProjectClient) (apiv0.DeployResponse, error) {
		return pc.DeployProjectFile(c, pending.data)
	}, func(ctx app.Context, _ apiv0.DeployResponse) {
		p.projectFileBusy = false
		p.closeProjectFileDialog()
		p.loadDeploy(ctx)
		if p.revisionsDialog {
			p.loadRevisions(ctx)
		}
		ctx.Update()
	}, func(ctx app.Context, err error) {
		p.projectFileBusy = false
		p.showProjectFileError(ctx, err)
	})
}

// showProjectFileError lists every problem the server found in the file; any
// other failure is a toast.
func (p *Project) showProjectFileError(ctx app.Context, err error) {
	var statusErr *apiclient.StatusError
	if errors.As(err, &statusErr) && len(statusErr.Problem.FileIssues) > 0 {
		p.projectFileIssues = statusErr.Problem.FileIssues
		p.projectFileDetail = statusErr.Problem.Detail
		ctx.Update()
		return
	}
	uiutil.ShowError(ctx, err)
	ctx.Update()
}

// renderProjectFileDialog is the dialog of the header's button: the three
// actions, the confirmation of a picked file with its counts, and every
// problem the server found.
func (p *Project) renderProjectFileDialog() app.UI {
	body := []app.UI{
		app.P().
			Style("margin", "0 0 8px").
			Style("color", "var(--text-2)").
			Text("A project file is the whole configuration as one document: the Draft, or any Revision. It replaces the Draft or becomes a Revision; it is not kept after a reload."),
		app.Div().
			Style("display", "flex").
			Style("flex-wrap", "wrap").
			Style("gap", "8px").
			Body(
				p.projectFileButton("Download Draft", "Save the Draft as a project file", !p.projectFileBusy,
					func(ctx app.Context) { p.downloadDraftProjectFile(ctx) }),
				p.projectFilePickButton("Open a file…", p.projectFileOpenTitle(),
					!p.projectFileBusy, "open"),
				p.projectFilePickButton("Deploy a file…", "Make a file the Project's new Revision; the Draft does not change",
					!p.projectFileBusy, "deploy"),
			),
	}
	if p.pendingProjectFile != nil {
		body = append(body, p.renderProjectFileConfirmation(p.pendingProjectFile))
	}
	if len(p.projectFileIssues) > 0 {
		body = append(body, p.renderProjectFileIssues())
	}
	return app.Div().
		Attr("data-projectfile-dialog", "true").
		Style("position", "absolute").
		Style("top", "48px").
		Style("right", "16px").
		Style("z-index", "20").
		Style("width", "520px").
		Style("padding", "14px").
		Style("border", "1px solid var(--border)").
		Style("border-radius", "6px").
		Style("background", "var(--bg)").
		Style("box-shadow", "var(--toast-shadow)").
		Style("font-size", "13px").
		Body(
			app.H3().Style("margin", "0 0 8px").Text("Project file"),
			app.Div().Body(body...),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("gap", "8px").
				Style("margin-top", "10px").
				Body(app.Button().
					Style("padding", "4px 12px").
					Style("font-size", "13px").
					Style("cursor", "pointer").
					Text("Close").
					OnClick(func(ctx app.Context, e app.Event) {
						p.closeProjectFileDialog()
						ctx.Update()
					})),
		)
}

// renderProjectFileConfirmation is the picked file as it would apply: what it
// is, what it would do, and a warning for the action it cannot take.
func (p *Project) renderProjectFileConfirmation(pending *pendingProjectFile) app.UI {
	file := pending.file
	widgets := 0
	for _, scene := range file.Scenes {
		widgets += len(scene.Widgets)
	}
	action := "Open it as the Draft: it replaces the whole Draft, and undo restores the previous one."
	if pending.action == "deploy" {
		action = "Deploy it: Operation switches to a new Revision, and the Draft does not change."
	}
	lines := []app.UI{
		app.P().
			Style("margin", "8px 0 4px").
			Style("font-weight", "600").
			Text(pending.name),
		app.P().
			Style("margin", "0 0 4px").
			Style("color", "var(--text-2)").
			Text(fmt.Sprintf("Schema %s · project %s", file.SchemaVersion, file.ProjectID)),
		app.P().
			Style("margin", "0 0 4px").
			Style("color", "var(--text-2)").
			Text(fmt.Sprintf("%d tags, %d scenes, %d widgets, %d devices, %d environments, %d library pins",
				len(file.Tags), len(file.Scenes), widgets, len(file.Devices), len(file.Environments), len(file.LibraryPins))),
		app.P().
			Style("margin", "0 0 4px").
			Text(action),
	}
	mismatch := file.ProjectID != p.projectID
	if mismatch {
		lines = append(lines, app.P().
			Style("margin", "0 0 4px").
			Style("color", "var(--toast-warn-title)").
			Text("The file configures another project and is refused."))
	}
	if pending.action == "deploy" {
		lines = append(lines, app.P().
			Style("margin", "0 0 4px").
			Style("color", "var(--text-muted)").
			Text(p.currentRevisionText()))
	}
	confirm := app.Button().
		Attr("data-projectfile-confirm", "true").
		Disabled(p.projectFileBusy || mismatch).
		Style("padding", "4px 12px").
		Style("font-size", "13px").
		Style("cursor", "pointer").
		Style("background", "var(--accent)").
		Style("color", "var(--accent-text)").
		Text(map[string]string{"open": "Open", "deploy": "Deploy"}[pending.action]).
		OnClick(func(ctx app.Context, e app.Event) {
			p.confirmPendingProjectFile(ctx)
		})
	return app.Div().
		Attr("data-projectfile-confirmation", pending.action).
		Style("margin", "10px 0 0").
		Style("padding", "8px").
		Style("border-top", "1px solid var(--border)").
		Body(
			app.Div().Body(lines...),
			app.Div().
				Style("display", "flex").
				Style("justify-content", "flex-end").
				Style("gap", "8px").
				Body(
					app.Button().
						Style("padding", "4px 12px").
						Style("font-size", "13px").
						Style("cursor", "pointer").
						Text("Cancel").
						OnClick(func(ctx app.Context, e app.Event) {
							p.pendingProjectFile = nil
							ctx.Update()
						}),
					confirm,
				),
		)
}

// currentRevisionText names the Revision the Project runs, for the deploy
// confirmation.
func (p *Project) currentRevisionText() string {
	if !p.deployLoaded {
		return "The current Revision is being read."
	}
	if p.deployPreview.CurrentRevision == nil {
		return "The project has not been deployed yet."
	}
	return fmt.Sprintf("Current revision: %d, deployed %s.",
		p.deployPreview.CurrentRevision.Number, formatDeployedAt(p.deployPreview.CurrentRevision.DeployedAt))
}

// renderProjectFileIssues lists every problem of the file: its path in the
// document and what is wrong there.
func (p *Project) renderProjectFileIssues() app.UI {
	rows := make([]app.UI, 0, len(p.projectFileIssues))
	for _, issue := range p.projectFileIssues {
		path := issue.Path
		if path == "" {
			path = "(document)"
		}
		rows = append(rows, app.Li().
			Style("margin", "0 0 3px").
			Text(path+": "+issue.Message))
	}
	return app.Div().
		Attr("data-projectfile-issues", "true").
		Style("margin", "8px 0 0").
		Style("color", "var(--toast-warn-title)").
		Body(
			app.P().
				Style("margin", "0 0 4px").
				Text(p.projectFileDetail),
			app.Ul().Style("margin", "0").Style("padding-left", "18px").Body(rows...),
		)
}

// projectFileOpenTitle is the hover text of the open action: what it does.
func (p *Project) projectFileOpenTitle() string {
	return "Replace the whole Draft with a file; can be undone"
}

func (p *Project) projectFileButton(label, title string, enabled bool, onClick func(app.Context)) app.UI {
	return app.Button().
		Title(title).
		Disabled(!enabled).
		Style("padding", "2px 9px").
		Style("font-size", "12px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Text(label).
		OnClick(func(ctx app.Context, e app.Event) { onClick(ctx) }, app.EventScope("projectfile", label))
}

// projectFilePickButton is a button that opens the browser's file picker: a
// labelled file input, so a click and the file's choice are one control.
func (p *Project) projectFilePickButton(label, title string, enabled bool, scope string) app.UI {
	input := app.Input().
		Type("file").
		Accept(".json,application/json").
		ID("projectfile-"+scope).
		Name("projectfile-"+scope).
		Attr("data-projectfile-input", scope).
		Attr("aria-label", label).
		Title(title).
		Disabled(!enabled).
		Style("position", "absolute").
		Style("inline-size", "1px").
		Style("block-size", "1px").
		Style("opacity", "0").
		Style("overflow", "hidden")
	if !enabled {
		return app.Button().
			Title(title).
			Disabled(true).
			Style("padding", "2px 9px").
			Style("font-size", "12px").
			Style("cursor", "not-allowed").
			Style("border", "1px solid var(--border-input)").
			Style("border-radius", "4px").
			Style("background", "var(--bg)").
			Style("color", "var(--text)").
			Text(label)
	}
	return app.Label().
		Title(title).
		Style("position", "relative").
		Style("display", "inline-flex").
		Style("align-items", "center").
		Style("padding", "2px 9px").
		Style("font-size", "12px").
		Style("cursor", "pointer").
		Style("border", "1px solid var(--border-input)").
		Style("border-radius", "4px").
		Style("background", "var(--bg)").
		Style("color", "var(--text)").
		Body(
			app.Span().Text(label),
			input.OnChange(func(ctx app.Context, e app.Event) {
				files := e.Get("target").Get("files")
				if files.Length() == 0 {
					return
				}
				readProjectFile(files.Index(0), func(name string, data []byte, err error) {
					p.compoCtx.Dispatch(func(ctx app.Context) {
						if err != nil {
							p.projectFileIssues = []apiv0.FileIssue{{Path: "", Message: err.Error()}}
							p.projectFileDetail = "the file cannot be read"
							ctx.Update()
							return
						}
						p.pickProjectFile(ctx, scope, name, data)
					})
				})
			}),
		)
}

// readProjectFile reads the picked file as bytes and calls done once. It is
// browser-only: the tests call pickProjectFile with the bytes directly.
func readProjectFile(file app.Value, done func(name string, data []byte, err error)) {
	reader := app.Window().Get("FileReader").New()
	var loaded, failed app.Func
	release := func() {
		loaded.Release()
		failed.Release()
	}
	loaded = app.FuncOf(func(app.Value, []app.Value) any {
		defer release()
		buffer := app.Window().Get("Uint8Array").New(reader.Get("result"))
		data := make([]byte, buffer.Length())
		app.CopyBytesToGo(data, buffer)
		done(file.Get("name").String(), data, nil)
		return nil
	})
	failed = app.FuncOf(func(app.Value, []app.Value) any {
		defer release()
		done("", nil, errors.New("the file cannot be read"))
		return nil
	})
	reader.Set("onload", loaded)
	reader.Set("onerror", failed)
	reader.Call("readAsArrayBuffer", file)
}

// saveProjectFile hands bytes to the browser as a download under name: a Blob
// and an anchor with the download attribute.
func saveProjectFile(name string, data []byte) {
	window := app.Window()
	array := window.Get("Uint8Array").New(len(data))
	app.CopyBytesToJS(array, data)
	blob := window.Get("Blob").New([]any{array}, map[string]any{"type": "application/json"})
	url := window.Get("URL").Call("createObjectURL", blob)
	document := window.Get("document")
	anchor := document.Call("createElement", "a")
	anchor.Set("href", url)
	anchor.Set("download", name)
	document.Get("body").Call("appendChild", anchor)
	anchor.Call("click")
	document.Get("body").Call("removeChild", anchor)
	window.Get("URL").Call("revokeObjectURL", url)
}
