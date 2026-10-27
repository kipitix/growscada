package librarypin

import (
	"errors"
	"fmt"

	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/library"
	"github.com/kipitix/growscada/internal/server/domain/scene"
)

// ErrWidgetsOrphaned matches the error of a switch or removal of a pin that
// would leave Widgets of the Project without their WidgetType (ADR 0012);
// the error is an OrphansError naming them.
var ErrWidgetsOrphaned = errors.New("widgets would be left without their widget type")

// OrphansError refuses a switch or removal of a pin: the Widgets it names
// would be left without their WidgetType.
type OrphansError struct {
	Widgets []PlacedWidget
}

func (e OrphansError) Error() string {
	return fmt.Sprintf("%v: %d widgets", ErrWidgetsOrphaned, len(e.Widgets))
}

func (e OrphansError) Is(target error) bool { return target == ErrWidgetsOrphaned }

// PlacedWidget is a Widget of a Project with the Scene that holds it.
type PlacedWidget struct {
	SceneID id.ID[scene.Scene]
	Widget  scene.Widget
}

// BrokenBinding is a PortBinding of a Widget that a switch makes invalid: its
// port is no longer declared, or its type hint no longer accepts the Tag's
// TagType.
type BrokenBinding struct {
	PlacedWidget
	Binding scene.PortBinding
}

// Impact is what switching or removing a pin would do to the Project's
// Widgets: those left without their WidgetType, which refuse it, and the
// PortBindings it makes invalid, which a policy removes after it.
type Impact struct {
	Orphaned []PlacedWidget
	Broken   []BrokenBinding
}

// Check returns an OrphansError if the change would orphan Widgets.
func (i Impact) Check() error {
	if len(i.Orphaned) > 0 {
		return OrphansError{Widgets: i.Orphaned}
	}
	return nil
}

// ImpactOf computes the Impact on widgets, the Widgets of a Project, of
// pinning to instead of from, two Library Releases of one Library: from is
// the release pinned now (nil if none), to the one pinned after (nil for
// removing the pin). A Widget of a WidgetType from holds and to does not is
// orphaned; a PortBinding of a Widget whose WidgetType both hold is broken
// if to's copy of it does not declare the port, or the port's type hint does
// not accept the TagType of the bound Tag as tagTypes, the TagTypes of the
// Project's Tags, has it. A binding to a Tag that does not exist is no
// matter of the pin: it is removed whatever the pin.
// It is both the preview of a switch and the check that refuses it.
func ImpactOf(widgets []PlacedWidget, from, to library.LibraryRelease, tagTypes scene.TagTypes) Impact {
	var impact Impact
	if from == nil {
		return impact
	}
	for _, pw := range widgets {
		if _, ok := from.FindWidgetType(pw.Widget.TypeID()); !ok {
			continue
		}
		var released library.ReleasedWidgetType
		found := false
		if to != nil {
			released, found = to.FindWidgetType(pw.Widget.TypeID())
		}
		if !found {
			impact.Orphaned = append(impact.Orphaned, pw)
			continue
		}
		ports := make(map[library.InputPortName]library.InputPort)
		for _, p := range released.InputPorts() {
			ports[p.Name()] = p
		}
		for _, b := range pw.Widget.PortBindings() {
			port, declared := ports[b.PortName()]
			tagType, tagExists := tagTypes.Of(b.TagID())
			if !declared || (tagExists && !port.TypeHint().Accepts(tagType)) {
				impact.Broken = append(impact.Broken, BrokenBinding{PlacedWidget: pw, Binding: b})
			}
		}
	}
	return impact
}
