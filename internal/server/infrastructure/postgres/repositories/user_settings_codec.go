package repositories

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/kipitix/growscada/internal/server/domain/settings"
)

// libraryOrderPayload is the stored shape of the library_order setting: a
// document, so the key can gain fields later without a migration.
type libraryOrderPayload struct {
	LibraryIDs []uuid.UUID `json:"library_ids"`
}

// encodeLibraryOrder encodes the order as the row's payload.
func encodeLibraryOrder(order settings.Order) ([]byte, error) {
	payload, err := json.Marshal(libraryOrderPayload{LibraryIDs: order.IDs()})
	if err != nil {
		return nil, fmt.Errorf("cannot encode library order: %w", err)
	}
	return payload, nil
}

// decodeLibraryOrder decodes the order of a row's payload. Duplicates and the
// zero ID are normalized away, as a client's request is.
func decodeLibraryOrder(payload []byte) (settings.Order, error) {
	var stored libraryOrderPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return settings.Order{}, fmt.Errorf("cannot decode library order: %w", err)
	}
	return settings.NewOrder(stored.LibraryIDs), nil
}

// projectOrderPayload is the stored shape of the project_order setting
// (task 83): a document of its own, so each key keeps its own field name.
type projectOrderPayload struct {
	ProjectIDs []uuid.UUID `json:"project_ids"`
}

// encodeProjectOrder encodes the order as the row's payload.
func encodeProjectOrder(order settings.Order) ([]byte, error) {
	payload, err := json.Marshal(projectOrderPayload{ProjectIDs: order.IDs()})
	if err != nil {
		return nil, fmt.Errorf("cannot encode project order: %w", err)
	}
	return payload, nil
}

// decodeProjectOrder decodes the order of a row's payload. Duplicates and the
// zero ID are normalized away, as a client's request is.
func decodeProjectOrder(payload []byte) (settings.Order, error) {
	var stored projectOrderPayload
	if err := json.Unmarshal(payload, &stored); err != nil {
		return settings.Order{}, fmt.Errorf("cannot decode project order: %w", err)
	}
	return settings.NewOrder(stored.ProjectIDs), nil
}
