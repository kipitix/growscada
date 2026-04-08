package tag

import "github.com/google/uuid"

// TagID - идентификатор тега, основанный на UUID
// Представляет собой value object для уникальной идентификации тегов
type TagID uuid.UUID

// String преобразует TagID в строковое представление UUID
func (id TagID) String() string {
	return uuid.UUID(id).String()
}

// ParseTagID создает TagID из строки UUID
// Возвращает ошибку, если строка не является валидным UUID
func ParseTagID(s string) (TagID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return TagID(uuid.Nil), err
	}
	return TagID(id), nil
}

// MustParseTagID создает TagID из строки, паникует при ошибке
// Используется в случаях, когда гарантируется валидность входной строки
func MustParseTagID(s string) TagID {
	id, err := ParseTagID(s)
	if err != nil {
		panic(err)
	}
	return id
}

// NewTagID генерирует новый случайный TagID
// Использует uuid.New() для создания уникального идентификатора
func NewTagID() TagID {
	return TagID(uuid.New())
}

// IsNil проверяет, является ли TagID пустым (nil)
// Сравнивает с uuid.Nil
func (id TagID) IsNil() bool {
	return uuid.UUID(id) == uuid.Nil
}
