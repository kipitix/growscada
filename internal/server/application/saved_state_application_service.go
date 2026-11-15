package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/application/appdto"
	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

// SavedStateService is the Engineering node's service of saved states (task
// 72_2; ADR 0021): a Save fixes the whole Draft as an immutable, numbered
// snapshot, the history is listed and read, and a title is renamed without
// touching the snapshot. It serves both a Project's Draft and a Library's,
// named by a draft.Scope. The labels that mark a saved state come from the
// Label repository, read for a whole history at once (task 85_3; ADR 0025).
type SavedStateService interface {
	// Save fixes the Draft of scope at version as a saved state with the title and
	// author, and returns it. Returns ErrNothingToSave if the Draft is at the
	// Version of its last saved state, a DraftNotSaveableError if the Draft cannot
	// be assembled, and draft.ConflictError if version is stale. An empty title or
	// author is invalid input.
	Save(ctx context.Context, scope draft.Scope, version int, title, author string) (appdto.SavedState, error)
	// List returns the Draft's saved states, newest first, without their content;
	// labelID (uuid.Nil: every) keeps only the saved states it marks.
	List(ctx context.Context, scope draft.Scope, labelID uuid.UUID) ([]appdto.SavedState, error)
	// Find returns one saved state; a Library's carries its WidgetTypes.
	Find(ctx context.Context, scope draft.Scope, number int) (appdto.SavedState, error)
	// Rename changes the title of a saved state, stating no Draft Version.
	Rename(ctx context.Context, scope draft.Scope, number int, title string) (appdto.SavedState, error)
	// ProjectFile returns the ProjectFile of a Project's saved state as a
	// download. It is refused with invalid input for a Library's scope.
	ProjectFile(ctx context.Context, scope draft.Scope, number int) (ProjectFileDownload, error)
}

type savedStateServiceImpl struct {
	content     ProjectContent
	libraries   LibraryContent
	builder     ProjectFileBuilder
	widgetTypes library.WidgetTypeRepositories
	labels      LabelService
}

var _ SavedStateService = (*savedStateServiceImpl)(nil)

// NewSavedStateService returns the service of the saved states of the Projects
// aContent serves and of the Libraries aLibraries serves, assembling a
// Project's snapshot with aBuilder, reading a Library's Draft through
// someWidgetTypes, and reading the labels of a saved state through someLabels,
// the service that serves them (task 85_3; ADR 0025).
func NewSavedStateService(aContent ProjectContent, aLibraries LibraryContent, aBuilder ProjectFileBuilder,
	someWidgetTypes library.WidgetTypeRepositories, someLabels LabelService) SavedStateService {
	return &savedStateServiceImpl{content: aContent, libraries: aLibraries, builder: aBuilder,
		widgetTypes: someWidgetTypes, labels: someLabels}
}

func (s savedStateServiceImpl) Save(ctx context.Context, scope draft.Scope, rawVersion int, rawTitle, rawAuthor string) (appdto.SavedState, error) {
	if !scope.IsProject() && !scope.IsLibrary() {
		return appdto.SavedState{}, invalidInput(fmt.Errorf("scope: a project or a library is required"))
	}
	title, err := draft.NewSavedStateTitle(rawTitle)
	if err != nil {
		return appdto.SavedState{}, invalidInput(err)
	}
	author, err := draft.NewAuthor(rawAuthor)
	if err != nil {
		return appdto.SavedState{}, invalidInput(err)
	}
	expected, err := draftVersion(rawVersion)
	if err != nil {
		return appdto.SavedState{}, err
	}

	var content draft.SavedStateContent
	if scope.IsProject() {
		content, err = s.projectContent(ctx, scope, expected)
	} else {
		content, err = s.libraryContent(ctx, scope, expected)
	}
	if err != nil {
		return appdto.SavedState{}, err
	}

	saved, err := s.content.Drafts().SaveSavedState(ctx, scope, expected, title, author, content)
	if err != nil {
		return appdto.SavedState{}, fmt.Errorf("cannot save %s: %w", scope, err)
	}
	return appdto.NewSavedState(saved), nil
}

// projectContent checks the Project is live and at expected, then assembles
// its ProjectFile with the builder a Deploy uses. A refused build is a Draft
// that cannot be saved.
func (s savedStateServiceImpl) projectContent(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft]) (draft.SavedStateContent, error) {
	projectID, _, err := s.content.open(ctx, scope.ProjectID().UUID())
	if err != nil {
		return draft.SavedStateContent{}, err
	}
	current, err := s.content.DraftVersion(ctx, projectID)
	if err != nil {
		return draft.SavedStateContent{}, err
	}
	if current.Number() != expected.Number() {
		return draft.SavedStateContent{}, draft.ConflictError{Expected: expected.Number(), Current: current.Number(), Scope: scope}
	}
	file, err := s.builder.Build(ctx, projectID.UUID())
	if err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return draft.SavedStateContent{}, err
		}
		return draft.SavedStateContent{}, DraftNotSaveableError{Reason: err.Error()}
	}
	return draft.NewProjectSavedStateContent(file), nil
}

// libraryContent checks the Library is live and at expected, then takes the
// copies of its Draft's WidgetTypes.
func (s savedStateServiceImpl) libraryContent(ctx context.Context, scope draft.Scope, expected version.Version[draft.Draft]) (draft.SavedStateContent, error) {
	libraryID, err := s.libraries.open(ctx, scope.LibraryID().UUID())
	if err != nil {
		return draft.SavedStateContent{}, err
	}
	current, err := s.libraries.Version(ctx, libraryID.UUID())
	if err != nil {
		return draft.SavedStateContent{}, err
	}
	if current != expected.Number() {
		return draft.SavedStateContent{}, draft.ConflictError{Expected: expected.Number(), Current: current, Scope: scope}
	}
	types, err := s.widgetTypes.ForLibrary(libraryID).FindAll(ctx)
	if err != nil {
		return draft.SavedStateContent{}, fmt.Errorf("cannot read the draft of %s: %w", scope, err)
	}
	copies := make([]library.WidgetTypeCopy, len(types))
	for i, wt := range types {
		copies[i] = library.CopyOf(wt)
	}
	return draft.NewLibrarySavedStateContent(copies), nil
}

func (s savedStateServiceImpl) List(ctx context.Context, scope draft.Scope, labelID uuid.UUID) ([]appdto.SavedState, error) {
	savedStates, err := s.content.Drafts().FindSavedStates(ctx, scope, draft.SavedStateFilter{LabelID: labelID})
	if err != nil {
		return nil, err
	}
	dtos := appdto.NewSavedStateList(savedStates)
	if err := s.attachLabels(ctx, scope, dtos); err != nil {
		return nil, err
	}
	return dtos, nil
}

func (s savedStateServiceImpl) Find(ctx context.Context, scope draft.Scope, rawNumber int) (appdto.SavedState, error) {
	number, err := savedStateNumber(rawNumber)
	if err != nil {
		return appdto.SavedState{}, err
	}
	savedState, err := s.content.Drafts().FindSavedState(ctx, scope, number)
	if err != nil {
		return appdto.SavedState{}, err
	}
	dto := appdto.NewSavedStateWithContent(savedState)
	return s.labelledSavedState(ctx, scope, dto)
}

// labelledSavedState returns dto with the labels that mark it (task 85_3).
func (s savedStateServiceImpl) labelledSavedState(ctx context.Context, scope draft.Scope, dto appdto.SavedState) (appdto.SavedState, error) {
	dtos := []appdto.SavedState{dto}
	if err := s.attachLabels(ctx, scope, dtos); err != nil {
		return appdto.SavedState{}, err
	}
	return dtos[0], nil
}

// attachLabels fills the labels of each saved state of the list, read for the
// whole list in one call of the label service (task 85_3; ADR 0025).
func (s savedStateServiceImpl) attachLabels(ctx context.Context, scope draft.Scope, dtos []appdto.SavedState) error {
	if len(dtos) == 0 {
		return nil
	}
	byNumber, err := s.labels.LabelsOf(ctx, scope)
	if err != nil {
		return err
	}
	for i := range dtos {
		dtos[i].Labels = byNumber[dtos[i].Number]
	}
	return nil
}

func (s savedStateServiceImpl) Rename(ctx context.Context, scope draft.Scope, rawNumber int, rawTitle string) (appdto.SavedState, error) {
	number, err := savedStateNumber(rawNumber)
	if err != nil {
		return appdto.SavedState{}, err
	}
	title, err := draft.NewSavedStateTitle(rawTitle)
	if err != nil {
		return appdto.SavedState{}, invalidInput(err)
	}
	savedState, err := s.content.Drafts().RenameSavedState(ctx, scope, number, title)
	if err != nil {
		return appdto.SavedState{}, err
	}
	// A title is metadata: the rename changes no content and no label.
	return s.labelledSavedState(ctx, scope, appdto.NewSavedState(savedState))
}

func (s savedStateServiceImpl) ProjectFile(ctx context.Context, scope draft.Scope, rawNumber int) (ProjectFileDownload, error) {
	if !scope.IsProject() {
		return ProjectFileDownload{}, invalidInput(fmt.Errorf("saved states of a library have no project file"))
	}
	number, err := savedStateNumber(rawNumber)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	projectID, _, err := s.content.open(ctx, scope.ProjectID().UUID())
	if err != nil {
		return ProjectFileDownload{}, err
	}
	savedState, err := s.content.Drafts().FindSavedState(ctx, scope, number)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	file, ok := savedState.Content().ProjectFile()
	if !ok {
		return ProjectFileDownload{}, fmt.Errorf("saved state %d of %s holds no project file", number, scope)
	}
	name, err := projectDownloadName(ctx, s.content, projectID)
	if err != nil {
		return ProjectFileDownload{}, err
	}
	return ProjectFileDownload{File: file, ProjectName: name}, nil
}

// savedStateNumber parses the number of a saved state a client names; a number
// below 1 is the client's input, not a missing saved state.
func savedStateNumber(raw int) (int, error) {
	if raw < 1 {
		return 0, invalidInput(fmt.Errorf("saved state number must be positive, got %d", raw))
	}
	return raw, nil
}
