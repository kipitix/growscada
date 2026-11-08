package appdto

import (
	"time"

	"github.com/kipitix/growscada/internal/server/domain/release"
)

// Release is the application-layer DTO of a Release (task 72_3; ADR 0021): its
// number, the Commit it published, the Commit's title at that moment, its
// note, author, time, origin and the counts of its snapshot. It carries no
// running flag: the running Release is the Deployment's (task 72_4).
type Release struct {
	Number       int
	CommitNumber int
	Title        string
	Note         string
	Author       string
	CreatedAt    time.Time
	Origin       string
	Counts       CommitCounts
}

// NewRelease creates a Release DTO from the domain aggregate, without its
// content.
func NewRelease(r release.Release) Release {
	return Release{
		Number:       r.Number().Int(),
		CommitNumber: r.CommitNumber(),
		Title:        r.CommitTitle().String(),
		Note:         r.Note().String(),
		Author:       r.Author().String(),
		CreatedAt:    r.CreatedAt(),
		Origin:       r.Origin().String(),
		Counts:       commitCounts(r.Content()),
	}
}

// NewReleaseList creates the DTOs of a list of Releases, newest first.
func NewReleaseList(list []release.Release) []Release {
	result := make([]Release, len(list))
	for i, r := range list {
		result[i] = NewRelease(r)
	}
	return result
}
