package widget

import (
	"errors"
	"testing"

	"github.com/kipitix/growscada/internal/domain/tag"
)

// ── InputPortName ─────────────────────────────────────────────────────────────

func TestNewInputPortName_ValidIdentifiers(t *testing.T) {
	valid := []string{"temperature", "_val", "$price", "port0", "camelCase", "a", "_", "$"}
	for _, s := range valid {
		if _, err := NewInputPortName(s); err != nil {
			t.Errorf("expected %q to be valid, got error: %v", s, err)
		}
	}
}

func TestNewInputPortName_EmptyString(t *testing.T) {
	_, err := NewInputPortName("")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestNewInputPortName_InvalidIdentifiers(t *testing.T) {
	invalid := []string{"0start", "has space", "has-dash", "has.dot", "with@at"}
	for _, s := range invalid {
		if _, err := NewInputPortName(s); err == nil {
			t.Errorf("expected %q to be invalid, got nil error", s)
		}
	}
}

func TestInputPortName_String(t *testing.T) {
	name, _ := NewInputPortName("temperature")
	if name.String() != "temperature" {
		t.Errorf("expected %q, got %q", "temperature", name.String())
	}
}

// ── InputPort ─────────────────────────────────────────────────────────────────

func TestNewInputPort_StoresAllFields(t *testing.T) {
	name, _ := NewInputPortName("pressure")
	port := NewInputPort(name, "Process pressure in bar", tag.TagTypeInteger)

	if port.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, port.Name())
	}
	if port.Description() != "Process pressure in bar" {
		t.Errorf("Description mismatch: expected %q, got %q", "Process pressure in bar", port.Description())
	}
	if port.TypeHint() != tag.TagTypeInteger {
		t.Errorf("TypeHint mismatch: expected %v, got %v", tag.TagTypeInteger, port.TypeHint())
	}
}

func TestNewInputPort_UnknownTypeHintIsZeroValue(t *testing.T) {
	name, _ := NewInputPortName("val")
	port := NewInputPort(name, "", tag.TagTypeUnknown)
	if port.TypeHint() != tag.TagTypeUnknown {
		t.Errorf("expected TagTypeUnknown, got %v", port.TypeHint())
	}
}

// ── NewWidgetType duplicate port names ────────────────────────────────────────

func TestNewWidgetType_DuplicateInputPortName(t *testing.T) {
	wtID := mustWidgetTypeID(t)
	name, _ := NewWidgetTypeName("gauge")
	html, _ := NewHtmlTemplate("<div></div>")
	script, _ := NewScript("function update(){}")
	size := DefaultSize()

	portName, _ := NewInputPortName("temperature")
	ports := []InputPort{
		NewInputPort(portName, "first", tag.TagTypeUnknown),
		NewInputPort(portName, "duplicate", tag.TagTypeUnknown),
	}

	_, err := NewWidgetType(wtID, name, html, script, ScriptLanguageJavaScript, size, ports, mustWidgetTypeVersion(t))
	if err == nil {
		t.Fatal("expected ErrDuplicateInputPortName, got nil")
	}
	if !errors.Is(err, ErrDuplicateInputPortName) {
		t.Errorf("expected ErrDuplicateInputPortName, got: %v", err)
	}
}

func TestNewWidgetType_UniqueInputPorts_OK(t *testing.T) {
	wtID := mustWidgetTypeID(t)
	name, _ := NewWidgetTypeName("gauge")
	html, _ := NewHtmlTemplate("<div></div>")
	script, _ := NewScript("function update(){}")
	size := DefaultSize()

	p1Name, _ := NewInputPortName("temperature")
	p2Name, _ := NewInputPortName("pressure")
	ports := []InputPort{
		NewInputPort(p1Name, "temp", tag.TagTypeInteger),
		NewInputPort(p2Name, "pres", tag.TagTypeInteger),
	}

	wt, err := NewWidgetType(wtID, name, html, script, ScriptLanguageJavaScript, size, ports, mustWidgetTypeVersion(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(wt.InputPorts()) != 2 {
		t.Errorf("expected 2 input ports, got %d", len(wt.InputPorts()))
	}
}

func TestNewWidgetType_EmptyInputPorts_OK(t *testing.T) {
	wt := makeTestWidgetType(t)
	if len(wt.InputPorts()) != 0 {
		t.Errorf("expected 0 input ports, got %d", len(wt.InputPorts()))
	}
}

func TestNewWidgetType_InputPortsCopied(t *testing.T) {
	wtID := mustWidgetTypeID(t)
	name, _ := NewWidgetTypeName("gauge")
	html, _ := NewHtmlTemplate("<div></div>")
	script, _ := NewScript("function update(){}")
	size := DefaultSize()

	pName, _ := NewInputPortName("val")
	ports := []InputPort{NewInputPort(pName, "", tag.TagTypeUnknown)}

	wt, _ := NewWidgetType(wtID, name, html, script, ScriptLanguageJavaScript, size, ports, mustWidgetTypeVersion(t))

	// Mutate original slice — should not affect the aggregate.
	ports[0] = NewInputPort(pName, "mutated", tag.TagTypeString)
	if wt.InputPorts()[0].Description() != "" {
		t.Error("NewWidgetType must copy the inputPorts slice; external mutation should not affect the aggregate")
	}
}
