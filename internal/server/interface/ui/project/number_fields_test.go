package project

import "testing"

func TestNumberFields_KeepsValuesTheServerJudges(t *testing.T) {
	var nf numberFields
	w := nf.int("width", "0")
	x := nf.float("origin X", "2.5")

	if nf.err != nil {
		t.Fatalf("err: expected nil, got %v", nf.err)
	}
	if w != 0 || x != 2.5 {
		t.Errorf("values: expected 0 and 2.5 unchanged, got %d and %v", w, x)
	}
}

func TestNumberFields_ReportsFirstFieldThatIsNotANumber(t *testing.T) {
	var nf numberFields
	nf.int("width", "")
	nf.float("origin X", "abc")

	want := `width: "" is not a whole number`
	if nf.err == nil || nf.err.Error() != want {
		t.Errorf("err: expected %q, got %v", want, nf.err)
	}
}
