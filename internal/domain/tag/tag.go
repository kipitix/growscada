package tag

// Tag - интерфейс, представляющий тег
// Тег - это базовая сущность системы, содержащая идентификатор, имя, тип, значение и качество
type Tag interface {
	ID() TagID
	Name() string
	Kind() TagKind
	Value() TagValue
	Quality() TagQuality
	Version() int

	UpdateValue(TagValue, TagQuality) error
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
func NewTag(anID TagID, aName string, aKind TagKind, aValue TagValue, aQuality TagQuality, aVersion int) (Tag, error) {
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
func (t tagImpl) Name() string {
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
func (t *tagImpl) UpdateValue(aValue TagValue, aQuality TagQuality) error {
	// if !t.kind.IsValidValue(aValue) {
	// 	t.value = nil
	// 	t.quality = TagQualityBad
	// 	return fmt.Errorf("invalid value type: expected %s, got %T", t.kind, aValue)
	// }
	// t.value = aValue
	// t.quality = aQuality
	return nil
}

// IncrementVersion увеличивает версию тега на единицу
func (t *tagImpl) IncrementVersion() {
	t.version++
}
