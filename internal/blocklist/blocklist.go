// Package blocklist is the set of sites this tool closes.
package blocklist

// Social is Facebook, Instagram, and LinkedIn. Opening it takes the longer quiz and also opens the short group.
func Social() []string {
	return append([]string(nil), social...)
}

// Short is WhatsApp and Gmail. Opening it takes fewer questions and leaves the social sites closed.
func Short() []string {
	return append([]string(nil), short...)
}

// Domains returns every name that can be blocked.
func Domains() []string {
	out := Social()
	return append(out, Short()...)
}

var social = []string{
	"facebook.com",
	"www.facebook.com",
	"m.facebook.com",
	"fb.com",
	"instagram.com",
	"www.instagram.com",
	"i.instagram.com",
	"linkedin.com",
	"www.linkedin.com",
	"lnkd.in",
}

var short = []string{
	"whatsapp.com",
	"www.whatsapp.com",
	"web.whatsapp.com",
	"wa.me",
	"gmail.com",
	"mail.google.com",
}
