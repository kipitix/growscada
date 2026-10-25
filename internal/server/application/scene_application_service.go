package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/scene"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// SceneService is the service interface for working with the scenes of a
// project and their widgets. Widget, as an entity of the Scene aggregate, has
// no service of its own: every widget operation is reached through a scene.
// Every method takes the project first; a project that does not exist (or is
// deleted) is an error wrapping project.ErrProjectNotFound.
type SceneService interface {
	FindAllScenes(ctx context.Context, projectID uuid.UUID) ([]appdto.Scene, error)
	FindSceneByID(ctx context.Context, projectID, sceneID uuid.UUID) (appdto.Scene, error)
	CreateScene(ctx context.Context, projectID uuid.UUID, input appdto.SceneInput) (appdto.Scene, error)
	UpdateScene(ctx context.Context, projectID, sceneID uuid.UUID, version int, input appdto.SceneInput) (appdto.Scene, error)
	DeleteSceneByID(ctx context.Context, projectID, sceneID uuid.UUID) (appdto.Scene, error)

	FindWidgetsBySceneID(ctx context.Context, projectID, sceneID uuid.UUID) ([]appdto.Widget, error)
	FindWidgetByID(ctx context.Context, projectID, sceneID, widgetID uuid.UUID) (appdto.Widget, error)
	CreateWidget(ctx context.Context, projectID, sceneID uuid.UUID, sceneVersion int, input appdto.WidgetInput) (appdto.Widget, error)
	UpdateWidget(ctx context.Context, projectID, sceneID, widgetID uuid.UUID, sceneVersion int, input appdto.WidgetInput) (appdto.Widget, error)
	DeleteWidgetByID(ctx context.Context, projectID, sceneID, widgetID uuid.UUID) (appdto.Widget, error)
}

type sceneServiceImpl struct {
	content              ProjectContent
	widgetTypeRepository library.WidgetTypeRepository
}

var _ SceneService = (*sceneServiceImpl)(nil)

// NewSceneService returns the scene service. The events of its changes are
// recorded by the Scene and saved with it (ADR 0008).
func NewSceneService(aContent ProjectContent, widgetTypeRepo library.WidgetTypeRepository) SceneService {
	return &sceneServiceImpl{content: aContent, widgetTypeRepository: widgetTypeRepo}
}

// scenes returns the scene repository of the live project rawProjectID.
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

// loadWidgetType finds the WidgetType a widget from the caller names; a
// missing one is invalid input.
func (s sceneServiceImpl) loadWidgetType(ctx context.Context, typeID id.ID[library.WidgetType]) (library.WidgetType, error) {
	wt, err := s.widgetTypeRepository.FindByID(ctx, typeID)
	if errors.Is(err, library.ErrWidgetTypeNotFound) {
		return nil, fmt.Errorf("type_id: %w", invalidInput(err))
	}
	if err != nil {
		return nil, fmt.Errorf("cannot find widget type: %w", err)
	}
	return wt, nil
}

// loadTagTypes reads, in one query, the TagTypes of the Tags of the Project
// projectID with the given IDs, through tags, that Project's repository; a
// Tag that does not exist there is left out, for the Scene to judge the
// binding to it. A Tag deleted after it was read is unbound by policies
// (ADR 0010).
func loadTagTypes(ctx context.Context, projectID id.ID[project.Project], tags tag.TagRepository, tagIDs []id.ID[tag.Tag]) (scene.TagTypes, error) {
	types, err := tags.FindTypesByIDs(ctx, tagIDs)
	if err != nil {
		return scene.TagTypes{}, fmt.Errorf("cannot read tag types: %w", err)
	}
	return scene.NewTagTypes(projectID, types), nil
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

// widgetSaveError marks a failed save of a scene with a new or changed widget
// as invalid input when the widget's type was deleted after loadWidgetType
// found it: the type's foreign key catches it.
func widgetSaveError(err error) error {
	if errors.Is(err, library.ErrWidgetTypeNotFound) {
		return fmt.Errorf("cannot save scene: type_id: %w", invalidInput(err))
	}
	return fmt.Errorf("cannot save scene: %w", err)
}

func (s sceneServiceImpl) FindAllScenes(ctx context.Context, rawProjectID uuid.UUID) ([]appdto.Scene, error) {
	repo, err := s.scenes(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	list, err := repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find scenes in repository: %w", err)
	}
	return appdto.NewSceneList(list), nil
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
	return appdto.NewScene(found), nil
}

// CreateScene creates a new scene in the project. A project deleted just
// after it was read here loses the scene to its cleanup
// (NewProjectCleanupPolicy keys scene_created too).
func (s sceneServiceImpl) CreateScene(ctx context.Context, rawProjectID uuid.UUID, input appdto.SceneInput) (appdto.Scene, error) {
	projectID, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	repo := scope.Scenes()
	newID := repo.NextID()

	name, size, background, err := parseSceneFields(input)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot create scene: %w", invalidInput(err))
	}
	newScene := scene.CreateScene(newID, projectID, name, size, background)

	saved, err := repo.Save(ctx, newScene)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot save scene: %w", err)
	}

	return appdto.NewScene(saved), nil
}

func (s sceneServiceImpl) UpdateScene(ctx context.Context, rawProjectID, rawID uuid.UUID, rawExpectedVersion int, input appdto.SceneInput) (appdto.Scene, error) {
	repo, err := s.scenes(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawID))

	found, err := loadScene(ctx, repo, sceneID)
	if err != nil {
		return appdto.Scene{}, err
	}

	// A negative version is no version at all: the client's input, not a conflict.
	expectedVersion, err := version.New(version.WithNumber[scene.Scene](rawExpectedVersion))
	if err != nil {
		return appdto.Scene{}, invalidInput(fmt.Errorf("version: %w", err))
	}

	name, size, background, err := parseSceneFields(input)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot update scene: %w", invalidInput(err))
	}

	updated, err := found.Update(expectedVersion, name, size, background)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot update scene: %w", err)
	}

	saved, err := repo.Save(ctx, updated)
	if err != nil {
		return appdto.Scene{}, fmt.Errorf("cannot save scene: %w", err)
	}

	return appdto.NewScene(saved), nil
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

// DeleteSceneByID deletes a scene together with all of its widgets (the
// widgets are removed in the DB via ON DELETE CASCADE). The Scene records a
// WidgetDeletedEvent for each widget it holds, and the delete's CAS makes
// them the widgets the cascade removes: a widget added meanwhile makes the
// delete lose the race, and the scene is reread with it (retryOnRace).
func (s sceneServiceImpl) DeleteSceneByID(ctx context.Context, rawProjectID, rawID uuid.UUID) (appdto.Scene, error) {
	repo, err := s.scenes(ctx, rawProjectID)
	if err != nil {
		return appdto.Scene{}, err
	}
	deleted, err := deleteScene(ctx, repo, id.NewID(id.IDWithUUID[scene.Scene](rawID)))
	if err != nil {
		return appdto.Scene{}, err
	}
	return appdto.NewScene(deleted), nil
}

// deleteScene deletes the scene sceneID through repo and returns it as it
// was read (see DeleteSceneByID).
func deleteScene(ctx context.Context, repo scene.SceneRepository, sceneID id.ID[scene.Scene]) (scene.Scene, error) {
	var deleted scene.Scene
	err := retryOnRace(scene.ErrSceneConflict, func() error {
		found, err := loadScene(ctx, repo, sceneID)
		if err != nil {
			return err
		}
		if err := repo.Delete(ctx, found.Delete()); err != nil {
			return fmt.Errorf("cannot delete scene: %w", err)
		}
		deleted = found
		return nil
	})
	return deleted, err
}

func (s sceneServiceImpl) FindWidgetsBySceneID(ctx context.Context, rawProjectID, rawSceneID uuid.UUID) ([]appdto.Widget, error) {
	repo, err := s.scenes(ctx, rawProjectID)
	if err != nil {
		return nil, err
	}
	sc, err := loadScene(ctx, repo, id.NewID(id.IDWithUUID[scene.Scene](rawSceneID)))
	if err != nil {
		return nil, err
	}
	return appdto.NewWidgetList(sc.Widgets(), rawSceneID, sc.Version().Number()), nil
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
	return appdto.NewWidget(w, rawSceneID, sc.Version().Number()), nil
}

func (s sceneServiceImpl) CreateWidget(ctx context.Context, rawProjectID, rawSceneID uuid.UUID, rawSceneVersion int, input appdto.WidgetInput) (appdto.Widget, error) {
	projectID, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	repo := scope.Scenes()
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	newID := repo.NextWidgetID()

	newWidget, expectedVersion, err := parseWidgetWrite(newID, rawSceneVersion, input)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget: %w", err)
	}

	sc, err := loadScene(ctx, repo, sceneID)
	if err != nil {
		return appdto.Widget{}, err
	}
	// A stale write is a conflict whatever type it names.
	if err := sc.CheckVersion(expectedVersion); err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget: %w", err)
	}
	wt, err := s.loadWidgetType(ctx, newWidget.TypeID())
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget: %w", err)
	}
	tagTypes, err := loadTagTypes(ctx, projectID, scope.Tags(), scene.BoundTagIDs(newWidget))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget: %w", err)
	}

	changed, err := sc.AddWidget(expectedVersion, newWidget, wt, tagTypes)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot create widget: %w", widgetChangeError(err))
	}

	saved, err := repo.Save(ctx, changed)
	if err != nil {
		return appdto.Widget{}, widgetSaveError(err)
	}

	return savedWidget(saved, newID, rawSceneID)
}

func (s sceneServiceImpl) UpdateWidget(ctx context.Context, rawProjectID, rawSceneID, rawWidgetID uuid.UUID, rawSceneVersion int, input appdto.WidgetInput) (appdto.Widget, error) {
	projectID, scope, err := s.content.open(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	repo := scope.Scenes()
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	widgetID := id.NewID(id.IDWithUUID[scene.Widget](rawWidgetID))

	updated, expectedVersion, err := parseWidgetWrite(widgetID, rawSceneVersion, input)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot update widget: %w", err)
	}

	sc, err := loadScene(ctx, repo, sceneID)
	if err != nil {
		return appdto.Widget{}, err
	}
	// A stale write is a conflict, and the widget named in the URL is checked
	// before the type named in the body: a missing widget is 404 whatever
	// the body says.
	if err := sc.CheckVersion(expectedVersion); err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot update widget: %w", err)
	}
	if _, err := sc.FindWidget(widgetID); err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot update widget: %w", err)
	}
	wt, err := s.loadWidgetType(ctx, updated.TypeID())
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot update widget: %w", err)
	}
	tagTypes, err := loadTagTypes(ctx, projectID, scope.Tags(), scene.BoundTagIDs(updated))
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot update widget: %w", err)
	}

	changed, err := sc.UpdateWidget(expectedVersion, updated, wt, tagTypes)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("cannot update widget: %w", widgetChangeError(err))
	}

	saved, err := repo.Save(ctx, changed)
	if err != nil {
		return appdto.Widget{}, widgetSaveError(err)
	}

	return savedWidget(saved, widgetID, rawSceneID)
}

// savedWidget returns the widget as the repository stored it in saved, the
// scene Save returned.
func savedWidget(saved scene.Scene, widgetID id.ID[scene.Widget], rawSceneID uuid.UUID) (appdto.Widget, error) {
	w, err := saved.FindWidget(widgetID)
	if err != nil {
		return appdto.Widget{}, fmt.Errorf("saved scene lacks widget %s: %w", widgetID, err)
	}
	return appdto.NewWidget(w, rawSceneID, saved.Version().Number()), nil
}

// parseWidgetWrite checks what CreateWidget and UpdateWidget receive from the
// client: the widget's fields and the scene version the write expects, both
// invalid input on failure.
func parseWidgetWrite(widgetID id.ID[scene.Widget], rawSceneVersion int, input appdto.WidgetInput) (scene.Widget, version.Version[scene.Scene], error) {
	w, err := parseWidget(widgetID, input)
	if err != nil {
		return nil, version.Version[scene.Scene]{}, invalidInput(err)
	}
	expectedVersion, err := version.New(version.WithNumber[scene.Scene](rawSceneVersion))
	if err != nil {
		return nil, version.Version[scene.Scene]{}, invalidInput(fmt.Errorf("scene_version: %w", err))
	}
	return w, expectedVersion, nil
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

// DeleteWidgetByID removes a widget from its scene. Like deleting any
// aggregate it expects no scene version: a scene changed since it was read is
// reread and the widget removed from it as it is now (retryOnRace).
func (s sceneServiceImpl) DeleteWidgetByID(ctx context.Context, rawProjectID, rawSceneID, rawWidgetID uuid.UUID) (appdto.Widget, error) {
	repo, err := s.scenes(ctx, rawProjectID)
	if err != nil {
		return appdto.Widget{}, err
	}
	sceneID := id.NewID(id.IDWithUUID[scene.Scene](rawSceneID))
	widgetID := id.NewID(id.IDWithUUID[scene.Widget](rawWidgetID))

	var deleted scene.Widget
	var saved scene.Scene
	err = retryOnRace(scene.ErrSceneConflict, func() error {
		sc, err := loadScene(ctx, repo, sceneID)
		if err != nil {
			return err
		}
		changed, w, err := sc.RemoveWidget(widgetID)
		if err != nil {
			return fmt.Errorf("cannot delete widget: %w", err)
		}
		if saved, err = repo.Save(ctx, changed); err != nil {
			return fmt.Errorf("cannot save scene: %w", err)
		}
		deleted = w
		return nil
	})
	if err != nil {
		return appdto.Widget{}, err
	}

	return appdto.NewWidget(deleted, rawSceneID, saved.Version().Number()), nil
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
