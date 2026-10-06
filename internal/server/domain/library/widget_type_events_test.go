package library

import (
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/event"
	"github.com/kipitix/growscada/internal/server/domain/id"
	"github.com/kipitix/growscada/internal/server/domain/version"
)

func makeCommittedWidgetType(t *testing.T) WidgetType {
	t.Helper()
	name, _ := NewWidgetTypeName("gauge")
	html, _ := NewHtmlTemplate("<div></div>")
	script, _ := NewScript("")
	wt, err := ReconstituteWidgetType(id.NewID[WidgetType](), name, html, script, ScriptLanguageJavaScript, DefaultSize(), nil, version.Committed[WidgetType]())
	if err != nil {
		t.Fatalf("ReconstituteWidgetType: %v", err)
	}
	return wt
}

func pendingTypes(r event.Recorder) []event.EventType {
	var types []event.EventType
	for _, e := range r.PendingEvents() {
		types = append(types, e.Type())
	}
	return types
}

func TestCreateWidgetType_RecordsCreatedEventAtInitialVersion(t *testing.T) {
	name, _ := NewWidgetTypeName("gauge")
	html, _ := NewHtmlTemplate("<div></div>")
	script, _ := NewScript("")
	anID := id.NewID[WidgetType]()

	wt, err := CreateWidgetType(anID, name, html, script, ScriptLanguageJavaScript, DefaultSize(), nil)
	if err != nil {
		t.Fatalf("CreateWidgetType: %v", err)
	}
	if wt.Version() != version.Initial[WidgetType]() {
		t.Errorf("expected initial version, got %s", wt.Version())
	}
	events := wt.PendingEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 pending event, got %d", len(events))
	}
	created, ok := events[0].(WidgetTypeCreatedEvent)
	if !ok || created.WidgetTypeID() != anID || created.Type() != event.EventTypeWidgetTypeCreated {
		t.Errorf("expected WidgetTypeCreatedEvent for %s, got %v", anID, events[0])
	}
}

func TestReconstituteWidgetType_RecordsNothing(t *testing.T) {
	if got := makeCommittedWidgetType(t).PendingEvents(); len(got) != 0 {
		t.Errorf("expected no pending events, got %v", got)
	}
}

func TestWidgetType_Update_ChangesDefinitionAndRecordsUpdated(t *testing.T) {
	wt := makeCommittedWidgetType(t)
	name, _ := NewWidgetTypeName("thermometer")
	html, _ := NewHtmlTemplate("<svg></svg>")
	script, _ := NewScript("update()")
	size, _ := NewSize(40, 120)

	updated, err := wt.Update(wt.Version(), name, html, script, ScriptLanguagePython, size, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.ID() != wt.ID() || updated.Version() != wt.Version() {
		t.Errorf("Update must keep ID and Version, got %s/%s", updated.ID(), updated.Version())
	}
	if updated.Name() != name || updated.HtmlTemplate() != html || updated.Script() != script ||
		updated.ScriptLanguage() != ScriptLanguagePython || updated.DefaultSize() != size {
		t.Errorf("Update did not apply the definition: %s", updated)
	}
	if got := pendingTypes(updated); len(got) != 1 || got[0] != event.EventTypeWidgetTypeUpdated {
		t.Errorf("expected [widget_type_updated], got %v", got)
	}
	if len(wt.PendingEvents()) != 0 || wt.Name() == name {
		t.Error("Update changed the original widget type")
	}
}

func TestWidgetType_Update_StaleVersion_ReturnsConflict(t *testing.T) {
	wt := makeCommittedWidgetType(t)

	_, err := wt.Update(version.Initial[WidgetType](), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), nil)
	if !errors.Is(err, ErrWidgetTypeConflict) {
		t.Errorf("expected ErrWidgetTypeConflict, got %v", err)
	}
}

func TestWidgetType_Update_DuplicatePortNames_ReturnsInvalidWidgetType(t *testing.T) {
	wt := makeCommittedWidgetType(t)
	portName, _ := NewInputPortName("value")
	ports := []InputPort{
		NewInputPort(portName, "", AnyTypeHint()),
		NewInputPort(portName, "", AnyTypeHint()),
	}

	_, err := wt.Update(wt.Version(), wt.Name(), wt.HtmlTemplate(), wt.Script(), wt.ScriptLanguage(), wt.DefaultSize(), ports)
	if !errors.Is(err, ErrDuplicateInputPortName) || !errors.Is(err, ErrInvalidWidgetType) {
		t.Errorf("expected ErrInvalidWidgetType and ErrDuplicateInputPortName, got %v", err)
	}
	if errors.Is(err, ErrWidgetTypeConflict) {
		t.Errorf("an invalid definition is no conflict: %v", err)
	}
}

func TestWidgetType_Delete_RecordsDeletedAndKeepsOriginal(t *testing.T) {
	wt := makeCommittedWidgetType(t)

	deleted := wt.Delete()

	if got := pendingTypes(deleted); len(got) != 1 || got[0] != event.EventTypeWidgetTypeDeleted {
		t.Errorf("expected [widget_type_deleted], got %v", got)
	}
	if len(wt.PendingEvents()) != 0 {
		t.Error("Delete changed the original widget type")
	}
}
