// Package lease counts correct answers and caps how long the sites stay open.
package lease

import (
	"fmt"
	"time"
)

const (
	// SocialLength is how many correct answers open every blocked site.
	SocialLength = 10
	// ShortLength is how many correct answers open only WhatsApp and Gmail.
	ShortLength = 5
	// MaxGrant is the longest break.
	MaxGrant = 30 * time.Minute
)

// Session is one unlock or schedule change.
type Session struct {
	Correct int
}

// Accept counts a correct answer. A wrong answer leaves the count alone.
// It reports whether the quiz has reached need.
func (s *Session) Accept(right bool, need int) bool {
	if s == nil || !right || need <= 0 {
		return false
	}
	s.Correct++
	return s.Correct >= need
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
