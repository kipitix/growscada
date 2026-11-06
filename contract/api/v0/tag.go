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
// definition (name, type, version) with its state (value, quality, seq,
// written_at). Version is the Version of the Project's Draft, the same in
// every content entity of one response (ADR 0016); Seq is the Journal
// position of the latest change of the state, so of two states of one tag the
// one with the greater Seq is the newer. Until a supplier writes, a tag's
// state is its type's zero value with quality bad, Seq is 0, below every
// change's, and WrittenAt is absent. WrittenAt (RFC 3339) is when the state
// was last written, changed or not: a Device whose value repeats stays
// visibly alive without a Journal entry (ADR 0019).
type TagResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	Quality   string    `json:"quality"`
	Version   int       `json:"version"`
	Seq       int64     `json:"seq"`
	WrittenAt *string   `json:"written_at,omitempty"`
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
// of the state — the one the write took, or the previous one when the value
// and Quality did not change — and when it was last written. A write that
// changes nothing publishes no tag_updated (ADR 0019).
type UpdateTagResponse struct {
	Seq       int64   `json:"seq"`
	WrittenAt *string `json:"written_at,omitempty"`
}
