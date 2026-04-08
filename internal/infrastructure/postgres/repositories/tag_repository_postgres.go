package repositories

import (
	"context"
	"database/sql"

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
func NewTagRepositoryPostgres(db *sql.DB) *TagRepositoryPostgres {
	return &TagRepositoryPostgres{db: db}
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
	query := `SELECT id, name, kind, value, quality FROM tags`

	row := r.db.QueryRowContext(ctx, query)

	if row.Err() != nil {
		return nil, row.Err()
	}

	return nil, nil
}
