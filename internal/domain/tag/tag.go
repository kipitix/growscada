package tag

import "fmt"

// Tag - интерфейс, представляющий тег
// Тег - это базовая сущность системы, содержащая идентификатор, имя, тип, значение и качество
type Tag interface {
	ID() TagID
	Name() string
	Kind() TagKind
	Quality() TagQuality

	UpdateValue(TagValue, TagQuality) error

	ValueAsBoolean() (bool, error)
	ValueAsInteger() (int, error)
	ValueAsString() string

	Version() int
	IncrementVersion()
}

// tagImpl - структура реализации тега
type tagImpl struct {
	id TagID
	// TODO: make VO TagName
	name    string
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
func NewTag(anID TagID, aName string, aType TagKind) Tag {
	return &tagImpl{
		id:      anID,
		name:    aName,
		kind:    aType,
		value:   nil,
		quality: TagQualityBad,
		version: 0,
	}
}

// ID возвращает идентификатор тега
func (t tagImpl) ID() TagID {
	return t.id
}

// Name возвращает имя тега
func (t tagImpl) Name() string {
	return t.name
}

// Kind возвращает тип тега
func (t tagImpl) Kind() TagKind {
	return t.kind
}

// Quality возвращает качество тега
func (t tagImpl) Quality() TagQuality {
	return t.quality
}

// UpdateValue обновляет значение и качество тега
func (t *tagImpl) UpdateValue(aValue TagValue, aQuality TagQuality) error {
	if !t.kind.IsValidValue(aValue) {
		t.value = nil
		t.quality = TagQualityBad
		return fmt.Errorf("invalid value type: expected %s, got %T", t.kind, aValue)
	}
	t.value = aValue
	t.quality = aQuality
	return nil
}

// ValueAsBoolean возвращает значение тега как булево, если тип тега позволяет
func (t tagImpl) ValueAsBoolean() (bool, error) {
	if t.Kind() != TagKindBoolean {
		return false, fmt.Errorf("tag is not of type boolean")
	}
	if t.value == nil {
		return false, fmt.Errorf("tag value is nil")
	}
	boolValue, ok := t.value.(bool)
	if !ok {
		return false, fmt.Errorf("tag value is not a boolean")
	}
	return boolValue, nil
}

// ValueAsInteger возвращает значение тега как целое число, если тип тега позволяет
func (t tagImpl) ValueAsInteger() (int, error) {
	if t.Kind() != TagKindInteger {
		return 0, fmt.Errorf("tag is not of type integer")
	}
	if t.value == nil {
		return 0, fmt.Errorf("tag value is nil")
	}
	intValue, ok := t.value.(int)
	if !ok {
		return 0, fmt.Errorf("tag value is not an integer")
	}
	return intValue, nil
}

func (t tagImpl) ValueAsString() string {
	return fmt.Sprintf("%v", t.value)
}

// Version возвращает текущую версию тега
func (t tagImpl) Version() int {
	return t.version
}

// IncrementVersion увеличивает версию тега на единицу
func (t *tagImpl) IncrementVersion() {
	t.version++
}
