package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/librarypin"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// WidgetReconciliationPolicyName is the Name of the policy returned by
// NewWidgetReconciliationPolicy.
const WidgetReconciliationPolicyName = "reconcile_widgets_with_pins"

// widgetReconciliationPolicy removes, from the Widgets of a Project, the
// PortBindings that are no longer valid (ADR 0010) against the WidgetTypes
// of the Library Releases the Project pins: of ports a WidgetType no longer
// declares, of Tags whose TagType a port's type hint no longer accepts and
// of Tags that no longer exist. A library_pin_updated event may have dropped
// a port or narrowed a type hint: its job reconciles every Scene of the
// Project (ADR 0012). A widget_created or widget_updated event may have
// bound a port that a concurrent switch of a pin had just dropped or
// narrowed, or a Tag deleted concurrently (the Widget was checked against
// the WidgetType and the Tags as read before that change): its job
// reconciles only the Scene of the Widget. The events of the policy's own
// reconciliations bind nothing and enqueue no job. A change of a WidgetType
// in a Library's Draft touches no Scene.
type widgetReconciliationPolicy struct {
	scopes ProjectScopes
}

var _ Policy = (*widgetReconciliationPolicy)(nil)

// NewWidgetReconciliationPolicy returns the policy bringing Widgets into line
// with the WidgetTypes their Project pins.
func NewWidgetReconciliationPolicy(someScopes ProjectScopes) Policy {
	return &widgetReconciliationPolicy{scopes: someScopes}
}

func (p widgetReconciliationPolicy) Name() string {
	return WidgetReconciliationPolicyName
}

// Job keys: "project:<id>" for every Scene of a Project,
// "project:<id>/scene:<id>" for one Scene.
const sceneKeySeparator = "/scene:"

func (p widgetReconciliationPolicy) JobKey(e event.Event) (string, bool) {
	switch e.Type() {
	case event.EventTypeLibraryPinUpdated:
		if ev, ok := e.(librarypin.LibraryPinEvent); ok {
			return projectKeyPrefix + ev.ProjectID().String(), true
		}
	case event.EventTypeWidgetCreated, event.EventTypeWidgetUpdated:
		if ev, ok := e.(scene.WidgetUpdatedEvent); ok && ev.ByReconciliation() {
			return "", false
		}
		if ev, ok := e.(scene.WidgetTypedEvent); ok {
			return projectKeyPrefix + ev.ProjectID().String() + sceneKeySeparator + ev.SceneID().String(), true
		}
	}
	return "", false
}

// Run reconciles the Scenes of the job key, each in its own transaction, with
// the WidgetTypes the Project pins as they are now: every Scene of the
// Project, or the one Scene the key names. A Scene that fails does not stop
// the others: Run returns the errors of all that failed, and the job is run
// again, the Scenes already reconciled then left as they are. A deleted
// Scene has no Widgets left to reconcile.
func (p widgetReconciliationPolicy) Run(ctx context.Context, key string) error {
	projectID, rest, err := cutProjectKey(key)
	if err != nil {
		return err
	}
	scope := p.scopes.ForProject(projectID)
	if rest == "" {
		scenes, err := scope.Scenes().FindAll(ctx)
		if err != nil {
			return fmt.Errorf("cannot list scenes of project %s: %w", projectID, err)
		}
		return p.reconcileScenes(ctx, scope, scenes)
	}

	rawSceneID, ok := strings.CutPrefix(rest, sceneKeySeparator)
	if !ok {
		return fmt.Errorf("invalid job key %q", key)
	}
	sceneUUID, err := uuid.Parse(rawSceneID)
	if err != nil {
		return fmt.Errorf("invalid scene ID in job key %q: %w", key, err)
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](sceneUUID))
	sc, err := scope.Scenes().FindByID(ctx, sceneID)
	if errors.Is(err, scene.ErrSceneNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot find scene %s: %w", sceneID, err)
	}
	return p.reconcileScenes(ctx, scope, []scene.Scene{sc})
}

// reconcileScenes reconciles scenes, Scenes of scope's Project read through
// it, with the WidgetTypes the Project pins.
func (p widgetReconciliationPolicy) reconcileScenes(ctx context.Context, scope ProjectScope, scenes []scene.Scene) error {
	if err := changeEachScene(ctx, scope.Scenes(), scenes, p.reconcile(scope)); err != nil {
		return fmt.Errorf("cannot reconcile with pinned widget types: %w", err)
	}
	return nil
}

// reconcile returns the change reconciling a Scene of scope's Project with
// the WidgetTypes the Project pins and the Tags its Widgets bind, read
// through scope. They are read after the Scene, at every attempt of
// changeScene, so every Widget the Scene holds was checked against them or
// older ones, and no binding valid now is taken for an orphan. A Widget of a
// WidgetType no pinned release holds — none may be, but a pin removed
// concurrently with this read — is left as it is.
func (p widgetReconciliationPolicy) reconcile(scope ProjectScope) sceneChange {
	return func(ctx context.Context, sc scene.Scene) (scene.Scene, bool, error) {
		pinned, err := scope.LibraryPins().FindPinnedWidgetTypes(ctx)
		if err != nil {
			return nil, false, fmt.Errorf("cannot read pinned widget types: %w", err)
		}
		byID := make(map[id.ID[library.WidgetType]]library.ReleasedWidgetType, len(pinned))
		for _, wt := range pinned {
			byID[wt.ID()] = wt
		}
		tagTypes, err := loadTagTypes(ctx, scope, scene.BoundTagIDs(sc.Widgets()...))
		if err != nil {
			return nil, false, err
		}
		changed := false
		seen := map[id.ID[library.WidgetType]]struct{}{}
		for _, w := range sc.Widgets() {
			if _, done := seen[w.TypeID()]; done {
				continue
			}
			seen[w.TypeID()] = struct{}{}
			wt, ok := byID[w.TypeID()]
			if !ok {
				continue
			}
			var removed bool
			if sc, removed, err = sc.ReconcileWith(wt, tagTypes); err != nil {
				return nil, false, err
			}
			changed = changed || removed
		}
		return sc, changed, nil
	}
}
