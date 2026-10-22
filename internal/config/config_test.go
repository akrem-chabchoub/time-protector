package config

import (
	"os"
	"path/filepath"
	"testing"

	"time-protector/internal/schedule"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	want := schedule.Default()
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode %s", info.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Windows) != len(want.Windows) {
		t.Fatalf("windows %+v", got.Windows)
	}
	for i := range want.Windows {
		if got.Windows[i] != want.Windows[i] {
			t.Fatalf("window %d %+v", i, got.Windows[i])
		}
	}
}

func TestLoadRejectsBadWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("windows:\n  - start: \"12:00\"\n    end: \"08:00\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected an error")
	}
}
