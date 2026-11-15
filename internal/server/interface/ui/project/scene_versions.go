package project

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v11/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Scenes and the Draft's one Version ────────────────────────────────────────
//
// One Version serves the whole Draft (ADR 0016): a Scene, its Widgets, a Tag,
// an import all carry it, and it is the token every write states (load.go). It is
// therefore also what says whether a reread state is newer than the one shown;
// there are no per-Scene versions any more.

// markStale records sceneID as a Scene to reread (loadScene): its state a
// change of one of its Widgets made stale.
func (p *Project) markStale(sceneID uuid.UUID) {
	if p.staleScenes == nil {
		p.staleScenes = make(map[uuid.UUID]struct{})
	}
	p.staleScenes[sceneID] = struct{}{}
}

// loadScene rereads the Scene sceneID, whose state a change of one of its
// Widgets made stale (loadStaleScenes).
func (p *Project) loadScene(ctx app.Context, sceneID uuid.UUID) {
	p.markStale(sceneID)
	p.loadStaleScenes(ctx)
}

// loadStaleScenes rereads the stale Scenes, one request at a time; those
// marked meanwhile wait for it. One Scene is reread by itself and put in
// place in the list: only scene events and a resync change which Scenes the
// list holds, so a Scene not in it gives its version alone, one gone
// meanwhile (404) nothing. Several (a changed WidgetType reconciles every
// Scene using it) are reread with the list, in turn with its other loads.
//
// A failed read other than a 404 keeps the Scene stale: it is reread by the
// next load, without a timer. Forgetting it would leave the Draft's Version
// at the one the change replaced, and the next write of the Scene would be
// refused as stale by the server (task 61).
func (p *Project) loadStaleScenes(ctx app.Context) {
	if p.staleScenesReloader.Loading() {
		p.staleScenesReloader.Start() // queues a rerun
		return
	}
	if len(p.staleScenes) > 1 {
		clear(p.staleScenes)
		p.loadScenes(ctx)
		return
	}
	var sceneID uuid.UUID
	for sceneID = range p.staleScenes {
	}
	if sceneID == uuid.Nil {
		return
	}
	clear(p.staleScenes)
	content := p.content
	if content == nil {
		return
	}
	mark := p.markDraft()
	if !p.staleScenesReloader.Start() {
		return
	}
	uiutil.Do(p.compoCtx, func(c context.Context) (*apiv0.SceneResponse, error) {
		sc, err := content.GetScene(c, sceneID)
		return &sc, err
	}, func(ctx app.Context, sc *apiv0.SceneResponse, err error) {
		rerun := p.staleScenesReloader.Done()
		switch {
		case errors.Is(err, apiclient.ErrNotFound):
			// The Scene is gone: there is nothing to reread.
		case err != nil:
			// The state the change made stays unknown until the read that
			// answers: a write of this Scene would state a stale Version.
			p.markStale(sceneID)
			uiutil.ShowError(ctx, err)
			ctx.Update()
		case content == p.content:
			p.setScene(ctx, mark, *sc)
		}
		if rerun {
			p.loadStaleScenes(ctx)
		}
	})
}

// setScene records the reread Scene sc, advancing the Draft's Version by it,
// and — if the list holds an older state of it — puts sc in its place and
// merges the scene properties editor when it is the selected one. mark is the
// moment it was requested.
func (p *Project) setScene(ctx app.Context, mark uiutil.Mark, sc apiv0.SceneResponse) {
	fresh := p.draftVersions.Advance(mark, p.projectID, sc.Version)
	p.reloadShownWidgets(ctx, sc.ID, fresh)
	i := uiutil.IdxByID(p.scenes, sc.ID, uiutil.SceneID)
	if i < 0 || !fresh {
		return
	}
	old := p.scenes[i]
	scenes := slices.Clone(p.scenes)
	scenes[i] = sc
	p.scenes = scenes
	if sc.ID == p.selectedSceneID {
		p.mergeSceneEditingFields(old, sc)
	}
}

// reloadShownWidgets reads the shown Scene's Widgets again when a newer state
// of the Scene id arrived (fresh): they carry the Version they were read at,
// and the change may be one of them — or the whole Draft was replaced and the
// list of it is the truth (task 72_6). Widgets of another Scene are the caller's
// to read when the selection moves.
func (p *Project) reloadShownWidgets(ctx app.Context, id uuid.UUID, fresh bool) {
	if !fresh || id != p.selectedSceneID {
		return
	}
	if len(p.widgets) > 0 && p.widgets[0].SceneID != id {
		return
	}
	p.loadWidgets(ctx)
}
