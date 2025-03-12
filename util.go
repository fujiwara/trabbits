package trabbits

import "net/url"

// mask password in URL
func safeURLString(u url.URL) string {
	if u.User != nil {
		u.User = url.User(u.User.Username())
	}
	return u.String()
}
