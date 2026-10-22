package hosts

import (
	"strings"
	"testing"
)

func TestApplyKeepsUnrelatedLines(t *testing.T) {
	original := "127.0.0.1 localhost\n# keep me\n10.0.0.1 keep.test\n"
	got, err := Apply(original, []string{"mail.google.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "127.0.0.1 localhost\n") {
		t.Fatalf("lost localhost:\n%s", got)
	}
	if !strings.Contains(got, "# keep me\n") || !strings.Contains(got, "10.0.0.1 keep.test\n") {
		t.Fatalf("lost unrelated lines:\n%s", got)
	}
	if !strings.HasPrefix(got, Begin+"\n") {
		t.Fatalf("block should be first:\n%s", got)
	}
	if !strings.Contains(got, "127.0.0.1 mail.google.com\n") || !strings.Contains(got, "::1 mail.google.com\n") {
		t.Fatalf("missing block lines:\n%s", got)
	}
	if strings.Contains(got, "accounts.google.com") {
		t.Fatalf("unexpected domain:\n%s", got)
	}
	again, err := Apply(got, []string{"mail.google.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatalf("not idempotent\nfirst:\n%s\nsecond:\n%s", got, again)
	}
	if strings.Count(again, "10.0.0.1 keep.test") != 1 {
		t.Fatalf("unrelated line duplicated:\n%s", again)
	}
}

func TestApplyRemove(t *testing.T) {
	original := "127.0.0.1 localhost\n"
	blocked, err := Apply(original, []string{"fb.com"}, true)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := Apply(blocked, []string{"fb.com"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if opened != original {
		t.Fatalf("got %q", opened)
	}
	untouched, err := Apply(original, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if untouched != original {
		t.Fatalf("unblock changed a file with no block: %q", untouched)
	}
}

func TestBrokenMarker(t *testing.T) {
	_, err := Apply(Begin+"\n127.0.0.1 mail.google.com\n", []string{"mail.google.com"}, true)
	if err == nil {
		t.Fatal("expected an error for a block with no end marker")
	}
}

func TestBadDomain(t *testing.T) {
	_, err := Apply("127.0.0.1 localhost\n", []string{"evil.com\n127.0.0.1 extra"}, true)
	if err == nil {
		t.Fatal("expected an error for a domain with a newline")
	}
}
