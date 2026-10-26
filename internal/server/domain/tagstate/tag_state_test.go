package tagstate

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/project"
	"github.com/kipitix/growscada/internal/server/domain/tag"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func makeDefinition(t *testing.T, tagType tag.TagType) Definition {
	t.Helper()
	name, err := tag.NewTagName("speed")
	if err != nil {
		t.Fatal(err)
	}
	return Definition{
		TagID: id.NewID[tag.Tag](), ProjectID: id.NewID[project.Project](),
		Name: name, Type: tagType, Version: version.Committed[tag.Tag](),
	}
}

func TestInitial_IsTheZeroValueWithQualityBad(t *testing.T) {
	for tagType, want := range map[tag.TagType]string{tag.TagTypeString: "", tag.TagTypeBoolean: "false", tag.TagTypeInteger: "0"} {
		s, err := Initial(makeDefinition(t, tagType))
		if err != nil {
			t.Fatalf("%s: %v", tagType, err)
		}
		if s.Value().String() != want || s.Quality() != tag.TagQualityBad || s.Seq() != 0 {
			t.Errorf("%s: initial state %s, want %q, bad, no seq", tagType, s, want)
		}
	}
	if _, err := Initial(makeDefinition(t, tag.TagType{})); err == nil {
		t.Error("a state of the invalid TagType")
	}
}

func TestWrite_ParsesTheValueForTheTagType(t *testing.T) {
	initial, _ := Initial(makeDefinition(t, tag.TagTypeInteger))
	written, err := initial.Write("42", tag.TagQualityGood)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if written.Value().Value() != int64(42) || written.Quality() != tag.TagQualityGood {
		t.Errorf("written %s, want 42 good", written)
	}
	if initial.Value().Value() != int64(0) || initial.Quality() != tag.TagQualityBad {
		t.Errorf("Write changed the state it was called on: %s", initial)
	}
	if written.Definition() != initial.Definition() {
		t.Errorf("Write changed the definition: %v", written.Definition())
	}
}

func TestWrite_RefusesWhatDoesNotFit(t *testing.T) {
	initial, _ := Initial(makeDefinition(t, tag.TagTypeInteger))
	if _, err := initial.Write("fast", tag.TagQualityGood); err == nil {
		t.Error("a value not of the TagType was written")
	}
	if _, err := initial.Write("1", tag.TagQuality{}); err == nil {
		t.Error("the invalid Quality was written")
	}
}

func TestReconstitute_RefusesAnInvalidState(t *testing.T) {
	def := makeDefinition(t, tag.TagTypeInteger)
	if _, err := Reconstitute(def, nil, tag.TagQualityGood, 1); err == nil {
		t.Error("a state without a value")
	}
	if _, err := Reconstitute(def, tag.TagValueInteger(1), tag.TagQuality{}, 1); err == nil {
		t.Error("a state of the invalid Quality")
	}
	s, err := Reconstitute(def, tag.TagValueInteger(1), tag.TagQualityUncertain, 7)
	if err != nil || s.Seq() != 7 || s.TagID() != def.TagID || s.ProjectID() != def.ProjectID {
		t.Errorf("Reconstitute = %s, %v", s, err)
	}
}

func TestTagUpdatedEvent_IsATagEventOfTheProject(t *testing.T) {
	s, _ := Initial(makeDefinition(t, tag.TagTypeBoolean))
	var e event.Event = NewTagUpdatedEvent(s)
	tagEvent, ok := e.(tag.TagEvent)
	if !ok {
		t.Fatalf("%T is no tag.TagEvent", e)
	}
	if e.Type() != event.EventTypeTagUpdated || tagEvent.TagID() != s.TagID() || tagEvent.ProjectID() != s.ProjectID() {
		t.Errorf("event %s of tag %s, project %s", e.Type(), tagEvent.TagID(), tagEvent.ProjectID())
	}
	if _, isProjectEvent := e.(project.ProjectEvent); isProjectEvent {
		t.Error("a content event is also a project event")
	}
}
