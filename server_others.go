//go:build !linux

package trabbits

import (
	"context"
	"net"
)

func newListener(ctx context.Context, addr string) (net.Listener, error) {
	lc := &net.ListenConfig{}
	return lc.Listen(ctx, "tcp", addr)
}
