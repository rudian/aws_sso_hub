//go:build windows

package taskstate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Endpoint is the named pipe for a data folder. The pipe has a security descriptor that grants access to the
// current user only, and remote clients are refused.
func Endpoint(dataDir string) string {
	h := sha256.Sum256([]byte(dataDir))
	return `\\.\pipe\codepass-taskstate-` + hex.EncodeToString(h[:8])
}

// Listen creates the pipe. See Endpoint for who may connect. The first instance is created here with
// FILE_FLAG_FIRST_PIPE_INSTANCE, so a name already held by someone else (another user, or another app) is an
// error and not a silent retry loop.
func Listen(endpoint string) (*Server, error) {
	if c, err := os.OpenFile(endpoint, os.O_WRONLY, 0); err == nil {
		c.Close()
		return nil, ErrInUse
	}
	l, err := newPipeListener(endpoint)
	if err != nil {
		return nil, err
	}
	h, err := l.create(true)
	if err != nil {
		if errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return nil, ErrInUse
		}
		return nil, err
	}
	l.next = h
	return newServer(l, endpoint), nil
}

type pipeListener struct {
	name string
	sa   *windows.SecurityAttributes
	mu   sync.Mutex
	done bool
	next windows.Handle // the instance created ahead of the first Accept (0 once taken)
}

func newPipeListener(name string) (*pipeListener, error) {
	tok := windows.GetCurrentProcessToken()
	u, err := tok.GetTokenUser()
	if err != nil {
		return nil, err
	}
	// D:P = protected DACL, one entry: generic all for the current user's SID.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + u.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	return &pipeListener{name: name, sa: sa}, nil
}

// create makes one pipe instance. first demands that the name is new (set only by Listen, once).
func (p *pipeListener) create(first bool) (windows.Handle, error) {
	flags := uint32(windows.PIPE_ACCESS_INBOUND)
	if first {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	name, err := windows.UTF16PtrFromString(p.name)
	if err != nil {
		return 0, err
	}
	return windows.CreateNamedPipe(name, flags,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		windows.PIPE_UNLIMITED_INSTANCES, 4096, 4096, 0, p.sa)
}

func (p *pipeListener) Accept() (io.ReadCloser, error) {
	p.mu.Lock()
	if p.done {
		p.mu.Unlock()
		return nil, errors.New("taskstate: listener closed")
	}
	h := p.next
	p.next = 0
	p.mu.Unlock()
	if h == 0 {
		var err error
		if h, err = p.create(false); err != nil {
			return nil, err
		}
	}
	if err := windows.ConnectNamedPipe(h, nil); err != nil && err != windows.ERROR_PIPE_CONNECTED {
		windows.CloseHandle(h)
		return nil, err
	}
	p.mu.Lock()
	done := p.done
	p.mu.Unlock()
	if done {
		windows.CloseHandle(h)
		return nil, errors.New("taskstate: listener closed")
	}
	return os.NewFile(uintptr(h), p.name), nil
}

// Close stops accepting; a dummy client wakes an Accept that waits in ConnectNamedPipe.
func (p *pipeListener) Close() error {
	p.mu.Lock()
	p.done = true
	h := p.next
	p.next = 0
	p.mu.Unlock()
	if h != 0 {
		windows.CloseHandle(h)
	}
	if c, err := os.OpenFile(p.name, os.O_WRONLY, 0); err == nil {
		c.Close()
	}
	return nil
}

// serverIsUs checks that the process serving the pipe runs as the current user, so a report (an armed first
// prompt included) never goes to another user's process that took the name first.
func serverIsUs(c *os.File) error {
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(c.Fd()), &pid); err != nil {
		return err
	}
	ph, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(ph)
	var tok windows.Token
	if err := windows.OpenProcessToken(ph, windows.TOKEN_QUERY, &tok); err != nil {
		return err
	}
	defer tok.Close()
	su, err := tok.GetTokenUser()
	if err != nil {
		return err
	}
	me, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	if !windows.EqualSid(su.User.Sid, me.User.Sid) {
		return errors.New("taskstate: the pipe is served by another user")
	}
	return nil
}

func dialSend(name string, b []byte) error {
	deadline := time.Now().Add(dialTimeout)
	for {
		c, err := os.OpenFile(name, os.O_WRONLY, 0)
		if err == nil {
			defer c.Close()
			if err := serverIsUs(c); err != nil {
				return err
			}
			// A hung app must not hold the hook: closing the file cancels a pending write.
			t := time.AfterFunc(time.Until(deadline), func() { c.Close() })
			defer t.Stop()
			_, err = c.Write(b)
			return err
		}
		busy := errors.Is(err, windows.ERROR_PIPE_BUSY) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND)
		if !busy || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
