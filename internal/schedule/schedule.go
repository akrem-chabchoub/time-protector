// Package schedule decides which clock times the sites stay blocked.
package schedule

import (
	"fmt"
	"regexp"
	"time"
)

// Window is a block on a single day. End is exclusive, in minutes from midnight.
type Window struct {
	Start int
	End   int
}

// Schedule is the list of block windows. It applies every day.
type Schedule struct {
	Windows []Window
}

// Default is 08:00-12:00 and 13:00-18:00.
func Default() Schedule {
	return Schedule{Windows: []Window{{Start: 8 * 60, End: 12 * 60}, {Start: 13 * 60, End: 18 * 60}}}
}

// Blocks reports whether t falls inside a window. The check uses the clock in t's location.
func (s Schedule) Blocks(t time.Time) bool {
	m := t.Hour()*60 + t.Minute()
	for _, w := range s.Windows {
		if m >= w.Start && m < w.End {
			return true
		}
	}
	return false
}

// NextChange is the next minute a window starts or ends after t.
func (s Schedule) NextChange(t time.Time) time.Time {
	if len(s.Windows) == 0 {
		return time.Time{}
	}
	m := t.Hour()*60 + t.Minute()
	best := -1
	for _, w := range s.Windows {
		for _, edge := range []int{w.Start, w.End} {
			if edge > m && (best < 0 || edge < best) {
				best = edge
			}
		}
	}
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	if best >= 0 {
		return day.Add(time.Duration(best) * time.Minute)
	}
	earliest := s.Windows[0].Start
	for _, w := range s.Windows[1:] {
		if w.Start < earliest {
			earliest = w.Start
		}
	}
	return day.Add(24*time.Hour + time.Duration(earliest)*time.Minute)
}

var windowPattern = regexp.MustCompile(`^(\d{2}:\d{2})-(\d{2}:\d{2})$`)

// ParseWindow reads a window like 08:00-12:00.
func ParseWindow(raw string) (Window, error) {
	match := windowPattern.FindStringSubmatch(raw)
	if match == nil {
		return Window{}, fmt.Errorf("window %q should look like 08:00-12:00", raw)
	}
	start, err := parseClock(match[1])
	if err != nil {
		return Window{}, fmt.Errorf("window %q should look like 08:00-12:00", raw)
	}
	end, err := parseClock(match[2])
	if err != nil {
		return Window{}, fmt.Errorf("window %q should look like 08:00-12:00", raw)
	}
	if start >= end {
		return Window{}, fmt.Errorf("window %q ends before it starts", raw)
	}
	return Window{Start: start, End: end}, nil
}

func parseClock(raw string) (int, error) {
	var hour, minute int
	if _, err := fmt.Sscanf(raw, "%d:%d", &hour, &minute); err != nil {
		return 0, err
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("bad time %s", raw)
	}
	return hour*60 + minute, nil
}

// StartClock formats the start as HH:MM.
func (w Window) StartClock() string {
	return clock(w.Start)
}

// EndClock formats the end as HH:MM.
func (w Window) EndClock() string {
	return clock(w.End)
}

func clock(minutes int) string {
	return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
}
