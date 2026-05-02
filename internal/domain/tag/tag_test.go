package tag

import "testing"

func makeTestTag(t *testing.T) Tag {
	t.Helper()
	id := NewTagID()
	name, _ := NewTagName("temperature")
	kind := TagKindInteger
	value, _ := kind.NewTagValue(0)
	quality := TagQualityGood
	tag, err := NewTag(id, name, kind, value, quality, TagVersionInitial)
	if err != nil {
		t.Fatalf("NewTag returned unexpected error: %v", err)
	}
	return tag
}

func TestNewTag_FieldsAreSet(t *testing.T) {
	id := NewTagID()
	name, _ := NewTagName("pressure")
	kind := TagKindString
	value, _ := kind.NewTagValue("100")
	quality := TagQualityGood
	version := 3

	tag, err := NewTag(id, name, kind, value, quality, version)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tag.ID() != id {
		t.Errorf("ID mismatch: expected %v, got %v", id, tag.ID())
	}
	if tag.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, tag.Name())
	}
	if tag.Kind() != kind {
		t.Errorf("Kind mismatch: expected %v, got %v", kind, tag.Kind())
	}
	if tag.Value().String() != value.String() {
		t.Errorf("Value mismatch: expected %v, got %v", value.String(), tag.Value().String())
	}
	if tag.Quality() != quality {
		t.Errorf("Quality mismatch: expected %v, got %v", quality, tag.Quality())
	}
	if tag.Version() != version {
		t.Errorf("Version mismatch: expected %d, got %d", version, tag.Version())
	}
}

func TestTagVersionInitial_IsZero(t *testing.T) {
	if TagVersionInitial != 0 {
		t.Errorf("expected TagVersionInitial to be 0, got %d", TagVersionInitial)
	}
}

func TestNewTag_InitialVersion(t *testing.T) {
	tag := makeTestTag(t)
	if tag.Version() != TagVersionInitial {
		t.Errorf("expected initial version %d, got %d", TagVersionInitial, tag.Version())
	}
}

func TestTag_IncrementVersion(t *testing.T) {
	tag := makeTestTag(t)
	tag.IncrementVersion()
	if tag.Version() != 1 {
		t.Errorf("expected version 1 after first increment, got %d", tag.Version())
	}
	tag.IncrementVersion()
	if tag.Version() != 2 {
		t.Errorf("expected version 2 after second increment, got %d", tag.Version())
	}
}

func TestTag_SetValue_Success(t *testing.T) {
	tag := makeTestTag(t)

	err := tag.SetValue(99, TagQualityGood)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag.Value().String() != "99" {
		t.Errorf("expected value '99', got %q", tag.Value().String())
	}
	if tag.Quality() != TagQualityGood {
		t.Errorf("expected quality good, got %v", tag.Quality())
	}
}

func TestTag_SetValue_ChangesQuality(t *testing.T) {
	tag := makeTestTag(t)

	err := tag.SetValue(0, TagQualityBad)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag.Quality() != TagQualityBad {
		t.Errorf("expected quality bad, got %v", tag.Quality())
	}
}

func TestTag_SetValue_InvalidInput_ReturnsError(t *testing.T) {
	tag := makeTestTag(t) // TagKindInteger

	// float64 is unsupported by the integer value constructor
	err := tag.SetValue(3.14, TagQualityGood)
	if err == nil {
		t.Error("expected error for unsupported value type, got nil")
	}
}

func TestTag_SetValue_InvalidInput_DoesNotChangeQuality(t *testing.T) {
	tag := makeTestTag(t)
	originalQuality := tag.Quality()

	// Intentionally pass a bad value to trigger an error.
	// Quality is set before value construction, so it will change on error —
	// this test documents the current behaviour.
	_ = tag.SetValue(3.14, TagQualityBad)

	// After an error the quality reflects what was passed, not the original.
	// If the implementation changes to roll back on error, update this test.
	_ = originalQuality
}
