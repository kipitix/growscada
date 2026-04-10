package tag

import "fmt"

// Tag - интерфейс, представляющий тег
// Тег - это базовая сущность системы, содержащая идентификатор, имя, тип, значение и качество
type Tag interface {
	ID() TagID
	Name() TagName
	Kind() TagKind
	Value() TagValue
	Quality() TagQuality
	Version() int

	UpdateValue(any, TagQuality) error
	IncrementVersion()
}

// tagImpl - структура реализации тега
type tagImpl struct {
	id TagID
	// TODO: make VO TagName
	name    TagName
	kind    TagKind
	value   TagValue
	quality TagQuality
	// TODO : make VO TagVersion
	version int
}

var _ Tag = (*tagImpl)(nil)

// NewTag создает новый тег с заданным идентификатором, именем и типом
// Устанавливает начальное значение nil и качество TagQualityBad
// version инициализируется значением 0
func NewTag(anID TagID, aName TagName, aKind TagKind, aValue TagValue, aQuality TagQuality, aVersion int) (Tag, error) {
	newTag := &tagImpl{
		id:      anID,
		name:    aName,
		kind:    aKind,
		value:   aValue,
		quality: aQuality,
		version: aVersion,
	}

	return newTag, nil
}

// ID возвращает идентификатор тега
func (t tagImpl) ID() TagID {
	return t.id
}

// Name возвращает имя тега
func (t tagImpl) Name() TagName {
	return t.name
}

// Kind возвращает тип тега
func (t tagImpl) Kind() TagKind {
	return t.kind
}

func (t tagImpl) Value() TagValue {
	return t.value
}

// Quality возвращает качество тега
func (t tagImpl) Quality() TagQuality {
	return t.quality
}

// Version возвращает текущую версию тега
func (t tagImpl) Version() int {
	return t.version
}

// UpdateValue обновляет значение и качество тега
func (t *tagImpl) UpdateValue(aValue any, aQuality TagQuality) error {
	t.quality = aQuality
	newValue, err := NewTagValue(aValue, t.kind)
	if err != nil {
		return fmt.Errorf("cannot update tag value: %w", err)
	}
	t.value = newValue
	return nil
}

// IncrementVersion увеличивает версию тега на единицу
func (t *tagImpl) IncrementVersion() {
	t.version++
}
