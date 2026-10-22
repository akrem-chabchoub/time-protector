package quiz

import (
	"math/rand"
	"testing"
)

func TestKnownAnswers(t *testing.T) {
	add := Problem{Left: 47, Right: 18, Op: "+"}
	if add.Answer() != 65 || add.Prompt() != "47 + 18" {
		t.Fatalf("add %+v answer %d", add, add.Answer())
	}
	sub := Problem{Left: 80, Right: 23, Op: "-"}
	if sub.Answer() != 57 || sub.Prompt() != "80 - 23" {
		t.Fatalf("sub %+v answer %d", sub, sub.Answer())
	}
	zero := Problem{Left: 10, Right: 10, Op: "-"}
	if zero.Answer() != 0 {
		t.Fatal(zero.Answer())
	}
}

func TestNewStaysBoring(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 2000; i++ {
		p := New(rng)
		if p.Left < 10 || p.Left > 99 || p.Right < 10 || p.Right > 99 {
			t.Fatalf("out of range %+v", p)
		}
		if p.Op != "+" && p.Op != "-" {
			t.Fatalf("op %q", p.Op)
		}
		if p.Op == "-" && p.Left < p.Right {
			t.Fatalf("negative subtraction %+v", p)
		}
		if p.Answer() < 0 || p.Answer() > 198 {
			t.Fatalf("answer %d for %+v", p.Answer(), p)
		}
	}
}
