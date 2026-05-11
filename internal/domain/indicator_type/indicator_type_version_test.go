package indicator_type

import "testing"

func TestNewIndicatorTypeVersion_Valid(t *testing.T) {
	v, err := NewIndicatorTypeVersion(0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Int() != 0 {
		t.Errorf("expected 0, got %d", v.Int())
	}
}

func TestNewIndicatorTypeVersion_Negative_ReturnsError(t *testing.T) {
	_, err := NewIndicatorTypeVersion(-1)
	if err == nil {
		t.Error("expected error for negative version, got nil")
	}
}

func TestIndicatorTypeVersion_Next(t *testing.T) {
	v := IndicatorTypeVersion(5)
	if v.Next() != 6 {
		t.Errorf("expected 6, got %d", v.Next())
	}
}

func TestIndicatorTypeVersion_String(t *testing.T) {
	v := IndicatorTypeVersion(7)
	if v.String() != "7" {
		t.Errorf("expected \"7\", got %q", v.String())
	}
}

func TestIndicatorTypeVersionInitial_IsZero(t *testing.T) {
	if IndicatorTypeVersionInitial.Int() != 0 {
		t.Errorf("expected IndicatorTypeVersionInitial to be 0, got %d", IndicatorTypeVersionInitial.Int())
	}
}
