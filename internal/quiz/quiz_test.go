package quiz

import (
	"math/rand"
	"strings"
	"testing"
)

func TestBankIsUsable(t *testing.T) {
	seen := map[string]bool{}
	for _, item := range Bank() {
		if seen[item.ID] {
			t.Fatalf("duplicate %s", item.ID)
		}
		seen[item.ID] = true
		if item.Topic != "k8s" && item.Topic != "go" && item.Topic != "problem" {
			t.Fatalf("%s topic %s", item.ID, item.Topic)
		}
		if item.Ask == "" || item.Why == "" {
			t.Fatalf("%s is incomplete", item.ID)
		}
		if item.Correct < 0 || item.Correct > 3 {
			t.Fatalf("%s correct %d", item.ID, item.Correct)
		}
		for _, choice := range item.Choices {
			if choice == "" {
				t.Fatalf("%s has an empty choice", item.ID)
			}
		}
		prompt := item.Prompt()
		if !strings.Contains(prompt, "["+item.Topic+"]") || !strings.Contains(prompt, "  d) ") {
			t.Fatalf("prompt %q", prompt)
		}
		if strings.Contains(prompt, item.Why) {
			t.Fatalf("prompt leaked the explanation: %s", item.ID)
		}
		if item.Letter() != string(rune('a'+item.Correct)) {
			t.Fatalf("letter %s", item.Letter())
		}
		if !strings.Contains(item.Explain(), "Answer "+item.Letter()) {
			t.Fatalf("explain %s", item.Explain())
		}
	}
	if len(seen) < 36 {
		t.Fatalf("only %d questions", len(seen))
	}
}

func TestDrawAvoidsRepeats(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	used := map[string]bool{}
	seen := map[string]bool{}
	for i := 0; i < len(bank); i++ {
		item := Draw(rng, used)
		if seen[item.ID] {
			t.Fatalf("repeat %s before the bank was used up", item.ID)
		}
		seen[item.ID] = true
		used[item.ID] = true
	}
	item := Draw(rng, used)
	if item.ID == "" || len(used) != 0 {
		t.Fatalf("draw after a full bank did not start over: %s used %d", item.ID, len(used))
	}
}
