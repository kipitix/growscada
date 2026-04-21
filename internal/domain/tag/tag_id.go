package tag

import "github.com/google/uuid"

// TagID - UUID-based tag identifier.
// Represents a value object for unique tag identification.
type TagID uuid.UUID

// UUID converts TagID to a uuid.UUID
func (id TagID) UUID() uuid.UUID {
	return uuid.UUID(id)
}

// NewTagID generates a new random TagID.
// Uses uuid.New() to create a unique identifier.
func NewTagID(opts ...TagIDOption) TagID {
	// Initialize config with default values
	cfg := &tagIDConfig{
		existingUUID: nil,
	}

	// Apply options
	for _, opt := range opts {
		opt(cfg)
	}

	// If an existing UUID is provided, use it
	if cfg.existingUUID != nil {
		return TagID(*cfg.existingUUID)
	}

	// Otherwise generate a new one
	return TagID(uuid.New())
}

// TagIDOption - option function for configuring TagID creation
type TagIDOption func(*tagIDConfig)

// tagIDConfig - configuration for TagID creation
type tagIDConfig struct {
	existingUUID *uuid.UUID
}

// TagIDWithUUID allows specifying an existing UUID for TagID creation.
// Used when a TagID needs to be created from an existing UUID.
func TagIDWithUUID(id uuid.UUID) TagIDOption {
	return func(cfg *tagIDConfig) {
		cfg.existingUUID = &id
	}
}

// MustParseTagID creates a TagID from a UUID string.
// Panics if the string is not a valid UUID.
func MustParseTagID(s string) TagID {
	id, err := ParseTagID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// ParseTagID creates a TagID from a UUID string.
// Returns an error if the string is not a valid UUID.
func ParseTagID(s string) (TagID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return TagID{}, err
	}
	return TagID(id), nil
}
