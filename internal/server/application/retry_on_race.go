package application

import (
	"errors"
	"fmt"
	"slices"
)

// maxRaceAttempts bounds how often retryOnRace runs an operation that keeps
// losing the write race.
const maxRaceAttempts = 3

// retryOnRace runs op, which reads an aggregate, changes it and writes it,
// and runs it again while it loses a write race: the write's compare-and-swap
// found the aggregate changed since op read it (an error wrapping conflict).
// It serves the operations that state no expected Version (removal,
// reconciliation): what they do does not rest on what a client saw, so a
// race is no edit conflict. After maxRaceAttempts lost races it returns the
// last error, the aggregate's conflict, marked with ErrRaceLost.
func retryOnRace(conflict error, op func() error) error {
	return retryOnRaces(op, conflict)
}

// retryOnRaces is retryOnRace for an operation that can lose a race of
// several kinds: it runs again while op fails with any of conflicts.
func retryOnRaces(op func() error, conflicts ...error) error {
	var err error
	for range maxRaceAttempts {
		if err = op(); !slices.ContainsFunc(conflicts, func(conflict error) bool { return errors.Is(err, conflict) }) {
			return err
		}
	}
	return fmt.Errorf("%w after %d attempts: %w", ErrRaceLost, maxRaceAttempts, err)
}
