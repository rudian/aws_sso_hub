//go:build darwin

package taskstate

import (
	"net"

	"golang.org/x/sys/unix"
)

// peerUIDOf reads the connecting user's ID from the kernel (LOCAL_PEERCRED).
func peerUIDOf(c *net.UnixConn) (uint32, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid uint32
	var ierr error
	if err := raw.Control(func(fd uintptr) {
		x, e := unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
		if e != nil {
			ierr = e
			return
		}
		uid = x.Uid
	}); err != nil {
		return 0, err
	}
	if ierr != nil {
		return 0, ierr
	}
	return uid, nil
}
