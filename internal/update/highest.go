package update

import (
	"errors"
	"fmt"
	"golang.org/x/mod/semver"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Highest stores the highest version this machine has ever run. The app calls Record on every start; the provider
// reads it as the floor for new releases. A missing file means a first run. A file that cannot be read or does not
// hold a version is an error, never "no floor": otherwise anyone who can corrupt the file could switch the rule off.
type Highest struct{ Path string }

// ErrHighestUnreadable is wrapped by every failure to read the stored highest version.
var ErrHighestUnreadable = errors.New("the stored highest version cannot be read")

// Get returns the stored version ("" when the file does not exist yet).
func (h Highest) Get() (string, error) {
	if h.Path == "" {
		return "", fmt.Errorf("%w: no file is configured", ErrHighestUnreadable)
	}
	b, err := os.ReadFile(h.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrHighestUnreadable, err)
	}
	v := strings.TrimSpace(string(b))
	if len(b) > 256 || !semver.IsValid("v"+v) {
		return "", fmt.Errorf("%w: %s does not hold a version", ErrHighestUnreadable, h.Path)
	}
	return v, nil
}

// Record raises the stored value to v when v is higher; it never lowers it. It fails if the stored value is
// unreadable (the file is left as it is, so the problem stays visible).
func (h Highest) Record(v string) error {
	if !semver.IsValid("v" + v) {
		return nil
	}
	cur, err := h.Get() // also refuses an empty Path
	if err != nil {
		return err
	}
	if cur != "" && semver.Compare("v"+cur, "v"+v) >= 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(h.Path), 0o700); err != nil {
		return err
	}
	tmp := h.Path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(v + "\n"); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, h.Path)
}
