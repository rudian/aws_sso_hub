package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

func wantRestoreCode(t *testing.T, err error, code api.ErrorCode) *api.AppError {
	t.Helper()
	ae := api.AsAppError(err)
	if err == nil || ae == nil || ae.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
	return ae
}

func writeText(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// plantBackup writes a backup file and makes it the only entry of the index, the way a tampered index would look.
func plantBackup(t *testing.T, w *world, id string, kind api.BackupKind, target, content string) {
	t.Helper()
	writeText(t, filepath.Join(w.p.BackupDir, id+".bak"), content)
	entries := []api.BackupEntry{{ID: id, CreatedAt: "2026-11-04T02:18:00Z", Path: target, File: target, Kind: kind, Reason: api.ReasonMigrate,
		ReasonParams: map[string]string{}, Size: int64(len(content))}}
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	writeText(t, filepath.Join(w.p.BackupDir, "index.json"), string(b))
}

func TestRestoreRefusesFileChangedSincePreview(t *testing.T) {
	w := newWorld(t)
	orig := w.read(w.p.AWSConfigFile)
	e1, err := w.c.Backup.Snapshot(w.p.AWSConfigFile, "~/.aws/config", api.BackupAWSConfig, api.ReasonAddProfiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeText(t, w.p.AWSConfigFile, "[profile reviewed]\nregion = eu-west-1\n")
	if _, err := w.sys.PreviewRestore(e1.ID); err != nil {
		t.Fatal(err)
	}

	// somebody edits the file after the user looked at the diff
	edited := "[profile reviewed]\nregion = eu-west-1\n[profile sneaked-in]\nregion = us-east-1\n"
	writeText(t, w.p.AWSConfigFile, edited)
	_, err = w.sys.RestoreBackup(e1.ID, false)
	ae := wantRestoreCode(t, err, api.ErrConfigChanged)
	if ae.Params["file"] == "" {
		t.Fatal("the error must name the file")
	}
	if w.read(w.p.AWSConfigFile) != edited {
		t.Fatal("the file must be left as it is")
	}
	if list, _ := w.c.Backup.List(); len(list) != 1 {
		t.Fatalf("no safety snapshot is expected for a refused restore: %d entries", len(list))
	}

	// reviewing again lets the restore through
	if _, err := w.sys.PreviewRestore(e1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.sys.RestoreBackup(e1.ID, false); err != nil {
		t.Fatal(err)
	}
	if w.read(w.p.AWSConfigFile) != orig {
		t.Fatal("not restored")
	}
	// the preview is used up, and no write of the app belongs to this backup: a restore without a new
	// preview is refused
	writeText(t, w.p.AWSConfigFile, "[profile later]\n")
	w.c.files.note(w.p.AWSConfigFile, []byte("[profile later]\n"))
	_, err = w.sys.RestoreBackup(e1.ID, false)
	wantRestoreCode(t, err, api.ErrConfigChanged)
}

func TestRestoreUndoFollowsOwnWrite(t *testing.T) {
	w := newWorld(t)
	e1, err := w.c.Backup.Snapshot(w.p.AWSConfigFile, "~/.aws/config", api.BackupAWSConfig, api.ReasonAddProfiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.c.files.prime()

	// the app writes the file and tells the watcher (what every own write does)
	appWrote := w.read(w.p.AWSConfigFile) + "\n[profile added-by-app]\nregion = us-east-1\n"
	writeText(t, w.p.AWSConfigFile, appWrote)
	w.c.files.note(w.p.AWSConfigFile, []byte(appWrote))
	if _, err := w.sys.RestoreBackup(e1.ID, false); err != nil {
		t.Fatalf("undo right after an own write must work: %v", err)
	}

	// an external edit the watcher has not recorded as an own write
	appWrote2 := w.read(w.p.AWSConfigFile) + "\n[profile second]\nregion = us-east-1\n"
	writeText(t, w.p.AWSConfigFile, appWrote2)
	w.c.files.note(w.p.AWSConfigFile, []byte(appWrote2))
	external := appWrote2 + "# edited by hand\n"
	writeText(t, w.p.AWSConfigFile, external)
	_, err = w.sys.RestoreBackup(e1.ID, false)
	wantRestoreCode(t, err, api.ErrConfigChanged)
	if w.read(w.p.AWSConfigFile) != external {
		t.Fatal("the file must be left as it is")
	}
}

func TestRestoreRejectsTargetsOutsideTheAppsFiles(t *testing.T) {
	w := newWorld(t)
	other := filepath.Join(t.TempDir(), "evil")
	writeText(t, other, "original\n")
	const goodID = "20261104T021800Z-aws-config-0a1b"

	for name, c := range map[string]struct {
		kind   api.BackupKind
		target string
	}{
		"elsewhere":            {api.BackupAWSConfig, other},
		"kind and path differ": {api.BackupAWSConfig, w.p.AWSCredentialsFile},
		"credentials slot":     {api.BackupAWSCredentials, w.p.AWSConfigFile},
		"git global slot":      {api.BackupGitGlobal, w.p.AWSConfigFile},
		"relative":             {api.BackupAWSConfig, "config"},
		"git local outside":    {api.BackupGitLocal, other},
		"git local config":     {api.BackupGitLocal, filepath.Join(w.p.Home, "unknown-repo", ".git", "config")},
		"unknown kind":         {api.BackupKind("other"), w.p.AWSConfigFile},
	} {
		plantBackup(t, w, goodID, c.kind, c.target, "restored\n")
		before := w.read(w.p.AWSConfigFile)
		if _, err := w.sys.PreviewRestore(goodID); err == nil {
			t.Errorf("%s: preview must fail", name)
		}
		if _, err := w.sys.RestoreBackup(goodID, true); err == nil {
			t.Errorf("%s: restore must fail", name)
		}
		if w.read(w.p.AWSConfigFile) != before {
			t.Errorf("%s: the AWS config changed", name)
		}
	}
	if w.read(other) != "original\n" {
		t.Fatal("a file outside the app's files was overwritten")
	}
}

func TestRestoreAcceptsKnownRepositoryConfig(t *testing.T) {
	w := newWorld(t)
	dir := w.gitRepo("web-app", ccURL("web-app"))
	if _, err := w.c.Repos.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, ".git", "config")
	cur := w.read(cfg)
	const id = "20261104T021800Z-git-local-0a1b"
	plantBackup(t, w, id, api.BackupGitLocal, cfg, "[core]\n\tbare = false\n")
	if _, err := w.sys.PreviewRestore(id); err != nil {
		t.Fatalf("a known repository config is a valid target: %v", err)
	}
	if _, err := w.sys.RestoreBackup(id, false); err != nil {
		t.Fatal(err)
	}
	if w.read(cfg) == cur || !strings.Contains(w.read(cfg), "bare = false") {
		t.Fatal("not restored")
	}

	// a file named config that is not inside .git of that repository is refused
	notGit := filepath.Join(dir, "config")
	writeText(t, notGit, "x\n")
	plantBackup(t, w, id, api.BackupGitLocal, notGit, "y\n")
	if _, err := w.sys.RestoreBackup(id, true); err == nil {
		t.Fatal("must be refused")
	}
	if w.read(notGit) != "x\n" {
		t.Fatal("overwritten")
	}
}

// The watcher records external edits as the new known state after its next pass (about 3 s); the undo of an
// earlier app write must still notice them.
func TestRestoreUndoRefusesExternalEditAfterWatcherPoll(t *testing.T) {
	w := newWorld(t)
	w.c.files.settle = 0
	e1, err := w.c.Backup.Snapshot(w.p.AWSConfigFile, "~/.aws/config", api.BackupAWSConfig, api.ReasonAddProfiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.c.files.prime()
	appWrote := w.read(w.p.AWSConfigFile) + "\n[profile added-by-app]\nregion = us-east-1\n"
	writeText(t, w.p.AWSConfigFile, appWrote)
	w.c.files.note(w.p.AWSConfigFile, []byte(appWrote))

	external := appWrote + "# edited by hand\n"
	writeText(t, w.p.AWSConfigFile, external)
	if got := w.c.files.poll(); len(got) != 1 {
		t.Fatalf("the watcher must report the external edit: %v", got)
	}
	_, err = w.sys.RestoreBackup(e1.ID, false)
	wantRestoreCode(t, err, api.ErrConfigChanged)
	if w.read(w.p.AWSConfigFile) != external {
		t.Fatal("the file must be left as it is")
	}
}

func TestRestoreUndoRefusedWithoutRecordForWatchedFile(t *testing.T) {
	w := newWorld(t)
	orig := w.read(w.p.AWSConfigFile)
	e1, err := w.c.Backup.Snapshot(w.p.AWSConfigFile, "~/.aws/config", api.BackupAWSConfig, api.ReasonAddProfiles, nil)
	if err != nil {
		t.Fatal(err)
	}
	// the write that belongs to the backup was never reported (for example the app was restarted since)
	writeText(t, w.p.AWSConfigFile, orig+"\n[profile x]\n")
	_, err = w.sys.RestoreBackup(e1.ID, false)
	wantRestoreCode(t, err, api.ErrConfigChanged)
}
