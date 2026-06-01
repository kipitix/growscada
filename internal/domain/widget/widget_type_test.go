package widget

import (
	"testing"

	"github.com/kipitix/growscada/internal/domain/id"
	"github.com/kipitix/growscada/internal/domain/version"
)

func mustWidgetTypeID(t *testing.T) id.ID[WidgetType] {
	t.Helper()
	return id.NewID[WidgetType]()
}

func mustWidgetTypeVersion(t *testing.T) version.Version[WidgetType] {
	t.Helper()
	return version.Initial[WidgetType]()
}

func makeTestWidgetType(t *testing.T) WidgetType {
	t.Helper()
	anID := id.NewID[WidgetType]()
	name, _ := NewWidgetTypeName("gauge")
	htmlTemplate, _ := NewHtmlTemplate("<div class='gauge'></div>")
	script, _ := NewScript("function render(value) { return value; }")
	lang := ScriptLanguageJavaScript
	wt, err := NewWidgetType(anID, name, htmlTemplate, script, lang, DefaultSize(), nil, version.Initial[WidgetType]())
	if err != nil {
		t.Fatalf("makeTestWidgetType: %v", err)
	}
	return wt
}

func TestNewWidgetType_FieldsAreSet(t *testing.T) {
	anID := id.NewID[WidgetType]()
	name, _ := NewWidgetTypeName("thermometer")
	htmlTemplate, _ := NewHtmlTemplate("<div class='thermometer'></div>")
	script, _ := NewScript("function draw() {}")
	lang := ScriptLanguagePython
	size, _ := NewSize(120, 80)
	ver, _ := version.New[WidgetType](version.WithNumber[WidgetType](3))

	wt, err := NewWidgetType(anID, name, htmlTemplate, script, lang, size, nil, ver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if wt.ID() != anID {
		t.Errorf("ID mismatch: expected %v, got %v", anID, wt.ID())
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
	if wt.DefaultSize() != size {
		t.Errorf("DefaultSize mismatch: expected %v, got %v", size, wt.DefaultSize())
	}
	if wt.Version() != ver {
		t.Errorf("Version mismatch: expected %d, got %d", ver, wt.Version())
	}
}

func TestNewWidgetType_InitialVersion(t *testing.T) {
	wt := makeTestWidgetType(t)
	if wt.Version() != version.Initial[WidgetType]() {
		t.Errorf("expected initial version %d, got %d", version.Initial[WidgetType](), wt.Version())
	}
}

func TestNewWidgetType_String(t *testing.T) {
	wt := makeTestWidgetType(t)
	if wt.String() == "" {
		t.Error("expected non-empty String() output")
	}
}
