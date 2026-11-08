package appdto

import (
	"time"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
)

// Commit is the application-layer DTO of a Commit (task 72_2; ADR 0021): its
// number, title, author and time, the Version of the Draft it fixed, and the
// counts of its snapshot. WidgetTypes is filled by Find of a Library's Commit
// alone: the list never carries content.
type Commit struct {
	ID           uuid.UUID
	Number       int
	Title        string
	Author       string
	CreatedAt    time.Time
	DraftVersion int
	// Released reports whether a Release was made of this Commit; false until
	// task 72_3.
	Released    bool
	Counts      CommitCounts
	WidgetTypes []ReleasedWidgetType
}

// CommitCounts is the size of a Commit's snapshot.
type CommitCounts struct {
	Scenes       int
	Widgets      int
	Tags         int
	Devices      int
	Environments int
	WidgetTypes  int
}

// NewCommit creates a Commit DTO from the domain entity, without its content.
func NewCommit(c draft.Commit) Commit {
	return Commit{
		ID:           c.ID().UUID(),
		Number:       c.Number(),
		Title:        c.Title().String(),
		Author:       c.Author().String(),
		CreatedAt:    c.CreatedAt(),
		DraftVersion: c.DraftVersion().Number(),
		Counts:       commitCounts(c.Content()),
	}
}

// NewCommitWithContent creates a Commit DTO with the WidgetTypes of a
// Library's snapshot.
func NewCommitWithContent(c draft.Commit) Commit {
	dto := NewCommit(c)
	if types, ok := c.Content().WidgetTypes(); ok {
		dto.WidgetTypes = NewReleasedWidgetTypeList(types)
	}
	return dto
}

// NewCommitList creates the DTOs of a list of Commits, without content.
func NewCommitList(list []draft.Commit) []Commit {
	result := make([]Commit, len(list))
	for i, c := range list {
		result[i] = NewCommit(c)
	}
	return result
}

// commitCounts counts what a snapshot holds.
func commitCounts(content draft.CommitContent) CommitCounts {
	if file, ok := content.ProjectFile(); ok {
		counts := CommitCounts{
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
	return CommitCounts{WidgetTypes: len(types)}
}
