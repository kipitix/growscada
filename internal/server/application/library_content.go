package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/draft"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/organization"
)

// LibraryContent opens the live Libraries of one Organization for the
// services: every access to a Library first checks it (ADR 0007), so a Library
// marked deleted, or one of another Organization, does not exist for the API
// even while its cleanup runs. The content of a Library is its Draft
// (ADR 0016): reads go through the Library's scope, writes through the Draft's
// repository.
type LibraryContent struct {
	libraries    library.LibraryRepository
	drafts       draft.Repository
	organization id.ID[organization.Organization]
}

// NewLibraryContent returns the LibraryContent of the Libraries of the
// Organization anOrganizationID, its writes through someDrafts.
func NewLibraryContent(aLibraryRepository library.LibraryRepository, someDrafts draft.Repository, anOrganizationID id.ID[organization.Organization]) LibraryContent {
	return LibraryContent{libraries: aLibraryRepository, drafts: someDrafts, organization: anOrganizationID}
}

// Drafts is the repository of the Drafts of every Library.
func (c LibraryContent) Drafts() draft.Repository { return c.drafts }

// ScopeOf is the scope of the Draft of libraryID.
func (c LibraryContent) ScopeOf(libraryID id.ID[library.Library]) draft.Scope {
	return draft.LibraryScope(libraryID)
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

// Version returns the Version of the Draft of the live Library rawLibraryID:
// the number every DTO of its content carries. A Draft never written has
// version 0; its row, and the first version a write states, starts at 1.
func (c LibraryContent) Version(ctx context.Context, rawLibraryID uuid.UUID) (int, error) {
	libraryID := id.NewID(id.IDWithUUID[library.Library](rawLibraryID))
	found, ok, err := c.drafts.Read(ctx, draft.LibraryScope(libraryID))
	if err != nil {
		return 0, fmt.Errorf("cannot read the draft of library %s: %w", libraryID, err)
	}
	if !ok {
		return 1, nil
	}
	return found.Version().Number(), nil
}
