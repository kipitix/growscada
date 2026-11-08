package release

import "errors"

var (
	// ErrReleaseNotFound is a read or a note change of a Release number the
	// scope does not have.
	ErrReleaseNotFound = errors.New("release not found")
	// ErrCommitAlreadyReleased is a Create refused because a Release of that
	// Commit exists already: one Commit is published at most once (ADR 0021).
	ErrCommitAlreadyReleased = errors.New("commit already released")
)
