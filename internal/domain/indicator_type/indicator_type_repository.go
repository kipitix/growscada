package indicator_type

import (
	"context"
	"errors"
)

var ErrIndicatorTypeNotFound = errors.New("indicator type not found")

// IndicatorTypeRepository - repository interface for storing and managing indicator types.
type IndicatorTypeRepository interface {
	// NextID returns a new unique identifier for an indicator type.
	NextID() IndicatorTypeID

	// Save stores an indicator type in the repository.
	Save(context.Context, IndicatorType) error

	// FindByID returns an indicator type by its identifier.
	// Returns ErrIndicatorTypeNotFound if not found.
	FindByID(context.Context, IndicatorTypeID) (IndicatorType, error)

	// DeleteByID removes an indicator type by its identifier and returns it.
	// Returns ErrIndicatorTypeNotFound if the indicator type does not exist.
	DeleteByID(context.Context, IndicatorTypeID) (IndicatorType, error)

	// FindAll returns all indicator types.
	FindAll(context.Context) ([]IndicatorType, error)
}
