package tag

// go-enum tool
// ENUM(Bad, Uncertain, Good, Simulated)
type TagQualityEnum int

// TagQuality is a value object for tag quality
type TagQuality interface {
	Quality() TagQualityEnum
	Equals(other TagQuality) bool
	String() string

	IsBad() bool
	IsUncertain() bool
	IsGood() bool
	IsSimulated() bool
}

type tagQuality struct {
	quality TagQualityEnum
}

var _ TagQuality = (*tagQuality)(nil)

func NewTagQuality(quality TagQualityEnum) TagQuality {
	return &tagQuality{quality: quality}
}

func (tq tagQuality) Quality() TagQualityEnum {
	return tq.quality
}

func (tq tagQuality) Equals(other TagQuality) bool {
	return tq.quality == other.Quality()
}

func (tq tagQuality) String() string {
	return tq.quality.String()
}

func (tq tagQuality) IsBad() bool {
	return tq.quality == TagQualityEnumBad
}

func (tq tagQuality) IsUncertain() bool {
	return tq.quality == TagQualityEnumUncertain
}

func (tq tagQuality) IsGood() bool {
	return tq.quality == TagQualityEnumGood
}

func (tq tagQuality) IsSimulated() bool {
	return tq.quality == TagQualityEnumSimulated
}
