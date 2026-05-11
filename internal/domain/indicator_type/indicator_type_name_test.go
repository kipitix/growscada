package indicator_type

import "testing"

func TestNewIndicatorTypeName_ReturnsCorrectValue(t *testing.T) {
	name, err := NewIndicatorTypeName("pressure_gauge")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "pressure_gauge" {
		t.Errorf("expected 'pressure_gauge', got %q", name.String())
	}
}

func TestNewIndicatorTypeName_EmptyString(t *testing.T) {
	name, err := NewIndicatorTypeName("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "" {
		t.Errorf("expected empty string, got %q", name.String())
	}
}

func TestIndicatorTypeName_String(t *testing.T) {
	const input = "temperature_sensor"
	name, _ := NewIndicatorTypeName(input)
	if name.String() != input {
		t.Errorf("expected %q, got %q", input, name.String())
	}
}
