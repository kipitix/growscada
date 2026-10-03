package livescene

import "github.com/kipitix/growscada/internal/server/interface/ui/uiutil"

// boundTag returns the Tag bound to the named port of the Widget, if the port
// has a PortBinding and its Tag still exists.
func boundTag(w Widget, port string, tags map[string]Tag) (Tag, bool) {
	for _, b := range w.PortBindings {
		if b.PortName == port {
			t, ok := tags[b.TagID]
			return t, ok
		}
	}
	return Tag{}, false
}

// SceneTags returns the Tags bound to the Widgets' PortBindings: all a View
// of these Widgets reads. Given to the View instead of every Tag, it keeps the
// View from re-rendering when a Tag shown elsewhere changes.
func SceneTags(widgets []Widget, tags map[string]Tag) map[string]Tag {
	bound := make(map[string]Tag)
	for _, w := range widgets {
		for _, b := range w.PortBindings {
			if t, ok := tags[b.TagID]; ok {
				bound[b.TagID] = t
			}
		}
	}
	return bound
}

// WidgetInputs computes the `inputs` of the Widget's render(inputs): each
// InputPort gets its bound Tag's current value, converted by the Tag's
// TagType. A port without a PortBinding, or bound to a deleted Tag, gets its
// type hint's default.
func WidgetInputs(w Widget, wt WidgetType, tags map[string]Tag) uiutil.Inputs {
	inputs := make(uiutil.Inputs, len(wt.InputPorts))
	for _, p := range wt.InputPorts {
		if t, ok := boundTag(w, p.Name, tags); ok {
			inputs[p.Name] = uiutil.NewInput(t.Value, t.Type)
		} else {
			inputs[p.Name] = uiutil.NewInput("", p.TypeHint)
		}
	}
	return inputs
}

// WidgetQuality is the Quality Operation marks the Widget with: the worst
// Quality among its InputPorts. A port without a PortBinding, or bound to a
// deleted Tag, counts as Bad, so an unfinished Widget stands out. While the
// connection to the server is lost, no value can be trusted to be current, so
// the result is at least Uncertain.
func WidgetQuality(w Widget, wt WidgetType, tags map[string]Tag, connected bool) Quality {
	q := QualityGood
	if !connected {
		q = QualityUncertain
	}
	for _, p := range wt.InputPorts {
		if t, ok := boundTag(w, p.Name, tags); ok {
			q = Worst(q, t.Quality)
		} else {
			q = QualityBad
		}
	}
	return q
}
