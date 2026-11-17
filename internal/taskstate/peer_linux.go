//go:build linux

package taskstate

import (
	"net"

	"golang.org/x/sys/unix"
)

func peerUIDOf(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var ierr error
	if err := raw.Control(func(fd uintptr) {
		x, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if e != nil {
			ierr = e
			return
		}
		uid = x.Uid
	}); err != nil {
		return 0, err
	}
	return uid, ierr
}
