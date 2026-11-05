package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// LibraryContent opens the live Libraries of one Organization for the
// services: every access to a Library first checks it (ADR 0007), so a Library
// marked deleted, or one of another Organization, does not exist for the API
// even while its cleanup runs.
type LibraryContent struct {
	libraries    library.LibraryRepository
	organization id.ID[organization.Organization]
}

// NewLibraryContent returns the LibraryContent of the Libraries of the
// Organization anOrganizationID.
func NewLibraryContent(aLibraryRepository library.LibraryRepository, anOrganizationID id.ID[organization.Organization]) LibraryContent {
	return LibraryContent{libraries: aLibraryRepository, organization: anOrganizationID}
}

// open returns the ID of the live Library rawLibraryID. Returns an error
// wrapping library.ErrLibraryNotFound if there is none.
func (c LibraryContent) open(ctx context.Context, rawLibraryID uuid.UUID) (id.ID[library.Library], error) {
	libraryID := id.NewID(id.IDWithUUID[library.Library](rawLibraryID))
	if _, err := findOwnLibrary(ctx, c.libraries, c.organization, libraryID); err != nil {
		return libraryID, err
	}
	return libraryID, nil
}

// openNotDiscarding is open, refused while a Discard of the Library is under
// way: its Draft is read-only until the mark clears (ADR 0012). It is for an
// act that is not a write of the Draft and so passes no repository lock — a
// Check out of the Draft's EditLock.
func (c LibraryContent) openNotDiscarding(ctx context.Context, rawLibraryID uuid.UUID) (id.ID[library.Library], error) {
	libraryID := id.NewID(id.IDWithUUID[library.Library](rawLibraryID))
	found, err := findOwnLibrary(ctx, c.libraries, c.organization, libraryID)
	if err != nil {
		return libraryID, err
	}
	if _, discarding := found.DiscardingTo(); discarding {
		return libraryID, fmt.Errorf("%w: %s", library.ErrLibraryDiscarding, libraryID)
	}
	return libraryID, nil
}
