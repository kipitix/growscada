package apiv0

import (
	"github.com/google/uuid"
)

// Name filters of GET /tags; at most one may be given.
const (
	// TagsQueryName selects the tag with this exact name, unique within the
	// project.
	TagsQueryName = "name"
	// TagsQueryNamePattern selects tags whose whole name matches a wildcard
	// pattern ("*" any sequence, "?" one character).
	TagsQueryNamePattern = "name_pattern"
	// TagsQueryNameRegex selects tags whose name matches a Go (RE2) regular
	// expression anywhere.
	TagsQueryNameRegex = "name_regex"
)

// TagResponse is the HTTP DTO for representing a tag in API responses: its
// definition (name, type, version) with its state (value, quality, seq).
// Version is the Version of the Project's Draft, the same in every content
// entity of one response (ADR 0016); Seq numbers the write of the state, each
// write higher than every write before it, so of two states of one tag the
// one with the greater Seq is the newer. Until a supplier writes, a tag's
// state is its type's zero value with quality bad, and Seq is 0, below
// every write's.
type TagResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	Type    string    `json:"type"`
	Value   string    `json:"value"`
	Quality string    `json:"quality"`
	Version int       `json:"version"`
	Seq     int64     `json:"seq"`
}

// GetTagsResponse is the HTTP DTO for a list of tags response.
type GetTagsResponse struct {
	Tags []TagResponse `json:"tags"`
}

// CreateTagRequest is the HTTP DTO for a tag creation request: the tag's
// definition and the Version of the Draft the client saw (ADR 0016). Its
// state starts as its type's zero value with quality bad.
type CreateTagRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Version int    `json:"version"`
}

// CreateTagResponse is the HTTP DTO for a tag creation response.
type CreateTagResponse struct {
	ID uuid.UUID `json:"id"`
}

// UpdateTagRequest is the HTTP DTO for a tag value update request. It states
// no version: a tag's state has a single writer, and the latest write wins.
type UpdateTagRequest struct {
	Value   string `json:"value"`
	Quality string `json:"quality"`
}

// UpdateTagResponse is the HTTP DTO for a tag value update response: the Seq
// the write took, the one its tag_updated carries.
type UpdateTagResponse struct {
	Seq int64 `json:"seq"`
}
