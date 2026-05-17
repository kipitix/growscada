package scene

// BackgroundHTML - static HTML content rendered as the scene background,
// on top of which widgets are overlaid.
// Value Object.
type BackgroundHTML struct {
	content string
}

// NewBackgroundHTML creates a BackgroundHTML value object.
func NewBackgroundHTML(content string) BackgroundHTML {
	return BackgroundHTML{content: content}
}

// Content returns the raw HTML string.
func (h BackgroundHTML) Content() string { return h.content }
