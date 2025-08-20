//go:build linux

package trabbits

import (
	"context"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

func newListener(ctx context.Context, addr string) (net.Listener, error) {
	lc := &net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var opErr error
			err := c.Control(func(fd uintptr) {
				// SO_REUSEPORT
				opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, unix.SO_REUSEPORT, 1)
			})
			if err != nil {
				return err
			}
			return opErr
		},
	}
	return lc.Listen(ctx, "tcp", addr)
}
