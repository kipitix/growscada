package tag

import "github.com/google/uuid"

// TagID - идентификатор тега, основанный на UUID
// Представляет собой value object для уникальной идентификации тегов
type TagID uuid.UUID

// UUID преобразует TagID в строковое представление UUID
func (id TagID) UUID() uuid.UUID {
	return uuid.UUID(id)
}

// NewTagID генерирует новый случайный TagID
// Использует uuid.New() для создания уникального идентификатора
func NewTagID(opts ...TagIDOption) TagID {
	// Создаем конфигурацию с значениями по умолчанию
	cfg := &tagIDConfig{
		existingUUID: nil,
	}

	// Применяем опции
	for _, opt := range opts {
		opt(cfg)
	}

	// Если указан существующий UUID, используем его
	if cfg.existingUUID != nil {
		return TagID(*cfg.existingUUID)
	}

	// Иначе генерируем новый
	return TagID(uuid.New())
}

// TagIDOption - функция опции для настройки создания TagID
type TagIDOption func(*tagIDConfig)

// tagIDConfig - конфигурация для создания TagID
type tagIDConfig struct {
	existingUUID *uuid.UUID
}

// WithUUID позволяет указать существующий UUID для создания TagID
// Используется, когда нужно создать TagID из уже существующего UUID
func WithUUID(id uuid.UUID) TagIDOption {
	return func(cfg *tagIDConfig) {
		cfg.existingUUID = &id
	}
}

// MustParseTagID создает TagID из строки UUID
// Паникует, если строка не является валидным UUID
func MustParseTagID(s string) TagID {
	id, err := ParseTagID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// ParseTagID создает TagID из строки UUID
// Возвращает ошибку, если строка не является валидным UUID
func ParseTagID(s string) (TagID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return TagID{}, err
	}
	return TagID(id), nil
}
