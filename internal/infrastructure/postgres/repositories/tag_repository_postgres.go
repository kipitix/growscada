package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"gitverse.ru/kipitix/growscada/internal/domain/tag"
)

// TagRepositoryPostgres реализация интерфейса TagRepository для работы с PostgreSQL
// Содержит подключение к базе данных
type TagRepositoryPostgres struct {
	db *sql.DB
}

// Убеждаемся, что TagRepositoryPostgres реализует интерфейс tag.TagRepository
var _ tag.TagRepository = (*TagRepositoryPostgres)(nil)

// NewTagRepositoryPostgres создает новый экземпляр репозитория тегов для PostgreSQL
// Принимает подключение к базе данных и возвращает указатель на TagRepositoryPostgres
func NewTagRepositoryPostgres(aDb *sql.DB) *TagRepositoryPostgres {
	return &TagRepositoryPostgres{db: aDb}
}

// NextID генерирует новый уникальный идентификатор тега
// Использует uuid.New() для генерации UUID и преобразует его в TagID
func (r TagRepositoryPostgres) NextID() tag.TagID {
	return tag.TagID(uuid.New())
}

// Save сохраняет тег в базе данных
// Временно возвращает nil, так как реализация находится в процессе разработки
// В будущем будет реализована логика сохранения с проверкой версий (оптимистичная блокировка)
func (r TagRepositoryPostgres) Save(ctx context.Context, tag tag.Tag) error {
	return nil

	/*
	   // SQL для обновления с проверкой версии
	   // Мы увеличиваем version на 1 в БД, но только если текущий version совпадает с тем, что в агрегате
	   query := `

	   	UPDATE orders
	   	SET status = $1, total_cents = $2, updated_at = $3, version = version + 1
	   	WHERE id = $4 AND version = $5

	   `

	   res, err := r.db.ExecContext(ctx, query,

	   	order.Status(),
	   	order.totalCents, // в реальном коде нужен геттер или доступ к полю
	   	order.updatedAt,
	   	order.ID(),
	   	order.Version(), // Ожидаемая версия

	   )

	   	if err != nil {
	   		return err
	   	}

	   rowsAffected, err := res.RowsAffected()

	   	if err != nil {
	   		return err
	   	}

	   // Если ни одна строка не обновлена, значит версия в БД уже изменилась

	   	if rowsAffected == 0 {
	   		return domain.ErrOptimisticLock
	   	}

	   // После успешного сохранения локальную версию агрегата можно обновить,
	   // если БД возвращает новую версию (через RETURNING),
	   // или просто инкрементировать локально, так как мы знаем, что успех = +1.
	   // Для строгости лучше прочитать новую версию из БД, но для примера:
	   // order.version++ (не рекомендуется делать внутри репо, лучше вернуть обновленный агрегат или сделать отдельный запрос)

	   // В идеале запрос должен быть: ... RETURNING version
	   // И тогда мы обновим поле order.version новым значением.

	   return nil
	*/
}

// FindByID получает тег из базы данных по его идентификатору
// Временно возвращает nil, nil, так как реализация находится в процессе разработки
// В будущем будет реализована логика выборки тега из таблицы базы данных
func (r TagRepositoryPostgres) FindByID(ctx context.Context, id tag.TagID) (tag.Tag, error) {
	return nil, nil
	/*
	   query := `SELECT id, status, total_cents, version, updated_at FROM orders WHERE id = $1`

	   row := r.db.QueryRowContext(ctx, query, id)


	   var order domain.Order
	   var statusStr string
	   err := row.Scan(&order.id, &statusStr, &order.totalCents, &order.version, &order.updatedAt)

	   	if err != nil {
	   		if errors.Is(err, sql.ErrNoRows) {
	   			return nil, domain.ErrOrderNotFound
	   		}
	   		return nil, err
	   	}

	   order.status = domain.Status(statusStr)
	   return &order, nil
	*/
}

func (r TagRepositoryPostgres) FindAll(ctx context.Context) ([]tag.Tag, error) {
	// SQL для выборки всех тегов
	query := `SELECT id, name, kind, value, quality, version FROM tags`
	// Запрос и обработка результатов
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("error querying tags: %w", err)
	}
	// Закрываем rows в конце
	defer rows.Close()
	// Инициализация слайса для хранения тегов
	var tags []tag.Tag
	// Проход по строкам
	for rows.Next() {
		var (
			uuid    uuid.UUID
			name    string
			kind    string
			value   string
			quality string
			version int
		)
		// Сканирование строки
		err := rows.Scan(&uuid, &name, &kind, &value, &quality, &version)
		if err != nil {
			return nil, fmt.Errorf("error scanning tag: %w", err)
		}
		// Создание нового VO TagID
		newID := tag.NewTagID(tag.TagIDWithUUID(uuid))
		// Создание нового VO TagKind
		newKind, err := tag.NewTagKind(kind)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag kind: %w", err)
		}
		// Создание нового VO TagValue
		newValue, err := tag.NewTagValue(value, newKind)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag value: %w", err)
		}
		// Создание нового VO TagQuality
		newQuality, err := tag.NewTagQuality(quality)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag quality: %w", err)
		}
		// Создание нового Aggregate Tag
		newTag, err := tag.NewTag(newID, name, newKind, newValue, newQuality, version)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag: %w", err)
		}
		// Добавление тега в слайс
		tags = append(tags, newTag)
	}
	// Проверка на ошибки после цикла
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over tags: %w", err)
	}
	// Возвращение слайса тегов
	return tags, nil
}
