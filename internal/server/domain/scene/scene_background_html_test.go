package scene

import "testing"

func TestNewBackgroundHTML_ValidContent(t *testing.T) {
	const html = "<svg><rect width='100' height='100'/></svg>"
	bg := NewBackgroundHTML(html)
	if bg.Content() != html {
		t.Errorf("expected %q, got %q", html, bg.Content())
	}
}

func TestNewBackgroundHTML_Empty(t *testing.T) {
	bg := NewBackgroundHTML("")
	if bg.Content() != "" {
		t.Errorf("expected empty content, got %q", bg.Content())
	}
}

func TestBackgroundHTML_ContentRoundtrip(t *testing.T) {
	cases := []string{
		"<div class='bg'></div>",
		"<!-- comment -->",
		"plain text",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			bg := NewBackgroundHTML(input)
			if bg.Content() != input {
				t.Errorf("Content() mismatch: expected %q, got %q", input, bg.Content())
			}
		})
	}
}
