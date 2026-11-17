//go:build !windows && !darwin && !linux

package taskstate

import (
	"errors"
	"net"
)

// No peer credentials here: fail closed.
func peerUIDOf(*net.UnixConn) (uint32, error) {
	return 0, errors.New("taskstate: no peer credentials on this system")
}
