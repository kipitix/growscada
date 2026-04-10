package tag

type TagName string

func NewTagName(name string) (TagName, error) {
	return TagName(name), nil
}

func (tn TagName) String() string {
	return string(tn)
}
