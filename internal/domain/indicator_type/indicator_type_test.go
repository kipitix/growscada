package indicator_type

import "testing"

func makeTestIndicatorType(t *testing.T) IndicatorType {
	t.Helper()
	id := NewIndicatorTypeID()
	name, _ := NewIndicatorTypeName("gauge")
	svgTemplate, _ := NewSvgTemplate("<svg><circle r='10'/></svg>")
	script, _ := NewScript("function render(value) { return value; }")
	lang := ScriptLanguageJavaScript
	it, err := NewIndicatorType(id, name, svgTemplate, script, lang, IndicatorTypeVersionInitial)
	if err != nil {
		t.Fatalf("NewIndicatorType returned unexpected error: %v", err)
	}
	return it
}

func TestNewIndicatorType_FieldsAreSet(t *testing.T) {
	id := NewIndicatorTypeID()
	name, _ := NewIndicatorTypeName("thermometer")
	svgTemplate, _ := NewSvgTemplate("<svg><rect/></svg>")
	script, _ := NewScript("function draw() {}")
	lang := ScriptLanguagePython
	version := IndicatorTypeVersion(3)

	it, err := NewIndicatorType(id, name, svgTemplate, script, lang, version)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if it.ID() != id {
		t.Errorf("ID mismatch: expected %v, got %v", id, it.ID())
	}
	if it.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, it.Name())
	}
	if it.SvgTemplate() != svgTemplate {
		t.Errorf("SvgTemplate mismatch: expected %v, got %v", svgTemplate, it.SvgTemplate())
	}
	if it.Script() != script {
		t.Errorf("Script mismatch: expected %v, got %v", script, it.Script())
	}
	if it.ScriptLanguage() != lang {
		t.Errorf("ScriptLanguage mismatch: expected %v, got %v", lang, it.ScriptLanguage())
	}
	if it.Version() != version {
		t.Errorf("Version mismatch: expected %d, got %d", version, it.Version())
	}
}

func TestNewIndicatorType_InitialVersion(t *testing.T) {
	it := makeTestIndicatorType(t)
	if it.Version() != IndicatorTypeVersionInitial {
		t.Errorf("expected initial version %d, got %d", IndicatorTypeVersionInitial, it.Version())
	}
}

func TestIndicatorType_IncrementVersion(t *testing.T) {
	it := makeTestIndicatorType(t)
	it.IncrementVersion()
	if it.Version() != 1 {
		t.Errorf("expected version 1 after first increment, got %d", it.Version())
	}
	it.IncrementVersion()
	if it.Version() != 2 {
		t.Errorf("expected version 2 after second increment, got %d", it.Version())
	}
}

func TestNewIndicatorType_String(t *testing.T) {
	it := makeTestIndicatorType(t)
	s := it.String()
	if s == "" {
		t.Error("expected non-empty String() output")
	}
}
