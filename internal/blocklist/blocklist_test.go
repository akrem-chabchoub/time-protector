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
	for _, domain := range []string{
		"facebook.com",
		"www.facebook.com",
		"instagram.com",
		"www.instagram.com",
		"web.whatsapp.com",
		"wa.me",
		"linkedin.com",
		"www.linkedin.com",
		"gmail.com",
		"mail.google.com",
	} {
		if !got[domain] {
			t.Errorf("missing %s", domain)
		}
	}
	for _, domain := range []string{"google.com", "accounts.google.com", "imap.gmail.com", "messenger.com"} {
		if got[domain] {
			t.Errorf("should not block %s", domain)
		}
	}
	quick := map[string]bool{}
	for _, domain := range Short() {
		quick[domain] = true
	}
	if quick["facebook.com"] || quick["linkedin.com"] || !quick["mail.google.com"] || !quick["web.whatsapp.com"] {
		t.Fatalf("short group %v", Short())
	}
	for _, domain := range Social() {
		if domain == "web.whatsapp.com" || domain == "gmail.com" {
			t.Fatalf("social group includes a short site: %s", domain)
		}
	}
}
