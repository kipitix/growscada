package tag

import "testing"

func makeTestTag(t *testing.T) Tag {
	t.Helper()
	id := NewTagID()
	name, _ := NewTagName("temperature")
	tagType := TagTypeInteger
	value, _ := tagType.NewTagValue(0)
	quality := TagQualityGood
	tag, err := NewTag(id, name, tagType, value, quality, TagVersionInitial)
	if err != nil {
		t.Fatalf("NewTag returned unexpected error: %v", err)
	}
	return tag
}

func TestNewTag_FieldsAreSet(t *testing.T) {
	id := NewTagID()
	name, _ := NewTagName("pressure")
	tagType := TagTypeString
	value, _ := tagType.NewTagValue("100")
	quality := TagQualityGood
	version := TagVersion(3)

	tag, err := NewTag(id, name, tagType, value, quality, version)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tag.ID() != id {
		t.Errorf("ID mismatch: expected %v, got %v", id, tag.ID())
	}
	if tag.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, tag.Name())
	}
	if tag.Type() != tagType {
		t.Errorf("Type mismatch: expected %v, got %v", tagType, tag.Type())
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
	tag := makeTestTag(t) // TagTypeInteger

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
	// Function makeTestTag makes tag with TagTypeInteger.
	// 3.14 is not a valid integer.
	err := tag.SetValue(3.14, TagQualityBad)
	if err == nil {
		t.Error("expected error for unsupported value type, got nil")
	}

	// Verify quality remains unchanged.
	if tag.Quality() != originalQuality {
		t.Errorf("expected quality %v, got %v", originalQuality, tag.Quality())
	}

	// Verify value remains unchanged.
	if tag.Value().Value() != 0 {
		t.Errorf("expected value 0, got %v", tag.Value().Value())
	}
}
