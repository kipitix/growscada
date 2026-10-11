package outbox

import (
	"testing"
	"time"
)

func TestPolicyRunner_PauseDoublesUpToTheMaximum(t *testing.T) {
	r := NewPolicyRunner(nil, nil)
	want := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second,
		32 * time.Second, 64 * time.Second, 128 * time.Second, 256 * time.Second, 5 * time.Minute, 5 * time.Minute,
	}
	for i, w := range want {
		if got := r.pauseAfter(i + 1); got != w {
			t.Errorf("after %d failures: pause %s, want %s", i+1, got, w)
		}
	}
	if got := r.pauseAfter(1000); got != 5*time.Minute {
		t.Errorf("after 1000 failures: pause %s, want 5m", got)
	}
}
