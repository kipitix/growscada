package apiv0

import (
	"strconv"
	"time"

	"github.com/google/uuid"
)

// CommitPath is the path, under a Project's or a Library's path, of its
// Commits: the immutable snapshots a Save fixes (task 72_2; ADR 0021).
const CommitPath = "/commits"

// CommitPathOf is the path of the Commit number of a Project or Library.
func CommitPathOf(number int) string { return CommitPath + "/" + strconv.Itoa(number) }

// CreateCommitRequest is the HTTP DTO of POST …/commits: a Save that fixes the
// Draft at version as a Commit with a title and the Author who made it.
type CreateCommitRequest struct {
	// Version is the Version of the Draft the client saw; a stale one is 409
	// draft_conflict, and one equal to the last Commit's is 409
	// nothing_to_save.
	Version int    `json:"version"`
	Title   string `json:"title"`
	Author  string `json:"author"`
}

// RenameCommitRequest is the HTTP DTO of PATCH …/commits/{n}: the new title of
// a Commit. It states no Draft Version: a title is not content.
type RenameCommitRequest struct {
	Title string `json:"title"`
}

// CommitContentCounts is the size of a Commit's snapshot: Scenes, Widgets,
// Tags, Devices and Environments of a Project's ProjectFile, WidgetTypes of a
// Library's Draft. The fields of the other kind are zero.
type CommitContentCounts struct {
	Scenes       int `json:"scenes"`
	Widgets      int `json:"widgets"`
	Tags         int `json:"tags"`
	Devices      int `json:"devices"`
	Environments int `json:"environments"`
	WidgetTypes  int `json:"widget_types"`
}

// CommitResponse is the HTTP DTO of a Commit without its content: its number
// within the Draft, its title, its author, the time it was made, the Version
// of the Draft it fixed, whether a Release was made of it (a Project's or a
// Library's — task 72_3, task 72_5) and the counts of its snapshot.
// WidgetTypes is filled for the Commit of a Library alone, by GET …/commits/{n};
// a Project's Commit is read as a ProjectFile from …/commits/{n}/projectfile.
type CommitResponse struct {
	ID            uuid.UUID                    `json:"id"`
	Number        int                          `json:"number"`
	Title         string                       `json:"title"`
	Author        string                       `json:"author"`
	CreatedAt     time.Time                    `json:"created_at"`
	DraftVersion  int                          `json:"draft_version"`
	Released      bool                         `json:"released"`
	ContentCounts CommitContentCounts          `json:"content_counts"`
	WidgetTypes   []ReleasedWidgetTypeResponse `json:"widget_types,omitempty"`
}

// GetCommitsResponse is the HTTP DTO of GET …/commits: newest first, without
// content.
type GetCommitsResponse struct {
	Commits []CommitResponse `json:"commits"`
}
