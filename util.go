package trabbits

import (
	"crypto/rand"
	"net/url"
	"strings"
	"syscall"
)

// mask password in URL
func safeURLString(u url.URL) string {
	if u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "********")
	}
	return u.String()
}

func isBrokenPipe(err error) bool {
	if errno, ok := err.(syscall.Errno); ok {
		return errno == syscall.EPIPE
	} else if strings.Contains(err.Error(), "broken pipe") {
		return true
	}
	return false
}

func generateID() string {
	return strings.ToLower(rand.Text())
}
