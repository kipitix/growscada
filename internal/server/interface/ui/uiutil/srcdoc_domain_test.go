package uiutil

import (
	"testing"

	"github.com/kipitix/growscada/internal/server/domain/tag"
)

// TestNewInput_MatchesDomain: NewInput accepts the same texts as the server's
// TagValues and gives the text in the form the server keeps it in. The UI
// does not import the domain, so this test is what keeps the two in step.
func TestNewInput_MatchesDomain(t *testing.T) {
	texts := []string{
		"0", "42", "+007", "-0", "-7", "007", " 1", "1.5", "1e3", "0x10",
		"9223372036854775807", "9223372036854775808", "-9223372036854775808",
		"true", "false", "TRUE", "True", "1", "yes", "", "hi",
	}
	for _, typeName := range []string{"integer", "boolean", "string"} {
		typ, err := tag.NewTagType(typeName)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range texts {
			value, err := typ.NewTagValue(text)
			in, ok := NewInput(text, typeName)
			if ok != (err == nil) {
				t.Errorf("%s %q: NewInput ok = %v, domain err = %v", typeName, text, ok, err)
				continue
			}
			if ok && *in.Text != value.String() {
				t.Errorf("%s %q: NewInput text = %q, domain = %q", typeName, text, *in.Text, value.String())
			}
		}
	}
}
