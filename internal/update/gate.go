package update

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Gate is what a staged bundle must satisfy before the helper may swap it in.
type Gate struct {
	BundleID          string
	Requirement       string // codesign requirement text, plan 4.14 item 4
	RequireGatekeeper bool   // spctl -a -t exec as a second check; true in production, never the only check
}

// rejectAppleDouble refuses a staged tree that holds AppleDouble "._*" entries. A zip made with a plain `ditto`
// carries them, Wails' extractor writes them as real files, and the sealed bundle no longer verifies. The release
// script builds the archive with --norsrc --noextattr --noqtn --noacl so a good archive has none.
func rejectAppleDouble(root string) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), "._") {
			rel, _ := filepath.Rel(root, p)
			return fmt.Errorf("the archive holds an AppleDouble entry (%s); it was not built with ditto --norsrc --noextattr", rel)
		}
		return nil
	})
}

// removeStaging removes the staging folder Wails made beside the app ("wails-update-*" next to the target) and
// nothing else: the folder must be the parent of the staged path, carry that prefix and sit in installDir.
func removeStaging(staged, installDir string) {
	dir := filepath.Dir(staged)
	if !strings.HasPrefix(filepath.Base(dir), "wails-update-") || !sameDir(filepath.Dir(dir), installDir) {
		return
	}
	_ = os.RemoveAll(dir)
}

// sameDir compares two folders after resolving symlinks (/var and /private/var are one folder on macOS).
func sameDir(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

var errNoRequirement = errors.New("no code-signature requirement configured")
