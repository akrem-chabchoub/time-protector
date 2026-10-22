// Package quiz builds the boring sums used to unlock.
package quiz

import (
	"fmt"
	"math/rand"
)

// Problem is one addition or subtraction of two numbers from 10 to 99.
type Problem struct {
	Left  int
	Right int
	Op    string
}

// Answer is the result. Subtraction is never negative.
func (p Problem) Answer() int {
	if p.Op == "-" {
		return p.Left - p.Right
	}
	return p.Left + p.Right
}

// Prompt is what the person sees, like "47 + 18".
func (p Problem) Prompt() string {
	return fmt.Sprintf("%d %s %d", p.Left, p.Op, p.Right)
}

// New draws one problem from rng.
func New(rng *rand.Rand) Problem {
	op := "+"
	if rng.Intn(2) == 1 {
		op = "-"
	}
	left := 10 + rng.Intn(90)
	right := 10 + rng.Intn(90)
	if op == "-" && left < right {
		left, right = right, left
	}
	return Problem{Left: left, Right: right, Op: op}
}
