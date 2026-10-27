package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

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

// DiscardLibraryPolicyName is the Name of the policy returned by
// NewDiscardLibraryPolicy.
const DiscardLibraryPolicyName = "discard_library"

// discardLibraryPolicy carries out the Discard of a Library (ADR 0012): it
// brings every WidgetType of the Draft back to the Library Release the
// Library is marked as being reset to — removing those added since,
// restoring those changed, recreating those removed under their IDs —, each
// in its own transaction through a Discard's repository, and clears the
// mark last; a library_discard_started event's job does it. While the mark
// stands no one else writes the Draft, so a run that failed half-way is run
// again from the Draft as it is. For a Library not being discarded the job
// does nothing.
type discardLibraryPolicy struct {
	libraries   library.LibraryRepository
	releases    library.LibraryReleaseRepository
	widgetTypes library.WidgetTypeRepositories
}

var _ Policy = (*discardLibraryPolicy)(nil)

// NewDiscardLibraryPolicy returns the policy carrying out Discards.
func NewDiscardLibraryPolicy(someLibraries library.LibraryRepository, someReleases library.LibraryReleaseRepository, someWidgetTypes library.WidgetTypeRepositories) Policy {
	return &discardLibraryPolicy{libraries: someLibraries, releases: someReleases, widgetTypes: someWidgetTypes}
}

func (p discardLibraryPolicy) Name() string { return DiscardLibraryPolicyName }

func (p discardLibraryPolicy) JobKey(e event.Event) (string, bool) {
	if e.Type() != event.EventTypeLibraryDiscardStarted {
		return "", false
	}
	return libraryJobKey(e)
}

func (p discardLibraryPolicy) Run(ctx context.Context, key string) error {
	libraryID, err := parseLibraryKey(key)
	if err != nil {
		return err
	}
	lib, err := p.libraries.FindByID(ctx, libraryID)
	if errors.Is(err, library.ErrLibraryNotFound) {
		return nil // deleted: its Draft goes with it
	}
	if err != nil {
		return fmt.Errorf("cannot find library %s: %w", libraryID, err)
	}
	to, discarding := lib.DiscardingTo()
	if !discarding {
		return nil
	}
	release, err := p.releases.FindByNumber(ctx, libraryID, to)
	if err != nil {
		return fmt.Errorf("cannot find release %s of library %s: %w", to, libraryID, err)
	}
	if err := p.reset(ctx, p.widgetTypes.ForDiscard(libraryID), release); err != nil {
		return fmt.Errorf("cannot discard library %s: %w", libraryID, err)
	}
	return retryOnRace(library.ErrLibraryConflict, func() error {
		lib, err := p.libraries.FindByID(ctx, libraryID)
		if err != nil {
			return fmt.Errorf("cannot reread library %s: %w", libraryID, err)
		}
		if _, err := p.libraries.Save(ctx, lib.FinishDiscard()); err != nil {
			return fmt.Errorf("cannot finish discard of library %s: %w", libraryID, err)
		}
		return nil
	})
}

// reset brings the Draft of repo back to release: first the WidgetTypes
// added since go, freeing their names; then each changed one is restored
// and each removed one recreated. A restore or recreation whose name another
// WidgetType of the Draft still holds — two names swapped since the
// release — waits for the others; when nothing else is left to do, the
// first such WidgetType takes a name no release holds, which breaks the
// cycle.
func (p discardLibraryPolicy) reset(ctx context.Context, repo library.WidgetTypeRepository, release library.LibraryRelease) error {
	draft, err := repo.FindAll(ctx)
	if err != nil {
		return fmt.Errorf("cannot read draft: %w", err)
	}
	inDraft := map[id.ID[library.WidgetType]]library.WidgetType{}
	for _, wt := range draft {
		if _, ok := release.FindWidgetType(wt.ID()); !ok {
			if err := repo.Delete(ctx, wt.Delete()); err != nil && !errors.Is(err, library.ErrWidgetTypeNotFound) {
				return fmt.Errorf("cannot remove widget type %s: %w", wt.ID(), err)
			}
			continue
		}
		inDraft[wt.ID()] = wt
	}

	// pending are the released WidgetTypes the Draft does not match yet.
	var pending []library.ReleasedWidgetType
	for _, released := range release.WidgetTypes() {
		if wt, ok := inDraft[released.ID()]; !ok || !released.Matches(wt) {
			pending = append(pending, released)
		}
	}
	for len(pending) > 0 {
		var waiting []library.ReleasedWidgetType
		for _, released := range pending {
			saved, err := p.restore(ctx, repo, inDraft[released.ID()], released)
			if errors.Is(err, library.ErrWidgetTypeNameTaken) {
				waiting = append(waiting, released)
				continue
			}
			if err != nil {
				return err
			}
			inDraft[released.ID()] = saved
		}
		if len(waiting) == len(pending) {
			// A cycle of names: move the first one out of the way.
			first := waiting[0]
			wt, ok := inDraft[first.ID()]
			if !ok {
				return fmt.Errorf("cannot recreate widget type %s: name %q taken", first.ID(), first.Name())
			}
			aside, err := library.NewWidgetTypeName(wt.Name().String() + " (" + wt.ID().String() + ")")
			if err != nil {
				return err
			}
			moved, err := wt.Update(wt.Version(), aside, wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), wt.InputPorts())
			if err != nil {
				return err
			}
			if inDraft[first.ID()], err = repo.Save(ctx, moved); err != nil {
				return fmt.Errorf("cannot rename widget type %s: %w", wt.ID(), err)
			}
		}
		pending = waiting
	}
	return nil
}

// restore saves through repo the WidgetType wt of the Draft (nil if it was
// removed) as released holds it.
func (p discardLibraryPolicy) restore(ctx context.Context, repo library.WidgetTypeRepository, wt library.WidgetType, released library.ReleasedWidgetType) (library.WidgetType, error) {
	var changed library.WidgetType
	var err error
	if wt == nil {
		changed, err = library.RecreateWidgetType(released)
	} else {
		changed, err = wt.ResetTo(released)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot reset widget type %s: %w", released.ID(), err)
	}
	saved, err := repo.Save(ctx, changed)
	if err != nil {
		return nil, fmt.Errorf("cannot save widget type %s: %w", released.ID(), err)
	}
	return saved, nil
}
