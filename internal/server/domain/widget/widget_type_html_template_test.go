package widget

import "testing"

func TestNewHtmlTemplate_Valid(t *testing.T) {
	const tmpl = "<div class='gauge'>{{.Value}}</div>"
	ht, err := NewHtmlTemplate(tmpl)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ht.String() != tmpl {
		t.Errorf("expected %q, got %q", tmpl, ht.String())
	}
}

func TestNewHtmlTemplate_Empty(t *testing.T) {
	ht, err := NewHtmlTemplate("")
	if err != nil {
		t.Fatalf("unexpected error for empty template: %v", err)
	}
	if ht.String() != "" {
		t.Errorf("expected empty string, got %q", ht.String())
	}
}

func TestHtmlTemplate_String(t *testing.T) {
	const content = "<span>{{.Tag}}</span>"
	ht, _ := NewHtmlTemplate(content)
	if ht.String() != content {
		t.Errorf("String() mismatch: expected %q, got %q", content, ht.String())
	}
}
