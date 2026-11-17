//go:build !darwin

package tasks

// ExcludeFromBackup does nothing here: Windows has no per-folder backup exclusion that needs one (File History
// and the system image do not read the app's data folder by default).
func ExcludeFromBackup(string) error { return nil }
