package tag

import (
	"context"
	"errors"
)

// ErrTagUpdateOptimisticLock ошибка, возникающая при конфликте версий при обновлении тега
// Используется для реализации оптимистичной блокировки при конкурентном доступе
var ErrTagUpdateOptimisticLock = errors.New("tag update optimistic lock")

// TagRepository - интерфейс репозитория для хранения и управления тегами
// Определяет операции для получения следующего идентификатора,
// сохранения тега и получения тега по его идентификатору
type TagRepository interface {
	// NextID возвращает новый уникальный идентификатор для тега
	NextID() TagID

	// Save сохраняет тег в хранилище
	// При успешном сохранении возвращает nil, при ошибке - соответствующую ошибку
	Save(context.Context, Tag) error

	// FindByID возвращает тег по его идентификатору
	// При успешном поиске возвращает тег и nil, при отсутствии тега или ошибке - nil и соответствующую ошибку
	FindByID(context.Context, TagID) (Tag, error)

	// FindAll возвращает все теги
	// При успешном поиске возвращает слайс тегов и nil, при ошибке - пустой слайс и соответствующую ошибку
	FindAll(context.Context) ([]Tag, error)
}
