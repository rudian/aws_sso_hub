//go:build !windows

package update

import (
	"io/fs"
	"path/filepath"
	"syscall"
)

// firstUnwritableDir walks the folders of root (symlinks are not followed) and returns the first one this user
// cannot write to, or "".
func firstUnwritableDir(root string) string {
	bad := ""
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			bad = p
			return fs.SkipAll
		}
		if d.IsDir() && syscall.Access(p, 2 /* W_OK */) != nil {
			bad = p
			return fs.SkipAll
		}
		return nil
	})
	return bad
}
