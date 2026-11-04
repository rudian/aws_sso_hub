package gitx

// SetNeutralParent makes the private working directory for repository-less git calls be created under dir
// and forgets the current one. The returned function restores the previous state.
func SetNeutralParent(dir string) (restore func()) {
	neutral.Lock()
	prevDir, prevParent := neutral.dir, neutral.parent
	neutral.dir, neutral.parent = "", dir
	neutral.Unlock()
	return func() {
		neutral.Lock()
		neutral.dir, neutral.parent = prevDir, prevParent
		neutral.Unlock()
	}
}

// NeutralDir exposes neutralDir.
func NeutralDir() string { return neutralDir() }

// CheckVersion exposes checkVersion.
func CheckVersion(v [3]int) error { return checkVersion(v) }
