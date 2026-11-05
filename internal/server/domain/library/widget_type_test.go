package library

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/id"
)

func mustWidgetTypeID(t *testing.T) id.ID[WidgetType] {
	t.Helper()
	return id.NewID[WidgetType]()
}

func makeTestWidgetType(t *testing.T) WidgetType {
	t.Helper()
	anID := id.NewID[WidgetType]()
	name, _ := NewWidgetTypeName("gauge")
	htmlTemplate, _ := NewHtmlTemplate("<div class='gauge'></div>")
	script, _ := NewScript("function render(value) { return value; }")
	lang := ScriptLanguageJavaScript
	wt, err := ReconstituteWidgetType(anID, testLibraryID, name, htmlTemplate, script, lang, DefaultSize(), nil)
	if err != nil {
		t.Fatalf("makeTestWidgetType: %v", err)
	}
	return wt
}

func TestReconstituteWidgetType_FieldsAreSet(t *testing.T) {
	anID := id.NewID[WidgetType]()
	name, _ := NewWidgetTypeName("thermometer")
	htmlTemplate, _ := NewHtmlTemplate("<div class='thermometer'></div>")
	script, _ := NewScript("function draw() {}")
	lang := ScriptLanguagePython
	size, _ := NewSize(120, 80)

	wt, err := ReconstituteWidgetType(anID, testLibraryID, name, htmlTemplate, script, lang, size, nil)
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
	if wt.LibraryID() != testLibraryID {
		t.Errorf("LibraryID mismatch: expected %v, got %v", testLibraryID, wt.LibraryID())
	}
}

func TestReconstituteWidgetType_String(t *testing.T) {
	wt := makeTestWidgetType(t)
	if wt.String() == "" {
		t.Error("expected non-empty String() output")
	}
}

func TestReconstituteWidgetType_InvalidScriptLanguage_ReturnsError(t *testing.T) {
	name, _ := NewWidgetTypeName("gauge")
	htmlTemplate, _ := NewHtmlTemplate("<div></div>")
	script, _ := NewScript("function update(){}")

	_, err := ReconstituteWidgetType(id.NewID[WidgetType](), testLibraryID, name, htmlTemplate, script, ScriptLanguage{}, DefaultSize(), nil)
	if err == nil {
		t.Error("expected error for invalid script language, got nil")
	}
}

// testLibraryID is the Library of the tests' widget types.
var testLibraryID = id.NewID[Library]()
