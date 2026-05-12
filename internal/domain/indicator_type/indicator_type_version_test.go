package indicator_type

import "testing"

func TestNewIndicatorTypeVersion_ValidValues(t *testing.T) {
	cases := []int{0, 1, 100}
	for _, v := range cases {
		got, err := NewIndicatorTypeVersion(IndicatorTypeVersionWithNumber(v))
		if err != nil {
			t.Errorf("NewIndicatorTypeVersion(%d): unexpected error: %v", v, err)
		}
		if got.Number() != v {
			t.Errorf("NewIndicatorTypeVersion(%d): got %d", v, got.Number())
		}
	}
}

func TestNewIndicatorTypeVersion_NegativeReturnsError(t *testing.T) {
	_, err := NewIndicatorTypeVersion(IndicatorTypeVersionWithNumber(-1))
	if err == nil {
		t.Error("expected error for negative version, got nil")
	}
}

func TestIndicatorTypeVersion_Constants(t *testing.T) {
	if IndicatorTypeVersionInitial.Number() != 0 {
		t.Errorf("IndicatorTypeVersionInitial: expected 0, got %d", IndicatorTypeVersionInitial.Number())
	}
	if IndicatorTypeVersionCommitted.Number() != 1 {
		t.Errorf("IndicatorTypeVersionCommitted: expected 1, got %d", IndicatorTypeVersionCommitted.Number())
	}
}

func TestIndicatorTypeVersion_Next(t *testing.T) {
	v, _ := NewIndicatorTypeVersion(IndicatorTypeVersionWithNumber(5))
	if v.Next().Number() != 6 {
		t.Errorf("Next(): expected 6, got %d", v.Next().Number())
	}
	if IndicatorTypeVersionInitial.Next() != IndicatorTypeVersionCommitted {
		t.Errorf("IndicatorTypeVersionInitial.Next() should equal IndicatorTypeVersionCommitted")
	}
}

func TestIndicatorTypeVersion_String(t *testing.T) {
	cases := []struct {
		v    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
	}
	for _, c := range cases {
		got, _ := NewIndicatorTypeVersion(IndicatorTypeVersionWithNumber(c.v))
		if got.String() != c.want {
			t.Errorf("String(): expected %q, got %q", c.want, got.String())
		}
	}
}
