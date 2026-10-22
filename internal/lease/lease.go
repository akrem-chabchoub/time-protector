// Package lease counts correct sums and caps how long the sites stay open.
package lease

import (
	"fmt"
	"time"
)

const (
	// QuizLength is how many correct sums open the sites.
	QuizLength = 50
	// MaxGrant is the longest break.
	MaxGrant = 30 * time.Minute
)

// Session is one unlock or schedule change.
type Session struct {
	Correct int
}

// Accept counts a correct answer. A wrong answer leaves the count alone.
// It reports whether the quiz is finished.
func (s *Session) Accept(right bool) bool {
	if s == nil || !right {
		return false
	}
	s.Correct++
	return s.Correct >= QuizLength
}

// Reset drops progress after a quit.
func (s *Session) Reset() {
	if s == nil {
		return
	}
	s.Correct = 0
}

// ParseGrant reads a duration like 15m. Longer requests are cut to MaxGrant.
func ParseGrant(raw string) (time.Duration, bool, error) {
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, false, fmt.Errorf("use a duration like 15m")
	}
	if d > MaxGrant {
		return MaxGrant, true, nil
	}
	return d, false, nil
}

// Active reports whether a lease still open at now. The end instant is already closed.
func Active(until, now time.Time) bool {
	return !until.IsZero() && now.Before(until)
}
