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
// Реализована логика сохранения с проверкой версий (оптимистичная блокировка)
func (r TagRepositoryPostgres) Save(ctx context.Context, aTag tag.Tag) error {
	// Если тег новый, то вставляем его в базу данных
	if aTag.Version() == tag.TagVersionInitial {
		// SQL для вставки нового тега
		sqlResult, err := r.db.ExecContext(ctx,
			`INSERT INTO tags (id, name, kind, value, quality, version)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			aTag.ID().UUID(), aTag.Name().String(), aTag.Kind().String(), aTag.Value().String(), aTag.Quality().String(), aTag.Version(),
		)
		// Обработка ошибки
		if err != nil {
			return fmt.Errorf("cannot insert new tag: %w", err)
		}
		// Проверка количества измененных строк
		if rowsAffected, _ := sqlResult.RowsAffected(); rowsAffected != 1 {
			return fmt.Errorf("expected 1 row affected on insert, got %d", rowsAffected)
		}

		return nil
	}

	// Если тег уже существует, то обновляем его в базе данных
	if aTag.Version() > tag.TagVersionInitial {
		// SQL для обновления тега
		sqlResult, err := r.db.ExecContext(ctx,
			`UPDATE tags
			 SET name = $1, kind = $2, value = $3, quality = $4, version = $5, version = version + 1
			 WHERE id = $6 AND version = $7`,
			aTag.Name().String(), aTag.Kind().String(), aTag.Value().String(), aTag.Quality().String(), aTag.Version(), aTag.ID().UUID(), aTag.Version(),
		)
		// Обработка ошибки
		if err != nil {
			return fmt.Errorf("cannot update tag: %w", err)
		}
		// Проверка количества измененных строк
		if rowsAffected, _ := sqlResult.RowsAffected(); rowsAffected != 1 {
			return fmt.Errorf("expected 1 row affected on update, got %d", rowsAffected)
		}
		// Увеличиваем версию тега
		aTag.IncrementVersion()

		return nil
	}

	// Если версия тега меньше нуля, то возвращаем ошибку
	return fmt.Errorf("undefined behavior with version %d", aTag.Version())
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
		// Создание нового VO TagName
		newName, err := tag.NewTagName(name)
		if err != nil {
			return nil, fmt.Errorf("cannot create tag name: %w", err)
		}
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
		newTag, err := tag.NewTag(newID, newName, newKind, newValue, newQuality, version)
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
