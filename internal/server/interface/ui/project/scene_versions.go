package project

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/maxence-charriere/go-app/v10/pkg/app"

	apiv0 "github.com/kipitix/growscada/contract/api/v0"
	"github.com/kipitix/growscada/internal/apiclient"
	"github.com/kipitix/growscada/internal/server/interface/ui/uiutil"
)

// ── Scene versions and states ─────────────────────────────────────────────────

// sceneMark is the moment a request of Scenes was sent, for both
// sceneVersions and sceneStates.
type sceneMark struct{ versions, states uiutil.Mark }

func (p *Project) markScenes() sceneMark {
	return sceneMark{versions: p.sceneVersions.Mark(), states: p.sceneStates.Mark()}
}

// observeSceneVersion records version of the Scene id, stated by a response
// to a request sent at mark that does not carry its Widgets. A write of the
// selected Scene's Widgets states the version they are shown at, so while
// some are shown it is taken from them alone: a newer version rereads them
// (after a drag, see loadWidgets) instead of being recorded over the Widgets
// it would overwrite.
func (p *Project) observeSceneVersion(ctx app.Context, mark uiutil.Mark, id uuid.UUID, version int) {
	if id == p.selectedSceneID && len(p.widgets) > 0 && p.widgets[0].SceneID == id {
		if version > p.sceneVersions.Of(id) {
			p.loadWidgets(ctx)
		}
		return
	}
	p.sceneVersions.Observe(mark, id, version)
}

// forgetSceneVersion forgets what is known of the Scene id's version, which
// the server may have taken back.
func (p *Project) forgetSceneVersion(id uuid.UUID) {
	p.sceneVersions.ForgetID(id)
	p.sceneStates.ForgetID(id)
}

// loadScene rereads the Scene sceneID, whose version a change of one of its
// Widgets made stale (loadStaleScenes).
func (p *Project) loadScene(ctx app.Context, sceneID uuid.UUID) {
	if p.staleScenes == nil {
		p.staleScenes = make(map[uuid.UUID]struct{})
	}
	p.staleScenes[sceneID] = struct{}{}
	p.loadStaleScenes(ctx)
}

// loadStaleScenes rereads the stale Scenes, one request at a time; those
// marked meanwhile wait for it. One Scene is reread by itself and put in
// place in the list: only scene events and a resync change which Scenes the
// list holds, so a Scene not in it gives its version alone, one gone
// meanwhile (404) nothing. Several (a changed WidgetType reconciles every
// Scene using it) are reread with the list, in turn with its other loads.
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
	mark := p.markScenes()
	uiutil.Load(p.compoCtx, &p.staleScenesReloader, func(c context.Context) (*apiv0.SceneResponse, error) {
		sc, err := p.client.GetScene(c, sceneID)
		if errors.Is(err, apiclient.ErrNotFound) {
			return nil, nil
		}
		return &sc, err
	}, func(ctx app.Context, sc *apiv0.SceneResponse) {
		if sc != nil {
			p.setScene(ctx, mark, *sc)
		}
	}, p.loadStaleScenes)
}

// setScene records the version of the reread Scene sc and, if the list holds
// an older state of it, puts sc in its place and merges the scene properties
// editor when it is the selected one. mark is the moment it was requested.
func (p *Project) setScene(ctx app.Context, mark sceneMark, sc apiv0.SceneResponse) {
	p.observeSceneVersion(ctx, mark.versions, sc.ID, sc.Version)
	i := uiutil.IdxByID(p.scenes, sc.ID, uiutil.SceneID)
	if i < 0 || !p.sceneStates.Advance(mark.states, sc.ID, sc.Version) {
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
