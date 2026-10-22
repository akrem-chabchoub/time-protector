// Package dnscheck looks at Chrome's secure DNS setting.
// Secure DNS can skip the hosts file, so the block would not hold.
package dnscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DefaultChromeDir is this user's Chrome folder.
func DefaultChromeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "Google", "Chrome"), nil
}

// AnyProfileSecureDNS reports whether the Chrome profile in use has secure DNS on.
// Chrome leaves the setting out of the file until someone changes it, and the default is on.
func AnyProfileSecureDNS(dir string) (bool, error) {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	profile := lastUsedProfile(dir)
	if profile == "" {
		profile = "Default"
	}
	return ChromeSecureDNS(filepath.Join(dir, profile, "Preferences"))
}

func lastUsedProfile(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, "Local State"))
	if err != nil {
		return ""
	}
	var state struct {
		Profile struct {
			LastUsed string `json:"last_used"`
		} `json:"profile"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return ""
	}
	return state.Profile.LastUsed
}

// ChromeSecureDNS reports whether the preferences file asks Chrome to use secure DNS.
// A missing file means Chrome is not set up here, which is not a warning.
func ChromeSecureDNS(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var prefs struct {
		DNSOverHTTPS struct {
			Mode string `json:"mode"`
		} `json:"dns_over_https"`
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return false, err
	}
	return strings.ToLower(prefs.DNSOverHTTPS.Mode) != "off", nil
}
