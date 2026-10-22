package lease

import (
	"testing"
	"time"
)

func TestWrongAnswerDoesNotCount(t *testing.T) {
	var s Session
	if s.Accept(false) {
		t.Fatal("wrong answer finished the quiz")
	}
	if s.Correct != 0 {
		t.Fatalf("count = %d", s.Correct)
	}
	if s.Accept(true) {
		t.Fatal("one correct answer finished the quiz")
	}
	if s.Correct != 1 {
		t.Fatalf("count = %d", s.Correct)
	}
}

func TestQuitResets(t *testing.T) {
	s := &Session{Correct: 10}
	s.Reset()
	if s.Correct != 0 {
		t.Fatalf("count = %d", s.Correct)
	}
	var done bool
	for i := 0; i < QuizLength; i++ {
		done = s.Accept(true)
	}
	if !done || s.Correct != QuizLength {
		t.Fatalf("done=%v count=%d", done, s.Correct)
	}
}

func TestFinishesOnlyAtQuizLength(t *testing.T) {
	var s Session
	for i := 0; i < QuizLength-1; i++ {
		if s.Accept(true) {
			t.Fatalf("finished at %d", s.Correct)
		}
	}
	if !s.Accept(true) {
		t.Fatal("expected the 50th correct answer to finish")
	}
}

func TestParseGrant(t *testing.T) {
	got, capped, err := ParseGrant("15m")
	if err != nil || capped || got != 15*time.Minute {
		t.Fatalf("15m got %s capped %v err %v", got, capped, err)
	}
	got, capped, err = ParseGrant("2h")
	if err != nil || !capped || got != MaxGrant {
		t.Fatalf("2h got %s capped %v err %v", got, capped, err)
	}
	got, capped, err = ParseGrant("30m")
	if err != nil || capped || got != MaxGrant {
		t.Fatalf("30m got %s capped %v err %v", got, capped, err)
	}
	for _, raw := range []string{"0s", "-1m", "15", "tomorrow"} {
		if _, _, err := ParseGrant(raw); err == nil {
			t.Errorf("expected error for %q", raw)
		}
	}
}

func TestLeaseExpiry(t *testing.T) {
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	until := now.Add(15 * time.Minute)
	if !Active(until, now) {
		t.Fatal("expected an active lease")
	}
	if Active(until, until) {
		t.Fatal("lease should be closed at the deadline")
	}
	if Active(until, until.Add(time.Second)) {
		t.Fatal("lease should be expired")
	}
	if Active(time.Time{}, now) {
		t.Fatal("empty lease should be inactive")
	}
}
