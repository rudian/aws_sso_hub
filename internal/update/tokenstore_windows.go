//go:build windows

package update

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32     = windows.NewLazySystemDLL("advapi32.dll")
	procCredW    = advapi32.NewProc("CredWriteW")
	procCredR    = advapi32.NewProc("CredReadW")
	procCredD    = advapi32.NewProc("CredDeleteW")
	procCredFree = advapi32.NewProc("CredFree")
)

const (
	credTypeGeneric        = 1
	credPersistLocalMachin = 2 // CRED_PERSIST_LOCAL_MACHINE: stays on this machine, never roams
	errNotFound            = syscall.Errno(1168)
)

// credential mirrors the CREDENTIALW structure.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// credBackend keeps the token in the Windows Credential Manager, readable only by the signed-in user.
type credBackend struct{ target, user string }

func platformBackend(service, account string) secretBackend {
	return credBackend{target: service, user: account}
}

func (c credBackend) read() ([]byte, bool, error) {
	t, err := windows.UTF16PtrFromString(c.target)
	if err != nil {
		return nil, false, err
	}
	var p *credential
	r, _, e := procCredR.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&p)))
	if r == 0 {
		if errors.Is(e, errNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("credential read failed (%v)", e)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(p)))
	b := make([]byte, p.CredentialBlobSize)
	if p.CredentialBlobSize > 0 {
		copy(b, unsafe.Slice(p.CredentialBlob, p.CredentialBlobSize))
	}
	return b, true, nil
}

func (c credBackend) write(secret []byte) error {
	t, err := windows.UTF16PtrFromString(c.target)
	if err != nil {
		return err
	}
	u, err := windows.UTF16PtrFromString(c.user)
	if err != nil {
		return err
	}
	cr := credential{Type: credTypeGeneric, TargetName: t, UserName: u, Persist: credPersistLocalMachin, CredentialBlobSize: uint32(len(secret))}
	if len(secret) > 0 {
		cr.CredentialBlob = &secret[0]
	}
	if r, _, e := procCredW.Call(uintptr(unsafe.Pointer(&cr)), 0); r == 0 {
		return fmt.Errorf("credential write failed (%v)", e)
	}
	return nil
}

func (c credBackend) remove() error {
	t, err := windows.UTF16PtrFromString(c.target)
	if err != nil {
		return err
	}
	if r, _, e := procCredD.Call(uintptr(unsafe.Pointer(t)), credTypeGeneric, 0); r == 0 && !errors.Is(e, errNotFound) {
		return fmt.Errorf("credential delete failed (%v)", e)
	}
	return nil
}
