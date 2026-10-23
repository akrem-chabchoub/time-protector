// Package quiz is the multiple-choice practice used to unlock.
package quiz

import (
	"fmt"
	"math/rand"
	"strings"
)

// Question is one practice item. The answer stays out of Prompt.
type Question struct {
	ID      string
	Topic   string
	Ask     string
	Choices [4]string
	Correct int
	Why     string
}

// Prompt is what the person sees. It does not mark the right choice.
func (q Question) Prompt() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s\n", q.Topic, q.Ask)
	for i, choice := range q.Choices {
		fmt.Fprintf(&b, "  %c) %s\n", 'a'+i, choice)
	}
	return strings.TrimRight(b.String(), "\n")
}

// Letter is the correct choice, a through d.
func (q Question) Letter() string {
	return string(rune('a' + q.Correct))
}

// Explain is shown after the person answers, so the quiz teaches.
func (q Question) Explain() string {
	return "Answer " + q.Letter() + ". " + q.Why
}

// Bank returns a copy of the practice questions.
func Bank() []Question {
	out := make([]Question, len(bank))
	copy(out, bank)
	return out
}

// Draw picks a question that is not in used. When all of them have been used, it starts over.
func Draw(rng *rand.Rand, used map[string]bool) Question {
	all := Bank()
	pool := make([]Question, 0, len(all))
	for _, item := range all {
		if used == nil || !used[item.ID] {
			pool = append(pool, item)
		}
	}
	if len(pool) == 0 {
		for id := range used {
			delete(used, id)
		}
		pool = all
	}
	return pool[rng.Intn(len(pool))]
}
