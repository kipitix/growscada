package widget

import (
	"testing"

	"github.com/kipitix/growscada/internal/domain/version"
)

func makeTestWidgetType(t *testing.T) WidgetType {
	t.Helper()
	id := NewWidgetTypeID()
	name, _ := NewWidgetTypeName("gauge")
	htmlTemplate, _ := NewHtmlTemplate("<div class='gauge'></div>")
	script, _ := NewScript("function render(value) { return value; }")
	lang := ScriptLanguageJavaScript
	wt, err := NewWidgetType(id, name, htmlTemplate, script, lang, version.Initial)
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
	version, _ := version.New(version.WithNumber(3))

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
	if wt.Version() != version.Initial {
		t.Errorf("expected initial version %d, got %d", version.Initial, wt.Version())
	}
}

func TestNewWidgetType_String(t *testing.T) {
	wt := makeTestWidgetType(t)
	if wt.String() == "" {
		t.Error("expected non-empty String() output")
	}
}
