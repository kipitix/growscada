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
)

// WidgetTypeService is the service interface for working with the
// WidgetTypes of a library's Draft. Every method takes the library first; a
// library that does not exist (or is deleted) is an error wrapping
// library.ErrLibraryNotFound. Every write states the Version of the Library's
// Draft (ADR 0016): a stale one is draft.ErrDraftConflict.
type WidgetTypeService interface {
	FindAllWidgetTypes(ctx context.Context, libraryID uuid.UUID) ([]appdto.WidgetType, error)
	FindWidgetTypeByID(ctx context.Context, libraryID, widgetTypeID uuid.UUID) (appdto.WidgetType, error)
	CreateWidgetType(ctx context.Context, libraryID uuid.UUID, version int, input appdto.WidgetTypeInput) (appdto.WidgetType, error)
	UpdateWidgetType(ctx context.Context, libraryID, widgetTypeID uuid.UUID, version int, input appdto.WidgetTypeInput) (appdto.WidgetType, error)
	DeleteWidgetTypeByID(ctx context.Context, libraryID, widgetTypeID uuid.UUID, version int) (appdto.WidgetType, error)
}

type widgetTypeServiceImpl struct {
	content     LibraryContent
	widgetTypes library.WidgetTypeRepositories
}

var _ WidgetTypeService = (*widgetTypeServiceImpl)(nil)

// NewWidgetTypeService returns the service of the WidgetTypes of the Drafts
// of the libraries of the Organization the content serves. The writes go
// through the Draft's repository, which records one DraftChange each
// (ADR 0017); a change of a WidgetType touches no Scene: Projects see only the
// releases they pin (ADR 0007).
func NewWidgetTypeService(aContent LibraryContent, someWidgetTypes library.WidgetTypeRepositories) WidgetTypeService {
	return &widgetTypeServiceImpl{content: aContent, widgetTypes: someWidgetTypes}
}

// draft returns the ID and the read repository of the WidgetTypes of the live
// library rawLibraryID.
func (s widgetTypeServiceImpl) draft(ctx context.Context, rawLibraryID uuid.UUID) (id.ID[library.Library], library.WidgetTypeRepository, error) {
	libraryID, err := s.content.open(ctx, rawLibraryID)
	if err != nil {
		return libraryID, nil, err
	}
	return libraryID, s.widgetTypes.ForLibrary(libraryID), nil
}

func (s widgetTypeServiceImpl) FindAllWidgetTypes(ctx context.Context, rawLibraryID uuid.UUID) ([]appdto.WidgetType, error) {
	libraryID, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return nil, err
	}
	list, err := repo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on find widget types in repository: %w", err)
	}
	version, err := s.content.Version(ctx, libraryID.UUID())
	if err != nil {
		return nil, err
	}
	return appdto.NewWidgetTypeList(list, version), nil
}

func (s widgetTypeServiceImpl) FindWidgetTypeByID(ctx context.Context, rawLibraryID, rawID uuid.UUID) (appdto.WidgetType, error) {
	libraryID, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	wt, err := repo.FindByID(ctx, id.NewID(id.IDWithUUID[library.WidgetType](rawID)))
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}
	version, err := s.content.Version(ctx, libraryID.UUID())
	if err != nil {
		return appdto.WidgetType{}, err
	}
	return appdto.NewWidgetType(wt, version), nil
}

func (s widgetTypeServiceImpl) CreateWidgetType(ctx context.Context, rawLibraryID uuid.UUID, rawVersion int, input appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	libraryID, _, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	def, err := parseWidgetTypeDefinition(input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", invalidInput(err))
	}
	newID := id.NewID[library.WidgetType]()
	newWt, err := library.CreateWidgetType(newID, libraryID, def.name, def.html, def.script, def.lang, def.defaultSize, def.inputPorts)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot create widget type: %w", invalidInput(err))
	}

	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(libraryID), expected,
		"Create widget type "+def.name.String(),
		func(session *draft.Session) error {
			session.CreateWidgetType(ctx, newWt)
			return nil
		})
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot save widget type: %w", err)
	}
	return appdto.NewWidgetType(newWt, result.Version().Number()), nil
}

func (s widgetTypeServiceImpl) UpdateWidgetType(ctx context.Context, rawLibraryID, rawID uuid.UUID, rawVersion int, input appdto.WidgetTypeInput) (appdto.WidgetType, error) {
	libraryID, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))
	// A missing widget type is not found whatever the body says; the write
	// then loads it again in its own transaction.
	if _, err := repo.FindByID(ctx, widgetTypeID); err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	def, err := parseWidgetTypeDefinition(input)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", invalidInput(err))
	}

	var updated library.WidgetType
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(libraryID), expected,
		"Update widget type "+def.name.String(),
		func(session *draft.Session) error {
			var err error
			updated, err = session.UpdateWidgetType(ctx, widgetTypeID, def.name, def.html, def.script, def.lang, def.defaultSize, def.inputPorts)
			return err
		})
	if errors.Is(err, library.ErrInvalidWidgetType) {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", invalidInput(err))
	}
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot update widget type: %w", err)
	}
	return appdto.NewWidgetType(updated, result.Version().Number()), nil
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
// they pin hold. The write states the Version of the Draft the client saw.
func (s widgetTypeServiceImpl) DeleteWidgetTypeByID(ctx context.Context, rawLibraryID, rawID uuid.UUID, rawVersion int) (appdto.WidgetType, error) {
	libraryID, repo, err := s.draft(ctx, rawLibraryID)
	if err != nil {
		return appdto.WidgetType{}, err
	}
	widgetTypeID := id.NewID(id.IDWithUUID[library.WidgetType](rawID))
	found, err := repo.FindByID(ctx, widgetTypeID)
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("error on find widget type by id in repository: %w", err)
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.WidgetType{}, err
	}

	var deleted library.WidgetType
	result, err := s.content.Drafts().Update(ctx, s.content.ScopeOf(libraryID), expected,
		"Delete widget type "+found.Name().String(),
		func(session *draft.Session) error {
			var err error
			deleted, err = session.DeleteWidgetType(ctx, widgetTypeID)
			return err
		})
	if err != nil {
		return appdto.WidgetType{}, fmt.Errorf("cannot delete widget type: %w", err)
	}
	return appdto.NewWidgetType(deleted, result.Version().Number()), nil
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
