package widget

import "testing"

func TestNewWidgetTypeVersion_ValidValues(t *testing.T) {
	cases := []int{0, 1, 100}
	for _, v := range cases {
		got, err := NewWidgetTypeVersion(WidgetTypeVersionWithNumber(v))
		if err != nil {
			t.Errorf("NewWidgetTypeVersion(%d): unexpected error: %v", v, err)
		}
		if got.Number() != v {
			t.Errorf("NewWidgetTypeVersion(%d): got %d", v, got.Number())
		}
	}
}

func TestNewWidgetTypeVersion_NegativeReturnsError(t *testing.T) {
	_, err := NewWidgetTypeVersion(WidgetTypeVersionWithNumber(-1))
	if err == nil {
		t.Error("expected error for negative version, got nil")
	}
}

func TestWidgetTypeVersion_Constants(t *testing.T) {
	if WidgetTypeVersionInitial.Number() != 0 {
		t.Errorf("WidgetTypeVersionInitial: expected 0, got %d", WidgetTypeVersionInitial.Number())
	}
	if WidgetTypeVersionCommitted.Number() != 1 {
		t.Errorf("WidgetTypeVersionCommitted: expected 1, got %d", WidgetTypeVersionCommitted.Number())
	}
}

func TestWidgetTypeVersion_Next(t *testing.T) {
	v, _ := NewWidgetTypeVersion(WidgetTypeVersionWithNumber(5))
	if v.Next().Number() != 6 {
		t.Errorf("Next(): expected 6, got %d", v.Next().Number())
	}
	if WidgetTypeVersionInitial.Next() != WidgetTypeVersionCommitted {
		t.Errorf("WidgetTypeVersionInitial.Next() should equal WidgetTypeVersionCommitted")
	}
}

func TestWidgetTypeVersion_String(t *testing.T) {
	cases := []struct {
		v    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
	}
	for _, c := range cases {
		got, _ := NewWidgetTypeVersion(WidgetTypeVersionWithNumber(c.v))
		if got.String() != c.want {
			t.Errorf("String(): expected %q, got %q", c.want, got.String())
		}
	}
}
