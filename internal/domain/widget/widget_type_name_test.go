package widget

import "testing"

func TestNewWidgetTypeName_ReturnsCorrectValue(t *testing.T) {
	name, err := NewWidgetTypeName("pressure_gauge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "pressure_gauge" {
		t.Errorf("expected 'pressure_gauge', got %q", name.String())
	}
}

func TestNewWidgetTypeName_EmptyString(t *testing.T) {
	name, err := NewWidgetTypeName("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "" {
		t.Errorf("expected empty string, got %q", name.String())
	}
}

func TestWidgetTypeName_String(t *testing.T) {
	const input = "temperature_sensor"
	name, _ := NewWidgetTypeName(input)
	if name.String() != input {
		t.Errorf("expected %q, got %q", input, name.String())
	}
}
