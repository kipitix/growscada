package widget

import "testing"

func TestNewScriptLanguage_ValidValues(t *testing.T) {
	cases := []struct {
		input    string
		expected ScriptLanguage
	}{
		{"javascript", ScriptLanguageJavaScript},
		{"python", ScriptLanguagePython},
		{"lua", ScriptLanguageLua},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			lang, err := NewScriptLanguage(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if lang != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, lang)
			}
		})
	}
}

func TestNewScriptLanguage_UnknownValue(t *testing.T) {
	_, err := NewScriptLanguage("ruby")
	if err == nil {
		t.Error("expected error for unknown language, got nil")
	}
}

func TestScriptLanguage_String(t *testing.T) {
	cases := []struct {
		lang     ScriptLanguage
		expected string
	}{
		{ScriptLanguageUnknown, "unknown"},
		{ScriptLanguageJavaScript, "javascript"},
		{ScriptLanguagePython, "python"},
		{ScriptLanguageLua, "lua"},
		{ScriptLanguage{language: 99}, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			if got := tc.lang.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
