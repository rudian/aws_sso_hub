package execx

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// IsAbsPath reports whether p is an absolute path in the given OS's syntax, independent of the running OS.
func IsAbsPath(p, goos string) bool {
	if goos == "windows" {
		if len(p) >= 3 && p[1] == ':' && (p[2] == '\\' || p[2] == '/') {
			c := p[0]
			return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		}
		return strings.HasPrefix(p, `\\`)
	}
	return strings.HasPrefix(p, "/")
}

// HasControlChars reports whether s contains a Unicode control character or a line/paragraph separator.
func HasControlChars(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cc, r) || r == '\u0085' || r == ' ' || r == ' ' {
			return true
		}
	}
	return false
}

// ValidateToolPath checks a user-chosen aws or git program path. kind is "aws" or "git"; "" means automatic
// detection and is valid. The path must be absolute, name the right program, and (on Unix) point at an
// executable regular file that only the current user or root can replace.
func ValidateToolPath(kind, path, goos string) error {
	if path == "" {
		return nil
	}
	if kind != "aws" && kind != "git" {
		return fmt.Errorf("execx: unknown tool kind %q", kind)
	}
	if !IsAbsPath(path, goos) {
		return fmt.Errorf("the path must be absolute")
	}
	if HasControlChars(path) || strings.Contains(path, `"`) {
		return fmt.Errorf("the path contains characters that are not allowed")
	}
	var base string
	if goos == "windows" {
		i := strings.LastIndexAny(path, `\/`)
		base = strings.ToLower(path[i+1:])
		if base != kind+".exe" {
			return fmt.Errorf("the file must be named %s.exe", kind)
		}
	} else {
		base = filepath.Base(path)
		if base != kind {
			return fmt.Errorf("the file must be named %s", kind)
		}
	}
	if goos != runtime.GOOS {
		return nil // the file system checks below only make sense for the running OS
	}
	// The checks apply to what will actually run: the symlink is resolved first (Homebrew links
	// /opt/homebrew/bin/aws into its Cellar), then the target file and the folders on both sides are checked.
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("cannot read the file: %w", err)
	}
	st, err := os.Stat(real)
	if err != nil {
		return fmt.Errorf("cannot read the file: %w", err)
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}
	if goos == "windows" {
		return nil
	}
	if st.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("the file is not executable")
	}
	if st.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("the file is writable by other users")
	}
	if !ownedByUserOrRoot(st) {
		return fmt.Errorf("the file belongs to another user")
	}
	dirs := []string{filepath.Dir(real)}
	if d := filepath.Dir(path); d != dirs[0] {
		dirs = append(dirs, d)
	}
	for _, d := range dirs {
		dst, err := os.Stat(d)
		if err != nil {
			return fmt.Errorf("cannot read the folder: %w", err)
		}
		if !folderTrusted(dst) {
			return fmt.Errorf("the folder is writable by other users")
		}
	}
	return nil
}
