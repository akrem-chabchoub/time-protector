// Package hosts edits only the marked block in the hosts file.
package hosts

import (
	"fmt"
	"strings"
)

const (
	// Begin marks the first line of the block this tool owns.
	Begin = "# BEGIN time-protector"
	// End marks the last line of that block.
	End = "# END time-protector"
)

// Apply inserts or removes the block. Lines outside it are kept.
// The block is placed at the top because the first matching hosts line wins.
func Apply(content string, domains []string, block bool) (string, error) {
	stripped, err := removeBlock(content)
	if err != nil {
		return "", err
	}
	if !block {
		return stripped, nil
	}
	for _, domain := range domains {
		if domain == "" || strings.ContainsAny(domain, " \t\n#") {
			return "", fmt.Errorf("bad domain %q", domain)
		}
	}
	var b strings.Builder
	b.WriteString(Begin)
	b.WriteByte('\n')
	for _, domain := range domains {
		fmt.Fprintf(&b, "127.0.0.1 %s\n::1 %s\n", domain, domain)
	}
	b.WriteString(End)
	b.WriteByte('\n')
	rest := strings.TrimLeft(stripped, "\n")
	if rest == "" {
		return b.String(), nil
	}
	if !strings.HasSuffix(rest, "\n") {
		rest += "\n"
	}
	return b.String() + rest, nil
}

func removeBlock(content string) (string, error) {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	in := false
	for _, line := range lines {
		if !in && line == Begin {
			in = true
			continue
		}
		if in && line == End {
			in = false
			continue
		}
		if in {
			continue
		}
		out = append(out, line)
	}
	if in {
		return "", fmt.Errorf("hosts block is missing the end marker")
	}
	return strings.Join(out, "\n"), nil
}
