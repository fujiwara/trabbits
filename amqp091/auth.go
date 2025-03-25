package amqp091

import (
	"fmt"
	"strings"
)

func ParsePLAINAuthResponse(s string) (user string, password string, err error) {
	p := strings.SplitN(s, "\x00", 3) // null, user, pass
	if len(p) != 3 {
		return "", "", fmt.Errorf("invalid auth response %s", s)
	}
	return p[1], p[2], nil
}

func ParseAMQPLAINAuthResponse(s string) (user string, password string, err error) {
	b := strings.NewReader(s)
	for {
		k, err := readShortstr(b)
		if err != nil {
			break
		}
		v, err := readField(b)
		if err != nil {
			break
		}
		if k == "LOGIN" {
			if u, ok := v.(string); ok {
				user = u
			}
		}
		if k == "PASSWORD" {
			if p, ok := v.(string); ok {
				password = p
			}
		}
	}
	if user == "" || password == "" {
		return "", "", fmt.Errorf("invalid auth response %s", s)
	}
	return user, password, nil
}
