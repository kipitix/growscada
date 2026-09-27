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

func TestNewScriptLanguage_UnrecognisedValue_ReturnsErrorAndInvalid(t *testing.T) {
	for _, input := range []string{"ruby", "unknown", ""} {
		t.Run(input, func(t *testing.T) {
			lang, err := NewScriptLanguage(input)
			if err == nil {
				t.Errorf("expected error for %q, got nil", input)
			}
			if lang.IsValid() {
				t.Errorf("expected invalid zero value for %q, got %v", input, lang)
			}
		})
	}
}

func TestScriptLanguage_String(t *testing.T) {
	cases := []struct {
		lang     ScriptLanguage
		expected string
	}{
		{ScriptLanguage{}, "invalid"},
		{ScriptLanguageJavaScript, "javascript"},
		{ScriptLanguagePython, "python"},
		{ScriptLanguageLua, "lua"},
		{ScriptLanguage{language: 99}, "invalid"},
	}

	for _, tc := range cases {
		t.Run(tc.expected, func(t *testing.T) {
			if got := tc.lang.String(); got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}
