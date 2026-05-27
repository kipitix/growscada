package tag

import (
	"testing"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
)

func makeTestTag(t *testing.T) Tag {
	t.Helper()
	tagID := id.NewID[Tag]()
	name, _ := NewTagName("temperature")
	tagType := TagTypeInteger
	value, _ := tagType.NewTagValue(0)
	quality := TagQualityGood
	tag, err := NewTag(tagID, name, tagType, value, quality, version.Initial[Tag]())
	if err != nil {
		t.Fatalf("NewTag returned unexpected error: %v", err)
	}
	return tag
}

func TestNewTag_FieldsAreSet(t *testing.T) {
	tagID := id.NewID[Tag]()
	name, _ := NewTagName("pressure")
	tagType := TagTypeString
	value, _ := tagType.NewTagValue("100")
	quality := TagQualityGood
	ver, err := version.New[Tag](version.WithNumber[Tag](3))
	if err != nil {
		t.Fatalf("unexpected error creating version: %v", err)
	}

	tag, err := NewTag(tagID, name, tagType, value, quality, ver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tag.ID() != tagID {
		t.Errorf("ID mismatch: expected %v, got %v", tagID, tag.ID())
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
	if tag.Version() != ver {
		t.Errorf("Version mismatch: expected %d, got %d", ver, tag.Version())
	}
}

func TestTagVersionInitial_IsZero(t *testing.T) {
	if version.Initial[Tag]().Number() != 0 {
		t.Errorf("expected version.Initial to be 0, got %d", version.Initial[Tag]().Number())
	}
}

func TestNewTag_InitialVersion(t *testing.T) {
	tag := makeTestTag(t)
	if tag.Version() != version.Initial[Tag]() {
		t.Errorf("expected initial version %d, got %d", version.Initial[Tag](), tag.Version())
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

	err := tag.SetValue(3.14, TagQualityGood)
	if err == nil {
		t.Error("expected error for unsupported value type, got nil")
	}
}

func TestTag_SetValue_InvalidInput_DoesNotChangeQuality(t *testing.T) {
	tag := makeTestTag(t)
	originalQuality := tag.Quality()

	err := tag.SetValue(3.14, TagQualityBad)
	if err == nil {
		t.Error("expected error for unsupported value type, got nil")
	}

	if tag.Quality() != originalQuality {
		t.Errorf("expected quality %v, got %v", originalQuality, tag.Quality())
	}

	if tag.Value().Value() != int64(0) {
		t.Errorf("expected value 0, got %v", tag.Value().Value())
	}
}
