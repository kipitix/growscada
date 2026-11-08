package apiv0

import (
	"strconv"
	"time"
)

// ReleasePath is the path, under a Project's path, of its Releases: the
// immutable publications of its Commits (task 72_3; ADR 0021).
const ReleasePath = "/releases"

// ReleasePathOf is the path of Release number of a Project.
func ReleasePathOf(number int) string { return ReleasePath + "/" + strconv.Itoa(number) }

// ReleaseProjectFilePathOf is the path of the ProjectFile a Project's Release
// copied.
func ReleaseProjectFilePathOf(number int) string { return ReleasePathOf(number) + "/projectfile" }

// ReleaseDeployPathOf is the path of the command that makes Release number the
// Project's running Deployment (task 72_4).
func ReleaseDeployPathOf(number int) string { return ReleasePathOf(number) + "/deploy" }

// RevertToDeployedPath, under a Project's path, resets the Draft to the
// Deployment the Project runs (task 72_4). Discard resets it to the last
// Commit instead.
const RevertToDeployedPath = "/revert-to-deployed"

// The origins of a Release: a Commit, or a project file (task 72_4).
const (
	ReleaseOriginCommit = "commit"
	ReleaseOriginFile   = "file"
)

// CreateReleaseRequest is the HTTP DTO of POST …/releases: the publication of
// the Commit commit of the Project as the next Release, with an optional note
// and the Author who published it. It states no Draft Version: a Commit is
// immutable.
type CreateProjectReleaseRequest struct {
	Commit int    `json:"commit"`
	Note   string `json:"note,omitempty"`
	Author string `json:"author"`
}

// UpdateReleaseNoteRequest is the HTTP DTO of PATCH …/releases/{n}: the new
// note of a Release. The content never changes.
type UpdateReleaseNoteRequest struct {
	Note string `json:"note"`
}

// ReleaseResponse is the HTTP DTO of a Release without its content: its
// number, the Commit it published and that Commit's title at the moment, its
// note, author, time, origin and the counts of its snapshot. It carries no
// running flag: the running Release is the Deployment's (task 72_4).
type ReleaseResponse struct {
	Number        int                 `json:"number"`
	CommitNumber  int                 `json:"commit_number"`
	Title         string              `json:"title"`
	Note          string              `json:"note"`
	Author        string              `json:"author"`
	CreatedAt     time.Time           `json:"created_at"`
	Origin        string              `json:"origin"`
	ContentCounts CommitContentCounts `json:"content_counts"`
}

// GetReleasesResponse is the HTTP DTO of GET …/releases: newest first.
type GetReleasesResponse struct {
	Releases []ReleaseResponse `json:"releases"`
}
