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
	_, err := NewWidgetTypeName("")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestWidgetTypeName_String(t *testing.T) {
	const input = "temperature_sensor"
	name, _ := NewWidgetTypeName(input)
	if name.String() != input {
		t.Errorf("expected %q, got %q", input, name.String())
	}
}
