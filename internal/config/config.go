// Package config reads and writes the hours file.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"time-protector/internal/schedule"
)

type file struct {
	Windows []window `yaml:"windows"`
}

type window struct {
	Start string `yaml:"start"`
	End   string `yaml:"end"`
}

// Load reads a schedule. A missing file is an error so the caller can fall back.
func Load(path string) (schedule.Schedule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return schedule.Schedule{}, err
	}
	var decoded file
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		return schedule.Schedule{}, err
	}
	var out schedule.Schedule
	for _, item := range decoded.Windows {
		parsed, err := schedule.ParseWindow(item.Start + "-" + item.End)
		if err != nil {
			return schedule.Schedule{}, fmt.Errorf("config %s: %w", path, err)
		}
		out.Windows = append(out.Windows, parsed)
	}
	return out, nil
}

// Save writes the schedule. The file mode is 0644.
func Save(path string, s schedule.Schedule) error {
	encoded := file{Windows: make([]window, 0, len(s.Windows))}
	for _, item := range s.Windows {
		encoded.Windows = append(encoded.Windows, window{Start: item.StartClock(), End: item.EndClock()})
	}
	data, err := yaml.Marshal(encoded)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeAtomic(path, data, 0o644)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}
