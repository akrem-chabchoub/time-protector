package blocklist

import "testing"

func TestDomains(t *testing.T) {
	got := map[string]bool{}
	for _, domain := range Domains() {
		if got[domain] {
			t.Fatalf("duplicate %s", domain)
		}
		got[domain] = true
	}
	want := []string{
		"gmail.com",
		"mail.google.com",
		"facebook.com",
		"www.facebook.com",
		"m.facebook.com",
		"fb.com",
		"messenger.com",
		"instagram.com",
		"www.instagram.com",
		"i.instagram.com",
		"linkedin.com",
		"www.linkedin.com",
		"lnkd.in",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d domains", len(got))
	}
	for _, domain := range want {
		if !got[domain] {
			t.Errorf("missing %s", domain)
		}
	}
	for _, domain := range []string{"google.com", "accounts.google.com", "imap.gmail.com"} {
		if got[domain] {
			t.Errorf("should not block %s", domain)
		}
	}
}
