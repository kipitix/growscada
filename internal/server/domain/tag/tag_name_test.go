package tag

import "testing"

func TestNewTagName_ReturnsCorrectValue(t *testing.T) {
	name, err := NewTagName("temperature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "temperature" {
		t.Errorf("expected 'temperature', got %q", name.String())
	}
}

func TestNewTagName_EmptyString(t *testing.T) {
	name, err := NewTagName("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "" {
		t.Errorf("expected empty string, got %q", name.String())
	}
}

func TestTagName_String(t *testing.T) {
	const input = "pressure_sensor_1"
	name, _ := NewTagName(input)
	if name.String() != input {
		t.Errorf("expected %q, got %q", input, name.String())
	}
}
