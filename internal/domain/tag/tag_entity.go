package tag

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gitverse.ru/kipitix/growscada/internal/domain/datatype"
)

type tagEntity struct {
	id           uuid.UUID
	name         string
	dataType     datatype.DataType
	dataSourceID uuid.UUID
	address      string
	scanRate     time.Duration
}

func NewTagEntity(
	id uuid.UUID,
	name string,
	dataType datatype.DataType,
	dataSourceID uuid.UUID,
	address string,
) (*tagEntity, error) {
	if id == uuid.Nil {
		return nil, errors.New("tag id required")
	}
	if name == "" {
		return nil, errors.New("tag name required")
	}

	return &tagEntity{
		id:           id,
		name:         name,
		dataType:     dataType,
		dataSourceID: dataSourceID,
		address:      address,
		scanRate:     1 * time.Second, // Значение по умолчанию
	}, nil
}

func (e *tagEntity) isValidValue(v any) bool {
	// Проверка соответствия типа
	switch e.dataType {
	case datatype.FloatType:
		_, ok := v.(float64)
		return ok
	case datatype.IntType:
		_, ok := v.(int32)
		return ok
	// ... другие типы
	default:
		return false
	}
}
