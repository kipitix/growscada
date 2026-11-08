package library

import "fmt"

// Origin tells where a Library Release's content came from (task 72_5;
// ADR 0007): a Commit of the Library's Draft, or the Draft as it was before
// Commits existed (a legacy Release).
// Value Object.
type Origin struct {
	name string
}

var _ fmt.Stringer = Origin{}

// The origins of a Library Release.
var (
	// OriginCommit is a Release of a Commit.
	OriginCommit = Origin{name: "commit"}
	// OriginLegacy is a Release made before Commits: it names none.
	OriginLegacy = Origin{name: "legacy"}
)

// NewOrigin rebuilds an Origin as stored. For repositories.
func NewOrigin(raw string) (Origin, error) {
	switch raw {
	case OriginCommit.name:
		return OriginCommit, nil
	case OriginLegacy.name:
		return OriginLegacy, nil
	default:
		return Origin{}, fmt.Errorf("unknown library release origin %q", raw)
	}
}

// IsValid reports whether the Origin is one the domain knows.
func (o Origin) IsValid() bool { return o.name != "" }

// IsCommit reports whether the Release came from a Commit.
func (o Origin) IsCommit() bool { return o == OriginCommit }

// String implements [fmt.Stringer].
func (o Origin) String() string { return o.name }
