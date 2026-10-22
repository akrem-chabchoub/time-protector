package schedule

import (
	"testing"
	"time"
)

func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 22, hour, minute, 0, 0, time.UTC)
}

func TestBlocks(t *testing.T) {
	s := Default()
	cases := []struct {
		hour, minute int
		blocked      bool
	}{
		{7, 59, false},
		{8, 0, true},
		{10, 0, true},
		{11, 59, true},
		{12, 0, false},
		{12, 30, false},
		{12, 59, false},
		{13, 0, true},
		{17, 59, true},
		{18, 0, false},
		{23, 0, false},
	}
	for _, tc := range cases {
		got := s.Blocks(at(tc.hour, tc.minute))
		if got != tc.blocked {
			t.Errorf("%02d:%02d blocked=%v, want %v", tc.hour, tc.minute, got, tc.blocked)
		}
	}
}

func TestNextChange(t *testing.T) {
	s := Default()
	cases := []struct {
		hour, minute int
		want         time.Time
	}{
		{7, 0, at(8, 0)},
		{10, 0, at(12, 0)},
		{12, 30, at(13, 0)},
		{18, 0, time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)},
		{19, 0, time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		got := s.NextChange(at(tc.hour, tc.minute))
		if !got.Equal(tc.want) {
			t.Errorf("%02d:%02d next=%s, want %s", tc.hour, tc.minute, got, tc.want)
		}
	}
}

func TestParseWindow(t *testing.T) {
	w, err := ParseWindow("08:00-12:00")
	if err != nil {
		t.Fatal(err)
	}
	if w.Start != 8*60 || w.End != 12*60 {
		t.Fatalf("window %+v", w)
	}
	for _, raw := range []string{"08:00-08:00", "12:00-08:00", "8:00-12:00", "08:60-12:00", ""} {
		if _, err := ParseWindow(raw); err == nil {
			t.Errorf("expected error for %q", raw)
		}
	}
}

func TestEmptyDoesNotBlock(t *testing.T) {
	if (Schedule{}).Blocks(at(10, 0)) {
		t.Fatal("empty schedule should not block by itself")
	}
}
