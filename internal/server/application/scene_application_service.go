package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// SceneService is the service interface for working with the scenes of a
// project and their widgets. Widget, as an entity of the Scene, has no
// service of its own: every widget operation is reached through a scene.
// Every write states the Version of the Project's Draft (ADR 0016). Every
// method takes the project first; a project that does not exist (or is
// deleted) is an error wrapping project.ErrProjectNotFound.
type SceneService interface {
	FindAllScenes(ctx context.Context, projectID uuid.UUID) ([]appdto.Scene, error)
	FindSceneByID(ctx context.Context, projectID, sceneID uuid.UUID) (appdto.Scene, error)
	CreateScene(ctx context.Context, projectID uuid.UUID, version int, input appdto.SceneInput) (appdto.Scene, error)
	UpdateScene(ctx context.Context, projectID, sceneID uuid.UUID, version int, input appdto.SceneInput) (appdto.Scene, error)
	DeleteSceneByID(ctx context.Context, projectID, sceneID uuid.UUID, version int) (appdto.Scene, error)

	FindWidgetsBySceneID(ctx context.Context, projectID, sceneID uuid.UUID) ([]appdto.Widget, error)
	FindWidgetByID(ctx context.Context, projectID, sceneID, widgetID uuid.UUID) (appdto.Widget, error)
	CreateWidget(ctx context.Context, projectID, sceneID uuid.UUID, version int, input appdto.WidgetInput) (appdto.Widget, error)
	UpdateWidget(ctx context.Context, projectID, sceneID, widgetID uuid.UUID, version int, input appdto.WidgetInput) (appdto.Widget, error)
	MoveWidgets(ctx context.Context, projectID, sceneID uuid.UUID, version int, moves []appdto.WidgetMove) (int, error)
	DeleteWidgetByID(ctx context.Context, projectID, sceneID, widgetID uuid.UUID, version int) (appdto.Widget, error)
}

type sceneServiceImpl struct {
	content ProjectContent
}

var _ SceneService = (*sceneServiceImpl)(nil)

// NewSceneService returns the scene service. The events of its changes are
// recorded by the Scene and saved with its Draft (ADR 0008, ADR 0016). A
// Widget's WidgetType is the one a Library Release its Project pins holds
// (ADR 0007).
func NewSceneService(aContent ProjectContent) SceneService {
	return &sceneServiceImpl{content: aContent}
}

// scenes returns the scene read repository of the live project rawProjectID.
func (s sceneServiceImpl) scenes(ctx context.Context, rawProjectID uuid.UUID) (scene.SceneRepository, error) {
	_, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return scope.Scenes(), nil
}

// loadScene finds the scene named by the caller; ErrSceneNotFound if there is none.
func loadScene(ctx context.Context, repo scene.SceneRepository, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	found, err := repo.FindByID(ctx, sceneID)
	if err != nil {
		return nil, fmt.Errorf("error on find scene by id in repository: %w", err)
	}
	return found, nil
}

// widgetChangeError marks a widget the Scene rejected as invalid input when
// the rejection is about what the caller sent: the ports it binds and the
// Tags it binds them to.
func widgetChangeError(err error) error {
	if errors.Is(err, scene.ErrPortNotDeclared) ||
		errors.Is(err, scene.ErrBoundTagNotFound) ||
		errors.Is(err, scene.ErrPortTypeMismatch) {
		return fmt.Errorf("port_bindings: %w", invalidInput(err))
	}
	return err
}

// widgetSaveError marks a failed write of a scene with a new or changed
// widget as invalid input when the widget's type is not in a release the
// Project pins: the Draft's transaction checks the pin while it writes
// (ADR 0012).
func widgetSaveError(err error) error {
	if errors.Is(err, library.ErrWidgetTypeNotFound) {
		return fmt.Errorf("cannot save scene: type_id: %w", invalidInput(err))
	}
	return fmt.Errorf("cannot save scene: %w", err)
}

// FindAllScenes returns the scenes of the project. The UI reloads them on
// every event: they are read with the check of their project, and the
// project is read apart only when there are none, to tell a missing project
// from an empty one.
func (s sceneServiceImpl) FindAllScenes(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.Scene, error) {
	_, scope := s.content.openUnchecked(rawProjectID)
	list, err := scope.Scenes().FindAllInLiveProject(ctx, s.content.organization)
	if err != nil {
		return nil, fmt.Errorf("error on find scenes in repository: %w", err)
	}
	if len(list) == 0 {
		if _, _, err := s.content.open(ctx, rawProjectID); err != nil {
			return nil, err
		}
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return appdto.NewSceneList(list, version), nil
}

func (s sceneServiceImpl) FindSceneByID(ctx context.Context, rawProjectID, rawID uuid.UUID) (appdto.Scene, error) {
	repo, err := s.scenes(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawID))
	found, err := repo.FindByID(ctx, sceneID)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("error on find scene by id in repository: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	return appdto.NewScene(found, version), nil
}

// CreateScene creates a new scene in the project, stating the Version of the
// Draft the client saw.
func (s sceneServiceImpl) CreateScene(ctx context.Context, rawProjectID uuid.UUID, rawVersion int, input appdto.SceneInput) (appdto.Scene, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Scene{}, err
	}
	name, size, background, err := parseSceneFields(input)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot create scene: %w", invalidInput(err))
	}
	newScene := scene.CreateScene(id.NewID[scene.Scene](), projectID, name, size, background)
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Create scene "+name.String(),
		func(session *draft.Session) error {
			session.CreateScene(ctx, newScene)
			return nil
		})
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot save scene: %w", err)
	}
	return appdto.NewScene(newScene, result.Version().Number()), nil
}

func (s sceneServiceImpl) UpdateScene(ctx context.Context, rawProjectID, rawID uuid.UUID, rawVersion int, input appdto.SceneInput) (appdto.Scene, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Scene{}, err
	}
	name, size, background, err := parseSceneFields(input)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot update scene: %w", invalidInput(err))
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawID))
	var updated scene.Scene
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Update scene "+name.String(),
		func(session *draft.Session) error {
			var err error
			updated, err = session.UpdateScene(ctx, sceneID, name, size, background)
			return err
		})
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot update scene: %w", err)
	}
	return appdto.NewScene(updated, result.Version().Number()), nil
}

// parseSceneFields parses the scene's own fields from the client; each error
// names the field it comes from. The caller marks the error as invalid input.
func parseSceneFields(input appdto.SceneInput) (scene.SceneName, scene.SceneSize, scene.BackgroundHTML, error) {
	name, err := scene.NewSceneName(input.Name)
	if err != nil {
		return scene.SceneName{}, scene.SceneSize{}, scene.BackgroundHTML{}, fmt.Errorf("name: %w", err)
	}
	size, err := scene.NewSceneSize(input.Width, input.Height)
	if err != nil {
		return scene.SceneName{}, scene.SceneSize{}, scene.BackgroundHTML{}, fmt.Errorf("width, height: %w", err)
	}
	return name, size, scene.NewBackgroundHTML(input.BackgroundHTML), nil
}

// DeleteSceneByID deletes a scene together with all of its widgets, in one
// DraftChange that records every Widget before it goes (ADR 0017).
func (s sceneServiceImpl) DeleteSceneByID(ctx context.Context, rawProjectID, rawID uuid.UUID, rawVersion int) (appdto.Scene, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Scene{}, err
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawID))
	var deleted scene.Scene
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Delete scene",
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteScene(ctx, sceneID)
			return err
		})
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot delete scene: %w", err)
	}
	return appdto.NewScene(deleted, result.Version().Number()), nil
}

// FindWidgetsBySceneID returns the widgets of a scene. The UI reloads them
// on every event of the scene: it is read with the check of its project,
// and the project is read apart only when the scene is not found, to tell a
// missing project from a missing scene.
func (s sceneServiceImpl) FindWidgetsBySceneID(ctx context.Context, rawProjectID, rawSceneID uuid.UUID) ([]appdto.Widget, error) {
	_, scope := s.content.openUnchecked(rawProjectID)
	sc, err := scope.Scenes().FindByIDInLiveProject(ctx, id.NewID(id.IDWithUUID[scene.Scene](rawSceneID)), s.content.organization)
	if errors.Is(err, scene.ErrSceneNotFound) {
		if _, _, projectErr := s.content.open(ctx, rawProjectID); projectErr != nil {
			return nil, projectErr
		}
	}
	if err != nil {
		return nil, fmt.Errorf("error on find scene by id in repository: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	return appdto.NewWidgetList(sc.Widgets(), rawSceneID, version), nil
}

func (s sceneServiceImpl) FindWidgetByID(ctx context.Context, rawProjectID, rawSceneID, rawWidgetID uuid.UUID) (appdto.Widget, error) {
	repo, err := s.scenes(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	sc, err := loadScene(ctx, repo, id.NewID(id.IDWithUUID[scene.Scene](rawSceneID)))
	if err != nil {
		return appdto.Widget{}, err
	}
	w, err := sc.FindWidget(id.NewID(id.IDWithUUID[scene.Widget](rawWidgetID)))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("error on find widget by id: %w", err)
	}
	version, err := s.content.Version(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	return appdto.NewWidget(w, rawSceneID, version), nil
}

func (s sceneServiceImpl) CreateWidget(ctx context.Context, rawProjectID, rawSceneID uuid.UUID, rawVersion int, input appdto.WidgetInput) (appdto.Widget, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Widget{}, err
	}
	newWidget, err := parseWidget(id.NewID[scene.Widget](), input)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget: %w", invalidInput(err))
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Create widget "+newWidget.Name().String(),
		func(session *draft.Session) error {
			_, err := session.AddWidget(ctx, sceneID, newWidget)
			return err
		})
	if err != nil {
		return appdto.Widget{}, widgetSaveError(widgetChangeError(err))
	}
	return appdto.NewWidget(newWidget, rawSceneID, result.Version().Number()), nil
}

func (s sceneServiceImpl) UpdateWidget(ctx context.Context, rawProjectID, rawSceneID, rawWidgetID uuid.UUID, rawVersion int, input appdto.WidgetInput) (appdto.Widget, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Widget{}, err
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	widgetID := id.NewID(id.IDWithUUID[scene.Widget](rawWidgetID))
	updated, err := parseWidget(widgetID, input)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot update widget: %w", invalidInput(err))
	}
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Update widget "+updated.Name().String(),
		func(session *draft.Session) error {
			_, err := session.UpdateWidget(ctx, sceneID, updated)
			return err
		})
	if err != nil {
		return appdto.Widget{}, widgetSaveError(widgetChangeError(err))
	}
	return appdto.NewWidget(updated, rawSceneID, result.Version().Number()), nil
}

// MoveWidgets moves several widgets of one scene in one DraftChange: only the
// position's x and y of each named widget change (ADR 0010, ADR 0017). The
// answer is the Version of the Draft after the change.
func (s sceneServiceImpl) MoveWidgets(ctx context.Context, rawProjectID, rawSceneID uuid.UUID, rawVersion int, moves []appdto.WidgetMove) (int, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return 0, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return 0, err
	}
	domainMoves, err := parseWidgetMoves(moves)
	if err != nil {
		return 0, fmt.Errorf("cannot move widgets: %w", invalidInput(err))
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected,
		fmt.Sprintf("Move %d widgets", len(moves)),
		func(session *draft.Session) error {
			_, err := session.MoveWidgets(ctx, sceneID, domainMoves)
			return err
		})
	if err != nil {
		if errors.Is(err, scene.ErrWidgetMovedTwice) {
			return 0, fmt.Errorf("cannot move widgets: %w", invalidInput(err))
		}
		return 0, widgetSaveError(err)
	}
	return result.Version().Number(), nil
}

// parseWidgetMoves builds the domain's moves from the client's; each error
// names the field it comes from. The caller marks the error as invalid input.
// A repeated widget_id is refused here, before the Draft is read; the domain
// refuses it too, for callers that do not come through the service.
func parseWidgetMoves(moves []appdto.WidgetMove) ([]scene.WidgetMove, error) {
	if len(moves) == 0 {
		return nil, errors.New("moves: is required")
	}
	seen := make(map[uuid.UUID]struct{}, len(moves))
	parsed := make([]scene.WidgetMove, len(moves))
	for i, m := range moves {
		if m.WidgetID == uuid.Nil {
			return nil, fmt.Errorf("moves[%d].widget_id: is required", i)
		}
		if _, twice := seen[m.WidgetID]; twice {
			return nil, fmt.Errorf("moves[%d].widget_id: is repeated", i)
		}
		seen[m.WidgetID] = struct{}{}
		parsed[i] = scene.NewWidgetMove(id.NewID(id.IDWithUUID[scene.Widget](m.WidgetID)), m.X, m.Y)
	}
	return parsed, nil
}

// parseWidget builds a widget from the client's fields; each error names the
// field it comes from. The caller marks the error as invalid input.
func parseWidget(widgetID id.ID[scene.Widget], input appdto.WidgetInput) (scene.Widget, error) {
	if input.TypeID == uuid.Nil {
		return nil, errors.New("type_id: is required")
	}
	name, err := scene.NewWidgetName(input.Name)
	if err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}
	size, err := library.NewSize(input.Width, input.Height)
	if err != nil {
		return nil, fmt.Errorf("size: %w", err)
	}
	origin, err := scene.NewOrigin(input.OriginX, input.OriginY)
	if err != nil {
		return nil, fmt.Errorf("origin: %w", err)
	}
	portBindings, err := dtoPortBindingsToDomain(input.PortBindings)
	if err != nil {
		return nil, fmt.Errorf("port_bindings: %w", err)
	}
	return scene.NewWidget(
		widgetID, name,
		scene.NewPosition(input.X, input.Y, input.Z),
		size, origin,
		scene.NewRotation(input.RotationDegrees),
		id.NewID(id.IDWithUUID[library.WidgetType](input.TypeID)),
		input.Labels, portBindings,
	), nil
}

// DeleteWidgetByID removes a widget from its scene, stating the Version of
// the Draft the client saw.
func (s sceneServiceImpl) DeleteWidgetByID(ctx context.Context, rawProjectID, rawSceneID, rawWidgetID uuid.UUID, rawVersion int) (appdto.Widget, error) {
	projectID, _, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.Widget{}, err
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	widgetID := id.NewID(id.IDWithUUID[scene.Widget](rawWidgetID))
	var deleted scene.Widget
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(projectID), expected, "Delete widget",
		func(session *draft.Session) error {
			var err error
			deleted, err = session.RemoveWidget(ctx, sceneID, widgetID)
			return err
		})
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot delete widget: %w", err)
	}
	return appdto.NewWidget(deleted, rawSceneID, result.Version().Number()), nil
}

// dtoPortBindingsToDomain converts appdto.PortBinding slice to domain PortBinding slice.
func dtoPortBindingsToDomain(dtos []appdto.PortBinding) ([]scene.PortBinding, error) {
	bindings := make([]scene.PortBinding, 0, len(dtos))
	for _, dto := range dtos {
		portName, err := library.NewInputPortName(dto.PortName)
		if err != nil {
			return nil, fmt.Errorf("invalid port binding name %q: %w", dto.PortName, err)
		}
		tagID := id.NewID(id.IDWithUUID[tag.Tag](dto.TagID))
		bindings = append(bindings, scene.NewPortBinding(portName, tagID))
	}
	return bindings, nil
}
