package operation

import (
	"testing"

	"github.com/google/uuid"

	projectv0 "github.com/kipitix/growscada/contract/project/v0"
)

// TestRevisionTags_MissingStateIsTheTypeZeroValue: a Tag of the Revision the
// Runtime node holds no state for shows its TagType's zero value with quality
// bad, not an empty string.
func TestRevisionTags_MissingStateIsTheTypeZeroValue(t *testing.T) {
	definitions := []projectv0.TagDefinition{
		{ID: uuid.New(), Name: "name", Type: "string"},
		{ID: uuid.New(), Name: "flag", Type: "boolean"},
		{ID: uuid.New(), Name: "count", Type: "integer"},
	}
	tags := revisionTags(definitions, nil)

	want := []string{"", "false", "0"}
	for i, tag := range tags {
		if tag.Value != want[i] {
			t.Errorf("%s value = %q, want %q", tag.Name, tag.Value, want[i])
		}
		if tag.Quality != "bad" {
			t.Errorf("%s quality = %q, want bad", tag.Name, tag.Quality)
		}
	}
}
