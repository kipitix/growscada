package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// WidgetTypeService is the service interface for working with the
// WidgetTypes of a library's Draft. Every method takes the library first; a
// library that does not exist (or is deleted) is an error wrapping
// library.ErrLibraryNotFound. A write while a Discard of the library is
// under way is refused (library.ErrLibraryDiscarding, ADR 0012).
type WidgetTypeService interface {
	FindAllWidgetTypes(ctx context.Context, libraryID uuid.UUID) ([]appdto.WidgetType, error)
	FindWidgetTypeByID(ctx context.Context, libraryID, widgetTypeID uuid.UUID) (appdto.WidgetType, error)
	CreateWidgetType(ctx context.Context, libraryID uuid.UUID, input appdto.WidgetTypeInput) (appdto.WidgetType, error)
	UpdateWidgetType(ctx context.Context, libraryID, widgetTypeID uuid.UUID, version int, input appdto.WidgetTypeInput) (appdto.WidgetType, error)
	DeleteWidgetTypeByID(ctx context.Context, libraryID, widgetTypeID uuid.UUID) (appdto.WidgetType, error)
}

type widgetTypeServiceImpl struct {
	libraries    library.LibraryRepository
	widgetTypes  library.WidgetTypeRepositories
	organization id.ID[organization.Organization]
}

var _ WidgetTypeService = (*widgetTypeServiceImpl)(nil)

// NewWidgetTypeService returns the service of the WidgetTypes of the Drafts
// of the libraries of the Organization anOrganizationID. The events of its
// changes are recorded by the aggregates and saved with them (ADR 0008).
// A change of a WidgetType touches no Scene: Projects see only the releases
// they pin (ADR 0007).
func NewWidgetTypeService(someLibraries library.LibraryRepository, someWidgetTypes library.WidgetTypeRepositories, anOrganizationID id.ID[organization.Organization]) WidgetTypeService {
	return &widgetTypeServiceImpl{libraries: someLibraries, widgetTypes: someWidgetTypes, organization: anOrganizationID}
}

// draft returns the ID and the WidgetType repository of the live library
// rawLibraryID.
func (s widgetTypeServiceImpl) draft(ctx context.Context, rawLibraryID uuid.UUID) (id.ID[library.Library], library.WidgetTypeRepository, error) {
	libraryID := id.NewID(id.IDWithUUID[library.Library](rawLibraryID))
	if _, err := findOwnLibrary(ctx, s.libraries, s.organization, libraryID); err != nil {
		return libraryID, nil, err
	}
	return libraryID, s.widgetTypes.ForLibrary(libraryID), nil
}

func (s widgetTypeServiceImpl) FindAllWidgetTypes(ctx context.Context, rawLibraryID uuid.UUID) ([]appdto.WidgetType, error) {
	_, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return nil, err
	}
	list, err := repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find widget types in repository: %w", err)
	}
	return appdto.NewWidgetTypeList(list), nil
}

func (s widgetTypeServiceImpl) FindWidgetTypeByID(ctx context.Context, rawLibraryID, rawID uuid.UUID) (appdto.WidgetType, error) {
	_, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	wt, err := repo.FindByID(ctx, id.NewID(id.IDWithUUID[library.WidgetType](rawID)))
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}
	return appdto.NewWidgetType(wt), nil
}

func (s widgetTypeServiceImpl) CreateWidgetType(ctx context.Context, rawLibraryID uuid.UUID, input appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	libraryID, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	newID := repo.NextID()

	def, err := parseWidgetTypeDefinition(input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", invalidInput(err))
	}
	newWt, err := library.CreateWidgetType(newID, libraryID, def.name, def.html, def.script, def.lang, def.defaultSize, def.inputPorts)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", invalidInput(err))
	}

	newWt, err = repo.Save(ctx, newWt)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	return appdto.NewWidgetType(newWt), nil
}

func (s widgetTypeServiceImpl) UpdateWidgetType(ctx context.Context, rawLibraryID, rawID uuid.UUID, expectedVersion int, input appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	_, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))

	found, err := repo.FindByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}

	// A negative version is no version at all: the client's input, not a conflict.
	expected, err := version.New(version.WithNumber[library.WidgetType](expectedVersion))
	if err != nil {
		return appdto.WidgetType{}, invalidInput(fmt.Errorf("version: %w", err))
	}
	// A stale write is a conflict whatever it sends: the version is checked
	// before the definition is parsed (Update checks it again, as its own rule).
	if err := found.CheckVersion(expected); err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", err)
	}

	def, err := parseWidgetTypeDefinition(input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", invalidInput(err))
	}
	updated, err := found.Update(expected, def.name, def.html, def.script, def.lang, def.defaultSize, def.inputPorts)
	if errors.Is(err, library.ErrInvalidWidgetType) {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", invalidInput(err))
	}
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", err)
	}

	found, err = repo.Save(ctx, updated)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}

	return appdto.NewWidgetType(found), nil
}

// widgetTypeDefinition is a widget type's definition as the client sent it,
// each field checked on its own; the WidgetType checks them together (e.g.
// port names are unique).
type widgetTypeDefinition struct {
	name        library.WidgetTypeName
	html        library.HtmlTemplate
	script      library.Script
	lang        library.ScriptLanguage
	defaultSize library.Size
	inputPorts  []library.InputPort
}

// parseWidgetTypeDefinition parses the client's fields; each error names the
// field it comes from. The caller marks the error as invalid input.
func parseWidgetTypeDefinition(input appdto.WidgetTypeInput) (widgetTypeDefinition, error) {
	var def widgetTypeDefinition
	var err error
	if def.name, err = library.NewWidgetTypeName(input.Name); err != nil {
		return def, fmt.Errorf("name: %w", err)
	}
	if def.html, err = library.NewHtmlTemplate(input.HtmlTemplate); err != nil {
		return def, fmt.Errorf("html_template: %w", err)
	}
	if def.script, err = library.NewScript(input.Script); err != nil {
		return def, fmt.Errorf("script: %w", err)
	}
	if def.lang, err = library.NewScriptLanguage(input.ScriptLanguage); err != nil {
		return def, fmt.Errorf("script_language: %w", err)
	}
	if def.defaultSize, err = library.NewSize(input.DefaultWidth, input.DefaultHeight); err != nil {
		return def, fmt.Errorf("default_width, default_height: %w", err)
	}
	if def.inputPorts, err = dtoInputPortsToDomain(input.InputPorts); err != nil {
		return def, fmt.Errorf("input_ports: %w", err)
	}
	return def, nil
}

// DeleteWidgetTypeByID removes a widget type from the library's Draft. It
// touches no Scene: the Widgets of Projects are of the copies the releases
// they pin hold. It expects no Version: a widget type changed meanwhile is
// reread and deleted as it is now (retryOnRace).
func (s widgetTypeServiceImpl) DeleteWidgetTypeByID(ctx context.Context, rawLibraryID, rawID uuid.UUID) (appdto.WidgetType, error) {
	_, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))

	var deleted library.WidgetType
	err = retryOnRace(library.ErrWidgetTypeConflict, func() error {
		found, err := repo.FindByID(ctx, widgetTypeID)
		if err != nil {
			return fmt.Errorf("error on find widget type by id in repository: %w", err)
		}
		if err := repo.Delete(ctx, found.Delete()); err != nil {
			return fmt.Errorf("cannot delete widget type: %w", err)
		}
		deleted = found
		return nil
	})
	if err != nil {
		return appdto.WidgetType{}, err
	}

	return appdto.NewWidgetType(deleted), nil
}

// dtoInputPortsToDomain converts appdto.InputPort slice to domain InputPort slice.
func dtoInputPortsToDomain(dtos []appdto.InputPort) ([]library.InputPort, error) {
	ports := make([]library.InputPort, 0, len(dtos))
	for _, dto := range dtos {
		name, err := library.NewInputPortName(dto.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid input port name %q: %w", dto.Name, err)
		}
		typeHint, err := library.NewPortTypeHint(dto.TypeHint)
		if err != nil {
			return nil, fmt.Errorf("invalid input port type hint %q: %w", dto.TypeHint, err)
		}
		ports = append(ports, library.NewInputPort(name, dto.Description, typeHint))
	}
	return ports, nil
}
