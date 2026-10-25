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
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// WidgetReconciliationPolicyName is the Name of the policy returned by
// NewWidgetReconciliationPolicy.
const WidgetReconciliationPolicyName = "reconcile_widgets_with_widget_type"

// widgetReconciliationPolicy removes, from Widgets of a WidgetType, the
// PortBindings that are no longer valid (ADR 0010): of ports the WidgetType
// no longer declares, of Tags whose TagType a port's type hint no longer
// accepts and of Tags that no longer exist. A widget_type_updated event may
// have dropped a port or narrowed a type hint: its job reconciles every Scene
// holding Widgets of the type. A widget_created or widget_updated event may
// have bound a port that a concurrent update of the WidgetType had just
// dropped or narrowed, or a Tag deleted concurrently (the Widget was checked
// against the WidgetType and the Tags as read before that change): its job
// reconciles only the Scene of the Widget. Every Scene is reconciled through
// the repositories of its own Project, with that Project's Tags; a
// WidgetType is shared by the Projects of the server until Libraries exist
// (ADR 0006), so a widget_type_updated job goes through each Project holding
// Widgets of the type. The events of the policy's own reconciliations bind
// nothing and enqueue no job.
//
// It is transitional: once a Project sees only a Library Release (ADR 0007),
// reconciliation moves to the DraftChange of a pinned release (task 33).
type widgetReconciliationPolicy struct {
	widgetTypes library.WidgetTypeRepository
	scopes      ProjectScopes
}

var _ Policy = (*widgetReconciliationPolicy)(nil)

// NewWidgetReconciliationPolicy returns the policy bringing Widgets into line
// with their WidgetType.
func NewWidgetReconciliationPolicy(aWidgetTypeRepository library.WidgetTypeRepository, someScopes ProjectScopes) Policy {
	return &widgetReconciliationPolicy{
		widgetTypes: aWidgetTypeRepository,
		scopes:      someScopes,
	}
}

func (p widgetReconciliationPolicy) Name() string {
	return WidgetReconciliationPolicyName
}

// Job keys: "widget_type:<id>" for every Scene of a WidgetType, in every
// Project, "widget_type:<id>/project:<id>/scene:<id>" for one Scene.
const (
	widgetTypeKeyPrefix = "widget_type:"
	sceneKeySeparator   = "/scene:"
)

func (p widgetReconciliationPolicy) JobKey(e event.Event) (string, bool) {
	switch e.Type() {
	case event.EventTypeWidgetTypeUpdated:
		if ev, ok := e.(library.WidgetTypeEvent); ok {
			return widgetTypeKeyPrefix + ev.WidgetTypeID().String(), true
		}
	case event.EventTypeWidgetCreated, event.EventTypeWidgetUpdated:
		if ev, ok := e.(scene.WidgetUpdatedEvent); ok && ev.ByReconciliation() {
			return "", false
		}
		if ev, ok := e.(scene.WidgetTypedEvent); ok {
			return widgetTypeKeyPrefix + ev.TypeID().String() + "/" +
				projectKeyPrefix + ev.ProjectID().String() + sceneKeySeparator + ev.SceneID().String(), true
		}
	}
	return "", false
}

// Run reconciles the Scenes of the job key, each in its own transaction, with
// the WidgetType as it is now: every Scene holding Widgets of the WidgetType,
// or the one Scene the key names. A Scene that fails does not stop the
// others: Run returns the errors of all that failed, and the job is run
// again, the Scenes already reconciled then left as they are. A deleted
// WidgetType or Scene has no Widgets left to reconcile.
func (p widgetReconciliationPolicy) Run(ctx context.Context, key string) error {
	widgetTypeID, one, err := parseJobKey(key)
	if err != nil {
		return err
	}

	if one != nil {
		scope := p.scopes.ForProject(one.project)
		sc, err := scope.Scenes().FindByID(ctx, one.scene)
		if errors.Is(err, scene.ErrSceneNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot find scene %s: %w", one.scene, err)
		}
		return p.reconcileScenes(ctx, widgetTypeID, one.project, scope, []scene.Scene{sc})
	}

	projects, err := p.scopes.FindProjectsUsingWidgetType(ctx, widgetTypeID)
	if err != nil {
		return fmt.Errorf("cannot list projects with widgets of type %s: %w", widgetTypeID, err)
	}
	var errs []error
	for _, projectID := range projects {
		scope := p.scopes.ForProject(projectID)
		scenes, err := scope.Scenes().FindByWidgetTypeID(ctx, widgetTypeID)
		if err == nil {
			err = p.reconcileScenes(ctx, widgetTypeID, projectID, scope, scenes)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("project %s: %w", projectID, err))
		}
	}
	return errors.Join(errs...)
}

// reconcileScenes reconciles scenes, Scenes of the Project projectID read
// through scope, with the WidgetType widgetTypeID.
func (p widgetReconciliationPolicy) reconcileScenes(ctx context.Context, widgetTypeID id.ID[library.WidgetType], projectID id.ID[project.Project], scope ProjectScope, scenes []scene.Scene) error {
	if err := changeEachScene(ctx, scope.Scenes(), scenes, p.reconcile(widgetTypeID, projectID, scope.Tags())); err != nil {
		return fmt.Errorf("cannot reconcile with widget type %s: %w", widgetTypeID, err)
	}
	return nil
}

// sceneOfProject is the one Scene, of its Project, a job key may name.
type sceneOfProject struct {
	project id.ID[project.Project]
	scene   id.ID[scene.Scene]
}

// parseJobKey returns the WidgetType of a job key and its Scene, nil for
// every Scene of the WidgetType.
func parseJobKey(key string) (id.ID[library.WidgetType], *sceneOfProject, error) {
	rest, ok := strings.CutPrefix(key, widgetTypeKeyPrefix)
	if !ok {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid job key %q", key)
	}
	rawTypeID, projectKey, oneScene := strings.Cut(rest, "/")
	typeUUID, err := uuid.Parse(rawTypeID)
	if err != nil {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid widget type ID in job key %q: %w", key, err)
	}
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](typeUUID))
	if !oneScene {
		return widgetTypeID, nil, nil
	}
	projectID, rest, err := cutProjectKey(projectKey)
	if err != nil {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid job key %q: %w", key, err)
	}
	rawSceneID, ok := strings.CutPrefix(rest, sceneKeySeparator)
	if !ok {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid job key %q", key)
	}
	sceneUUID, err := uuid.Parse(rawSceneID)
	if err != nil {
		return id.ID[library.WidgetType]{}, nil, fmt.Errorf("invalid scene ID in job key %q: %w", key, err)
	}
	return widgetTypeID, &sceneOfProject{project: projectID, scene: id.NewID(id.IDWithUUID[scene.Scene](sceneUUID))}, nil
}

// reconcile returns the change reconciling a Scene of the Project projectID
// with the WidgetType widgetTypeID and the Tags its Widgets of that type
// bind, read through tags, the Project's repository. The WidgetType and
// the Tags are read after the Scene, at every attempt of changeScene, so
// every Widget the Scene holds was checked against them or older versions,
// and no binding valid now is taken for an orphan. A deleted WidgetType
// leaves nothing to clean: its Widgets are gone with it.
func (p widgetReconciliationPolicy) reconcile(widgetTypeID id.ID[library.WidgetType], projectID id.ID[project.Project], tags tag.TagRepository) sceneChange {
	return func(ctx context.Context, sc scene.Scene) (scene.Scene, bool, error) {
		wt, err := p.widgetTypes.FindByID(ctx, widgetTypeID)
		if errors.Is(err, library.ErrWidgetTypeNotFound) {
			return sc, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf("cannot read widget type: %w", err)
		}
		tagTypes, err := loadTagTypes(ctx, projectID, tags, sc.TagsBoundByType(widgetTypeID))
		if err != nil {
			return nil, false, err
		}
		reconciled, changed := sc.ReconcileWith(wt, tagTypes)
		return reconciled, changed, nil
	}
}
