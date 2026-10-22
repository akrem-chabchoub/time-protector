// Package blocklist is the set of sites this tool closes.
package blocklist

// Domains returns a copy of the web names that get blocked.
// Mail servers and the rest of Google stay off this list.
func Domains() []string {
	return append([]string(nil), domains...)
}

var domains = []string{
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
