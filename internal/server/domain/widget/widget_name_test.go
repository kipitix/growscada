package widget

import "testing"

func TestNewWidgetName_Valid(t *testing.T) {
	name, err := NewWidgetName("pressure_indicator")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name.String() != "pressure_indicator" {
		t.Errorf("expected %q, got %q", "pressure_indicator", name.String())
	}
}

func TestNewWidgetName_Empty(t *testing.T) {
	_, err := NewWidgetName("")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestWidgetName_String(t *testing.T) {
	cases := []string{"valve", "pump_01", "FLOW_METER"}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			name, err := NewWidgetName(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name.String() != input {
				t.Errorf("expected %q, got %q", input, name.String())
			}
		})
	}
}
