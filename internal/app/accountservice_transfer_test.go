package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// ssoSelection picks every SSO profile of the first real portal from the state.
func ssoSelection(t *testing.T, st api.AccountsState) api.ExportRequest {
	t.Helper()
	req := api.ExportRequest{Portals: []string{}, Profiles: []string{}, IncludeExtras: true}
	for _, p := range st.Portals {
		if !p.Draft && !p.LegacyFormat {
			req.Portals = append(req.Portals, p.Name)
		}
	}
	for _, p := range st.Profiles {
		if p.Kind == api.ProfileKindSSO {
			req.Profiles = append(req.Profiles, p.Name)
		}
	}
	if len(req.Portals) == 0 || len(req.Profiles) == 0 {
		t.Fatal("fixture has no SSO profile to export")
	}
	return req
}

func TestExportImportServiceDialogs(t *testing.T) {
	w := newWorld(t)
	st := ok(w.acc.GetState(bg))
	req := ssoSelection(t, st)

	// preview and copy: no dialog, no file
	built := ok(w.acc.BuildExport(bg, req))
	if built.Path != "" || len(w.rt.dialogs) != 0 || !strings.Contains(built.JSON, `"format": "aws-sso-hub.accounts"`) {
		t.Fatalf("%+v dialogs=%v", built.Path, w.rt.dialogs)
	}

	// save: the path comes from the native dialog only
	out := filepath.Join(t.TempDir(), "accounts.json")
	w.rt.saveTo = out
	saved := ok(w.acc.SaveExport(bg, req))
	if saved.Path != out || w.read(out) != built.JSON {
		t.Fatalf("saved %q", saved.Path)
	}
	if len(w.rt.dialogs) != 1 || !strings.HasPrefix(w.rt.dialogs[0], "save:aws-sso-hub-accounts-") || !strings.HasSuffix(w.rt.dialogs[0], ".json:*.json") {
		t.Fatalf("dialogs %v", w.rt.dialogs)
	}

	// cancelled save, and nothing selected (the dialog must not even open)
	w.rt.saveTo = ""
	if ae := fails(w.acc.SaveExport(bg, req)); ae.Code != api.ErrCancelled {
		t.Fatalf("%+v", ae)
	}
	n := len(w.rt.dialogs)
	if ae := fails(w.acc.SaveExport(bg, api.ExportRequest{})); ae.Code != api.ErrInvalidInput || len(w.rt.dialogs) != n {
		t.Fatalf("%+v dialogs=%v", ae, w.rt.dialogs)
	}

	// open: native dialog, then preview and apply (everything is already there)
	w.rt.openFrom = ""
	if ae := fails(w.acc.OpenImportFile()); ae.Code != api.ErrCancelled {
		t.Fatalf("%+v", ae)
	}
	w.rt.openFrom = out
	src := ok(w.acc.OpenImportFile())
	if src.Name != "accounts.json" || src.JSON != built.JSON {
		t.Fatalf("%+v", src.Name)
	}
	imp := api.ImportRequest{JSON: src.JSON, ImportExtras: true}
	prev := ok(w.acc.PreviewImport(imp))
	for _, it := range prev.Items {
		if it.Action != api.ImportActionSame {
			t.Fatalf("%+v", it)
		}
	}
	before := w.read(w.p.AWSConfigFile)
	res := ok(w.acc.ApplyImport(bg, imp, prev.BaseHash))
	if res.Added+res.Replaced+res.Renamed != 0 || w.read(w.p.AWSConfigFile) != before {
		t.Fatalf("%+v", res)
	}

	// a deleted profile comes back; accounts:changed tells the UI
	gone := req.Profiles[0]
	pv := ok(w.acc.PreviewRemoveProfile(gone))
	ok(w.acc.RemoveProfile(gone, pv.BaseHash))
	events := w.rt.count(api.EventAccountsChanged)
	prev = ok(w.acc.PreviewImport(imp))
	if got := prev.Items; len(got) == 0 || prev.ToWrite != 1 {
		t.Fatalf("%+v", prev)
	}
	res = ok(w.acc.ApplyImport(bg, imp, prev.BaseHash))
	if res.Added != 1 || res.Write.BackupID == "" || w.rt.count(api.EventAccountsChanged) <= events {
		t.Fatalf("%+v", res)
	}
	st = ok(w.acc.GetState(bg))
	found := false
	for _, p := range st.Profiles {
		found = found || p.Name == gone
	}
	if !found {
		t.Fatal("imported profile missing from the state")
	}
	var reason api.BackupReason
	for _, b := range ok(w.sys.ListBackups()) {
		if b.Reason == api.ReasonImport {
			reason = b.Reason
		}
	}
	if reason != api.ReasonImport {
		t.Fatal("the import must leave a backup with reason import")
	}

	// the undo of that import works after a watcher pass, and refuses once the file was edited by hand
	w.c.files.settle = 0
	w.c.files.poll()
	afterImport := w.read(w.p.AWSConfigFile)
	handEdited := afterImport + "\n# edited by hand\n"
	if err := os.WriteFile(w.p.AWSConfigFile, []byte(handEdited), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := w.c.files.poll(); len(got) != 1 {
		t.Fatalf("watcher: %v", got)
	}
	if ae := fails(w.sys.RestoreBackup(res.Write.BackupID, false)); ae.Code != api.ErrConfigChanged {
		t.Fatalf("undo after a hand edit: %+v", ae)
	}
	if err := os.WriteFile(w.p.AWSConfigFile, []byte(afterImport), 0o600); err != nil {
		t.Fatal(err)
	}
	ok(w.sys.RestoreBackup(res.Write.BackupID, false))
	if err := os.WriteFile(w.p.AWSConfigFile, []byte(afterImport), 0o600); err != nil {
		t.Fatal(err)
	}
	w.c.files.note(w.p.AWSConfigFile, []byte(afterImport))

	// a stale preview is refused
	imp2 := api.ImportRequest{JSON: strings.Replace(src.JSON, gone, gone+"-copy", -1)}
	prev2 := ok(w.acc.PreviewImport(imp2))
	if err := os.WriteFile(w.p.AWSConfigFile, []byte(w.read(w.p.AWSConfigFile)+"\n# edited elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ae := fails(w.acc.ApplyImport(bg, imp2, prev2.BaseHash)); ae.Code != api.ErrConfigChanged {
		t.Fatalf("%+v", ae)
	}

	// the file the app wrote and every result carry no secret
	corpus := append([]string{w.read(out)}, w.recorded...)
	for _, text := range corpus {
		for _, secret := range secretStrings {
			if strings.Contains(text, secret) {
				t.Fatalf("secret %q in %.200s", secret, text)
			}
		}
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(w.read(out)), &doc); err != nil {
		t.Fatal(err)
	}
}

func TestImportFileTooLarge(t *testing.T) {
	w := newWorld(t)
	big := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", 1<<20+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	w.rt.openFrom = big
	if ae := fails(w.acc.OpenImportFile()); ae.Code != api.ErrImportFormat || ae.Params["reason"] != "size" {
		t.Fatalf("%+v", ae)
	}
}
