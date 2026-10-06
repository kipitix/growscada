package application

import "errors"

// maxRaceAttempts bounds how often retryOnRace runs an operation that keeps
// losing the write race.
const maxRaceAttempts = 3

// retryOnRace runs op, which reads an aggregate, changes it and writes it,
// and runs it again while it loses a write race: the write's compare-and-swap
// found the aggregate changed since op read it (an error wrapping conflict).
// It serves the operations that state no expected Version (removal,
// reconciliation): what they do does not rest on what a client saw, so a
// race is no edit conflict. After maxRaceAttempts lost races it returns the
// last error, the aggregate's conflict.
func retryOnRace(conflict error, op func() error) error {
	var err error
	for range maxRaceAttempts {
		if err = op(); !errors.Is(err, conflict) {
			return err
		}
	}
	return err
}
