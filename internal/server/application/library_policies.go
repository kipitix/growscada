package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/editlock"
	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
)

// Job keys of a Library: "library:<id>".
const libraryKeyPrefix = "library:"

// libraryJobKey returns the job key of the Library of e, if e is a
// LibraryEvent.
func libraryJobKey(e event.Event) (string, bool) {
	if ev, ok := e.(library.LibraryEvent); ok {
		return libraryKeyPrefix + ev.LibraryID().String(), true
	}
	return "", false
}

// parseLibraryKey returns the Library of a job key "library:<id>".
func parseLibraryKey(key string) (id.ID[library.Library], error) {
	raw, ok := strings.CutPrefix(key, libraryKeyPrefix)
	if !ok {
		return id.ID[library.Library]{}, fmt.Errorf("invalid job key %q", key)
	}
	libraryUUID, err := uuid.Parse(raw)
	if err != nil {
		return id.ID[library.Library]{}, fmt.Errorf("invalid library ID in job key %q: %w", key, err)
	}
	return id.NewID(id.IDWithUUID[library.Library](libraryUUID)), nil
}

// LibraryCleanupPolicyName is the Name of the policy returned by
// NewLibraryCleanupPolicy.
const LibraryCleanupPolicyName = "remove_deleted_library"

// libraryCleanupPolicy removes a Library marked deleted with its Draft and
// its Library Releases; a library_deleted event's job does it. It was marked
// only while no live Project pinned it, and no pin is added to it since; the
// pins of Projects marked deleted hold it until their cleanup removes them,
// and the job is run again until then. For a live Library the job does
// nothing.
type libraryCleanupPolicy struct {
	libraries library.LibraryRepository
}

var _ Policy = (*libraryCleanupPolicy)(nil)

// NewLibraryCleanupPolicy returns the policy removing deleted Libraries.
func NewLibraryCleanupPolicy(someLibraries library.LibraryRepository) Policy {
	return &libraryCleanupPolicy{libraries: someLibraries}
}

func (p libraryCleanupPolicy) Name() string { return LibraryCleanupPolicyName }

func (p libraryCleanupPolicy) JobKey(e event.Event) (string, bool) {
	if e.Type() != event.EventTypeLibraryDeleted {
		return "", false
	}
	return libraryJobKey(e)
}

func (p libraryCleanupPolicy) Run(ctx context.Context, key string) error {
	// A policy writes as the server: it passes no EditLock check (ADR 0015).
	ctx = editlock.System(ctx)
	libraryID, err := parseLibraryKey(key)
	if err != nil {
		return err
	}
	deleted, err := p.libraries.FindDeleted(ctx, libraryID)
	if errors.Is(err, library.ErrLibraryNotFound) {
		return nil // live, or already removed
	}
	if err != nil {
		return fmt.Errorf("cannot find deleted library %s: %w", libraryID, err)
	}
	if err := p.libraries.Delete(ctx, deleted); err != nil && !errors.Is(err, library.ErrLibraryNotFound) {
		return fmt.Errorf("cannot remove library %s: %w", libraryID, err)
	}
	return nil
}
