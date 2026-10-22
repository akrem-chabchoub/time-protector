package dnscheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnyProfileSecureDNS(t *testing.T) {
	dir := t.TempDir()
	on, err := AnyProfileSecureDNS(filepath.Join(dir, "missing"))
	if err != nil || on {
		t.Fatalf("missing dir on=%v err=%v", on, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "Profile 1"), 0o755); err != nil {
		t.Fatal(err)
	}
	off := filepath.Join(dir, "Default", "Preferences")
	other := filepath.Join(dir, "Profile 1", "Preferences")
	if err := os.WriteFile(off, []byte(`{"dns_over_https":{"mode":"off"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte(`{"dns_over_https":{"mode":"automatic"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	on, err = AnyProfileSecureDNS(dir)
	if err != nil || on {
		t.Fatalf("unused profile should not warn, on=%v err=%v", on, err)
	}
	state := filepath.Join(dir, "Local State")
	if err := os.WriteFile(state, []byte(`{"profile":{"last_used":"Profile 1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	on, err = AnyProfileSecureDNS(dir)
	if err != nil || !on {
		t.Fatalf("on=%v err=%v", on, err)
	}
}

func TestChromeSecureDNS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Preferences")
	on, err := ChromeSecureDNS(filepath.Join(dir, "missing"))
	if err != nil || on {
		t.Fatalf("missing file on=%v err=%v", on, err)
	}
	cases := []struct {
		body string
		on   bool
	}{
		{`{"dns_over_https":{"mode":"secure"}}`, true},
		{`{"dns_over_https":{"mode":"automatic"}}`, true},
		{`{"dns_over_https":{"mode":"off"}}`, false},
		{`{}`, true},
	}
	for _, tc := range cases {
		if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := ChromeSecureDNS(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.on {
			t.Errorf("%s on=%v", tc.body, got)
		}
	}
}
