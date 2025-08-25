package tag

import (
	"errors"

	"github.com/google/uuid"
	"gitverse.ru/kipitix/growscada/internal/domain/history"
	"gitverse.ru/kipitix/growscada/internal/domain/value"
)

type Tag interface {
	ID() uuid.UUID
	Name() string
	CurrentValue() value.Value
	UpdateValue(value any, quality value.Quality) error
}

type tag struct {
	entity  tagEntity        // Встроенная сущность
	current value.Value      // Текущее значение
	history *history.History // История значений
	version int              // Для оптимистичной блокировки
}

var _ Tag = (*tag)(nil)

func NewTag(entity tagEntity, historyCapacity int) *tag {
	return &tag{
		entity:  entity,
		history: history.NewHistory(historyCapacity),
		version: 1,
	}
}

// Методы агрегата
func (t *tag) UpdateValue(newValue any, quality value.Quality) error {
	if !t.entity.isValidValue(newValue) {
		return errors.New("invalid value type")
	}

	newVal := value.NewValue(newValue, quality)
	if t.shouldUpdate(newVal) {
		t.current = newVal
		t.history.Add(newVal)
		t.version++
	}
	return nil
}

func (t *tag) shouldUpdate(newVal value.Value) bool {
	// Логика deadband и проверки качества
	return true
}

// Геттеры
func (t *tag) ID() uuid.UUID {
	return t.entity.id
}

func (t *tag) Name() string {
	return t.entity.name
}

func (t *tag) CurrentValue() value.Value {
	return t.current
}
