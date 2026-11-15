package appdto

import (
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
)

// SavedState is the application-layer DTO of a saved state (task 72_2; ADR
// 0021): its number, title, author and time, the Version of the Draft it fixed,
// and the counts of its snapshot. WidgetTypes is filled by Find of a Library's
// saved state alone: the list never carries content.
type SavedState struct {
	ID           uuid.UUID
	Number       int
	Title        string
	Author       string
	CreatedAt    time.Time
	DraftVersion int
	Counts       SavedStateCounts
	// Labels are the labels that mark this saved state, ordered by name
	// ignoring case (task 85_3; ADR 0025).
	Labels      []Label
	WidgetTypes []WidgetTypeCopy
}

// SavedStateCounts is the size of a saved state's snapshot.
type SavedStateCounts struct {
	Scenes       int
	Widgets      int
	Tags         int
	Devices      int
	Environments int
	WidgetTypes  int
}

// NewSavedState creates a saved state DTO from the domain entity, without its
// content.
func NewSavedState(c draft.SavedState) SavedState {
	return SavedState{
		ID:           c.ID().UUID(),
		Number:       c.Number(),
		Title:        c.Title().String(),
		Author:       c.Author().String(),
		CreatedAt:    c.CreatedAt(),
		DraftVersion: c.DraftVersion().Number(),
		Counts:       savedStateCounts(c.Content()),
	}
}

// NewSavedStateWithContent creates a saved state DTO with the WidgetTypes of a
// Library's snapshot.
func NewSavedStateWithContent(c draft.SavedState) SavedState {
	dto := NewSavedState(c)
	if types, ok := c.Content().WidgetTypes(); ok {
		dto.WidgetTypes = NewWidgetTypeCopyList(types)
	}
	return dto
}

// NewSavedStateList creates the DTOs of a list of saved states, without
// content.
func NewSavedStateList(list []draft.SavedState) []SavedState {
	result := make([]SavedState, len(list))
	for i, c := range list {
		result[i] = NewSavedState(c)
	}
	return result
}

// savedStateCounts counts what a snapshot holds.
func savedStateCounts(content draft.SavedStateContent) SavedStateCounts {
	if file, ok := content.ProjectFile(); ok {
		counts := SavedStateCounts{
			Scenes:       len(file.Scenes),
			Tags:         len(file.Tags),
			Devices:      len(file.Devices),
			Environments: len(file.Environments),
			WidgetTypes:  len(file.WidgetTypes),
		}
		for _, scene := range file.Scenes {
			counts.Widgets += len(scene.Widgets)
		}
		return counts
	}
	types, _ := content.WidgetTypes()
	return SavedStateCounts{WidgetTypes: len(types)}
}
