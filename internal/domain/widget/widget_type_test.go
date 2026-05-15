package widget

import "testing"

func makeTestWidgetType(t *testing.T) WidgetType {
	t.Helper()
	id := NewWidgetTypeID()
	name, _ := NewWidgetTypeName("gauge")
	htmlTemplate, _ := NewHtmlTemplate("<div class='gauge'></div>")
	script, _ := NewScript("function render(value) { return value; }")
	lang := ScriptLanguageJavaScript
	wt, err := NewWidgetType(id, name, htmlTemplate, script, lang, WidgetTypeVersionInitial)
	if err != nil {
		t.Fatalf("NewWidgetType returned unexpected error: %v", err)
	}
	return wt
}

func TestNewWidgetType_FieldsAreSet(t *testing.T) {
	id := NewWidgetTypeID()
	name, _ := NewWidgetTypeName("thermometer")
	htmlTemplate, _ := NewHtmlTemplate("<div class='thermometer'></div>")
	script, _ := NewScript("function draw() {}")
	lang := ScriptLanguagePython
	version, _ := NewWidgetTypeVersion(WidgetTypeVersionWithNumber(3))

	wt, err := NewWidgetType(id, name, htmlTemplate, script, lang, version)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wt.ID() != id {
		t.Errorf("ID mismatch: expected %v, got %v", id, wt.ID())
	}
	if wt.Name() != name {
		t.Errorf("Name mismatch: expected %v, got %v", name, wt.Name())
	}
	if wt.HtmlTemplate() != htmlTemplate {
		t.Errorf("HtmlTemplate mismatch: expected %v, got %v", htmlTemplate, wt.HtmlTemplate())
	}
	if wt.Script() != script {
		t.Errorf("Script mismatch: expected %v, got %v", script, wt.Script())
	}
	if wt.ScriptLanguage() != lang {
		t.Errorf("ScriptLanguage mismatch: expected %v, got %v", lang, wt.ScriptLanguage())
	}
	if wt.Version() != version {
		t.Errorf("Version mismatch: expected %d, got %d", version, wt.Version())
	}
}

func TestNewWidgetType_InitialVersion(t *testing.T) {
	wt := makeTestWidgetType(t)
	if wt.Version() != WidgetTypeVersionInitial {
		t.Errorf("expected initial version %d, got %d", WidgetTypeVersionInitial, wt.Version())
	}
}

func TestWidgetType_IncrementVersion(t *testing.T) {
	wt := makeTestWidgetType(t)
	wt.IncrementVersion()
	if wt.Version().Number() != 1 {
		t.Errorf("expected version 1 after first increment, got %d", wt.Version().Number())
	}
	wt.IncrementVersion()
	if wt.Version().Number() != 2 {
		t.Errorf("expected version 2 after second increment, got %d", wt.Version().Number())
	}
}

func TestNewWidgetType_String(t *testing.T) {
	wt := makeTestWidgetType(t)
	s := wt.String()
	if s == "" {
		t.Error("expected non-empty String() output")
	}
}
