package application

import (
	"errors"
	"testing"
)

var errTestConflict = errors.New("test conflict")

func TestRetryOnRace_LostEveryTime_IsRaceLostAndConflict(t *testing.T) {
	calls := 0
	err := retryOnRace(errTestConflict, func() error {
		calls++
		return errTestConflict
	})
	if calls != maxRaceAttempts {
		t.Errorf("calls: expected %d, got %d", maxRaceAttempts, calls)
	}
	if !errors.Is(err, ErrRaceLost) {
		t.Errorf("expected ErrRaceLost, got: %v", err)
	}
	if !errors.Is(err, errTestConflict) {
		t.Errorf("expected the conflict, got: %v", err)
	}
}

func TestRetryOnRace_WonOnRetry_IsNoError(t *testing.T) {
	calls := 0
	err := retryOnRace(errTestConflict, func() error {
		calls++
		if calls == 1 {
			return errTestConflict
		}
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls: expected 2, got %d", calls)
	}
}

func TestRetryOnRace_OtherError_IsReturnedAsIs(t *testing.T) {
	other := errors.New("other")
	calls := 0
	err := retryOnRace(errTestConflict, func() error {
		calls++
		return other
	})
	if err != other {
		t.Errorf("expected %v, got: %v", other, err)
	}
	if calls != 1 {
		t.Errorf("calls: expected 1, got %d", calls)
	}
}
