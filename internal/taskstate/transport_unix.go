//go:build !windows

package taskstate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// maxSocketPath is below the shortest sun_path limit (104 on macOS).
const maxSocketPath = 100

// Endpoint is where the app listens for a data folder: <data>/run/taskstate.sock, or, when that path is too
// long for a socket, a short one under the temp dir that is private to the user.
func Endpoint(dataDir string) string {
	p := filepath.Join(dataDir, "run", "taskstate.sock")
	if len(p) <= maxSocketPath {
		return p
	}
	h := sha256.Sum256([]byte(dataDir))
	return filepath.Join(os.TempDir(), fmt.Sprintf("codepass-%d", os.Geteuid()), hex.EncodeToString(h[:6])+".sock")
}

// peerUID is replaced in tests to simulate another user.
var peerUID = peerUIDOf

type unixListener struct {
	l    *net.UnixListener
	path string
	uid  func(*net.UnixConn) (uint32, error) // peerUID as of Listen
}

// Listen creates the socket: its folder is private (0700, ours, not a link), the socket file 0600, and every
// connection is checked against the peer's user ID. A stale socket file of a dead app is replaced; a live
// one gives ErrInUse.
func Listen(endpoint string) (*Server, error) {
	l, err := listen(endpoint)
	if err != nil {
		return nil, err
	}
	return newServer(l, endpoint), nil
}

func listen(path string) (listener, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || !ok || int(st.Uid) != os.Geteuid() {
		return nil, fmt.Errorf("taskstate: %s is not a private folder of this user", dir)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	if c, err := net.DialTimeout("unix", path, dialTimeout); err == nil {
		c.Close()
		return nil, ErrInUse
	}
	_ = os.Remove(path)
	// No umask change: it is process-wide. The folder is private (0700, ours), so the socket file is not
	// reachable by others while the chmod below runs.
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	l.SetUnlinkOnClose(true)
	return &unixListener{l: l, path: path, uid: peerUID}, nil
}

func (u *unixListener) Accept() (io.ReadCloser, error) {
	for {
		c, err := u.l.AcceptUnix()
		if err != nil {
			return nil, err
		}
		uid, err := u.uid(c)
		if err != nil || uid != uint32(os.Geteuid()) {
			c.Close()
			continue
		}
		return c, nil
	}
}

func (u *unixListener) Close() error { return u.l.Close() }

func dialSend(path string, b []byte) error {
	c, err := net.DialTimeout("unix", path, dialTimeout)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetWriteDeadline(time.Now().Add(dialTimeout))
	_, err = c.Write(b)
	return err
}
