package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
)

const credFixture = "[acme]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse0000000000000000\n\n[globex]\naws_access_key_id = AKIAI44QH8DHBEXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse1111111111111111\n"

func openStore(t *testing.T, retention int) *Store {
	t.Helper()
	n := 0
	base := time.Date(2026, 11, 4, 2, 18, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "backups"), func() int { return retention }, func() time.Time {
		n++
		return base.Add(time.Duration(n) * time.Second)
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSnapshotRedactsCredentials(t *testing.T) {
	s := openStore(t, 20)
	p := writeTemp(t, "credentials", credFixture)
	e, err := s.Snapshot(p, "~/.aws/credentials", api.BackupAWSCredentials, api.ReasonRemoveKey, map[string]string{"profile": "acme"})
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == "" || !e.Redacted || e.Kind != api.BackupAWSCredentials || e.ReasonParams["profile"] != "acme" {
		t.Fatalf("%+v", e)
	}
	if !strings.Contains(e.ID, "-aws-credentials-") || !strings.HasPrefix(e.ID, "20261104T") {
		t.Fatal(e.ID)
	}
	fp, _ := s.FilePath(e.ID)
	b, _ := os.ReadFile(fp)
	if strings.Contains(string(b), "fakeSecret") || strings.Contains(string(b), "AKIAIOSF") || !strings.Contains(string(b), Placeholder) {
		t.Fatalf("not redacted: %s", b)
	}
	fi, _ := os.Stat(fp)
	if fi.Mode().Perm() != 0o600 {
		t.Fatal(fi.Mode())
	}
	idx, _ := os.ReadFile(filepath.Join(s.Dir(), "index.json"))
	if strings.Contains(string(idx), "fakeSecret") {
		t.Fatal("secret in index")
	}
	got, content, err := s.Get(e.ID)
	if err != nil || got.ID != e.ID || string(content) != string(b) {
		t.Fatal(err)
	}
	if _, _, err := s.Get("nope"); err == nil || api.AsAppError(err).Code != api.ErrBackupNotFound {
		t.Fatal(err)
	}
}

func TestRetentionPerPath(t *testing.T) {
	s := openStore(t, 20)
	a := writeTemp(t, "config", "[default]\nregion = x\n")
	b := writeTemp(t, "config", "[default]\nregion = y\n")
	if _, err := s.Snapshot(b, "b", api.BackupAWSConfig, api.ReasonMigrate, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		if _, err := s.Snapshot(a, "a", api.BackupAWSConfig, api.ReasonMigrate, nil); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.List()
	na, nb := 0, 0
	for _, e := range list {
		if e.Path == a {
			na++
		} else if e.Path == b {
			nb++
		}
	}
	if na != 20 || nb != 1 {
		t.Fatalf("a=%d b=%d", na, nb)
	}
	files, _ := filepath.Glob(filepath.Join(s.Dir(), "*.bak"))
	if len(files) != 21 {
		t.Fatalf("%d bak files", len(files))
	}
	if list[0].CreatedAt < list[1].CreatedAt {
		t.Fatal("not newest first")
	}
	// retention 0 keeps all
	s0 := openStore(t, 0)
	for i := 0; i < 25; i++ {
		s0.Snapshot(a, "a", api.BackupAWSConfig, api.ReasonMigrate, nil)
	}
	if l, _ := s0.List(); len(l) != 25 {
		t.Fatalf("%d", len(l))
	}
}

func TestMergeRedactedRestoresFromCurrent(t *testing.T) {
	red, _ := Redact(api.BackupAWSCredentials, []byte(credFixture))
	cur := "[globex]\naws_access_key_id = AKIANEWKEY000EXAMPLE\naws_secret_access_key = newSecret\n[acme]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = fakeSecretValueDoNotUse0000000000000000\n"
	merged, lost, err := MergeRedacted(red, []byte(cur))
	if err != nil || len(lost) != 0 {
		t.Fatal(err, lost)
	}
	if string(merged) != credFixture[:strings.Index(credFixture, "[globex]")]+"[globex]\naws_access_key_id = AKIANEWKEY000EXAMPLE\naws_secret_access_key = newSecret\n" {
		t.Fatalf("%s", merged)
	}
}

func TestMergeRedactedReportsLost(t *testing.T) {
	red, _ := Redact(api.BackupAWSCredentials, []byte(credFixture))
	merged, lost, _ := MergeRedacted(red, []byte("[globex]\naws_access_key_id = AKIANEWKEY000EXAMPLE\naws_secret_access_key = s\n"))
	if fmt.Sprint(lost) != "[acme]" {
		t.Fatalf("lost %v", lost)
	}
	if strings.Contains(string(merged), Placeholder) || strings.Contains(string(merged), "fakeSecret") {
		t.Fatalf("%s", merged)
	}
	if !strings.Contains(string(merged), "[acme]\n") {
		t.Fatal("header must stay")
	}
}

func TestSnapshotMissingFile(t *testing.T) {
	s := openStore(t, 20)
	e, err := s.Snapshot(filepath.Join(t.TempDir(), "nope"), "nope", api.BackupAWSConfig, api.ReasonMigrate, nil)
	if err != nil || e.ID != "" {
		t.Fatalf("%+v %v", e, err)
	}
	if l, _ := s.List(); len(l) != 0 {
		t.Fatal("no entry expected")
	}
}

func TestWriteFileAtomicTightensMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "f")
	if err := WriteFileAtomic(p, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Chmod(p, 0o640)
	if err := WriteFileAtomic(p, []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	b, _ := os.ReadFile(p)
	if fi.Mode().Perm() != 0o600 || string(b) != "bb" {
		t.Fatal("group and other bits must be cleared:", fi.Mode(), string(b))
	}
	ents, _ := os.ReadDir(filepath.Dir(p))
	if len(ents) != 1 {
		t.Fatalf("temp file left: %v", ents)
	}
	// symlink is followed, link survives
	target := filepath.Join(t.TempDir(), "real")
	os.WriteFile(target, []byte("x"), 0o600)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	WriteFileAtomic(link, []byte("y"), 0o600)
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
	if b, _ := os.ReadFile(target); string(b) != "y" {
		t.Fatal("target not updated")
	}
}

func TestGitRedaction(t *testing.T) {
	in := "[remote \"origin\"]\n\turl = https://alice:s3cretpw@git-codecommit.us-east-1.amazonaws.com/v1/repos/x\n[http]\n\textraheader = Authorization: Basic abc\n[credential]\n\thelper = !aws codecommit credential-helper $@\n\tmyToken = zzz\n"
	out, red := Redact(api.BackupGitLocal, []byte(in))
	s := string(out)
	if !red || strings.Contains(s, "s3cretpw") || strings.Contains(s, "Basic abc") || strings.Contains(s, "zzz") {
		t.Fatalf("not redacted:\n%s", s)
	}
	if !strings.Contains(s, "https://alice:"+Placeholder+"@git-codecommit") {
		t.Fatal("userinfo form")
	}
	if !strings.Contains(s, "helper = !aws codecommit credential-helper $@") {
		t.Fatal("helper must stay")
	}
	if _, red := Redact(api.BackupGitGlobal, []byte("[user]\n\tname = x\n")); red {
		t.Fatal("nothing to redact")
	}
}

func TestAWSRedactionAcceptsColonSeparator(t *testing.T) {
	in := "[acme]\naws_access_key_id: AKIAIOSFODNN7EXAMPLE\naws_secret_access_key: fakeSecretValueDoNotUse0000\naws_session_token : FAKE0123456789abcdef==\naws_security_token:FAKEtok\nregion: us-east-1\n"
	out, red := Redact(api.BackupAWSCredentials, []byte(in))
	s := string(out)
	if !red {
		t.Fatal("expected redaction")
	}
	for _, secret := range []string{"AKIAIOSF", "fakeSecret", "FAKE0123", "FAKEtok"} {
		if strings.Contains(s, secret) {
			t.Fatalf("%q survived:\n%s", secret, s)
		}
	}
	if !strings.Contains(s, "region: us-east-1") {
		t.Fatalf("other keys stay:\n%s", s)
	}
	if got := strings.Count(s, Placeholder); got != 4 {
		t.Fatalf("%d placeholders:\n%s", got, s)
	}
	// "=" still works and the config kind behaves the same
	out, _ = Redact(api.BackupAWSConfig, []byte("[profile acme]\naws_secret_access_key=abc\n"))
	if strings.Contains(string(out), "abc") {
		t.Fatalf("%s", out)
	}
}

func TestGitRedactionHidesEveryUserinfo(t *testing.T) {
	in := "[remote \"origin\"]\n\turl = https://ghp_0123456789abcdef0123@github.com/acme/x\n" +
		"[url \"https://deploy@example.com/\"]\n\tinsteadOf = https://example.com/\n" +
		"[remote \"two\"]\n\turl = https://alice:s3cretpw@host.example.com/y\n" +
		"[remote \"three\"]\n\turl = ssh://git@github.com/acme/z\n" +
		"[remote \"plain\"]\n\turl = https://github.com/acme/w\n"
	for _, kind := range []api.BackupKind{api.BackupGitGlobal, api.BackupGitLocal} {
		out, red := Redact(kind, []byte(in))
		s := string(out)
		if !red {
			t.Fatal("expected redaction")
		}
		for _, secret := range []string{"ghp_0123", "deploy@", "s3cretpw", "git@github"} {
			if strings.Contains(s, secret) {
				t.Fatalf("%s: %q survived:\n%s", kind, secret, s)
			}
		}
		for _, want := range []string{
			"url = https://" + Placeholder + "@github.com/acme/x",
			"[url \"https://" + Placeholder + "@example.com/\"]",
			"url = https://alice:" + Placeholder + "@host.example.com/y",
			"url = https://github.com/acme/w",
		} {
			if !strings.Contains(s, want) {
				t.Fatalf("%s: missing %q in\n%s", kind, want, s)
			}
		}
		again, _ := Redact(kind, out)
		if string(again) != s {
			t.Fatal("redaction must be idempotent")
		}
	}
	// the AWS kinds do not touch URLs
	if _, red := Redact(api.BackupAWSConfig, []byte("[sso-session x]\nsso_start_url = https://acme.awsapps.com/start\n")); red {
		t.Fatal("nothing to redact in an AWS config")
	}
}

func TestMergeRedactedKeepsRepeatedKeysInOrder(t *testing.T) {
	orig := "[http]\n\textraheader = Authorization: Basic AAA\n\textraheader = Authorization: Basic BBB\n" +
		"[url \"https://x.example.com/\"]\n\tinsteadOf = https://a.example.com/\n\tinsteadOf = https://u:pw1@b.example.com/\n\tinsteadOf = https://u:pw2@c.example.com/\n"
	red, _ := Redact(api.BackupGitGlobal, []byte(orig))
	if strings.Contains(string(red), "AAA") || strings.Contains(string(red), "pw1") {
		t.Fatalf("%s", red)
	}
	merged, lost, err := MergeRedacted(red, []byte(orig))
	if err != nil || len(lost) != 0 {
		t.Fatal(err, lost)
	}
	if string(merged) != orig {
		t.Fatalf("merged:\n%s\nwant:\n%s", merged, orig)
	}
	// the current file changed values but kept the order
	cur := strings.NewReplacer("AAA", "CCC", "BBB", "DDD", "pw1", "np1", "pw2", "np2").Replace(orig)
	merged, lost, _ = MergeRedacted(red, []byte(cur))
	if len(lost) != 0 || !strings.Contains(string(merged), "Basic CCC\n\textraheader = Authorization: Basic DDD") ||
		!strings.Contains(string(merged), "u:np1@b.example.com/\n\tinsteadOf = https://u:np2@c.example.com/") {
		t.Fatalf("%v\n%s", lost, merged)
	}
	// only one of two repeated values is still present: the second one is lost, not copied from the first
	merged, lost, _ = MergeRedacted(red, []byte("[http]\n\textraheader = Authorization: Basic ZZZ\n"))
	if len(lost) == 0 || lost[0] != "http" {
		t.Fatalf("the http section lost a value: %v", lost)
	}
	if strings.Contains(string(merged), Placeholder) {
		t.Fatalf("the placeholder must never be written back:\n%s", merged)
	}
	if strings.Count(string(merged), "Basic ZZZ") != 1 {
		t.Fatalf("a value must not be used twice:\n%s", merged)
	}
}

func TestMergeRedactedDropsSectionWithRedactedHeader(t *testing.T) {
	orig := "[user]\n\tname = Alex\n[url \"https://u:pw@h.example.com/\"]\n\tinsteadOf = https://h.example.com/\n\tpushInsteadOf = https://p.example.com/\n[core]\n\teditor = vi\n"
	red, _ := Redact(api.BackupGitGlobal, []byte(orig))
	if !strings.Contains(string(red), "[url \"https://u:"+Placeholder+"@h.example.com/\"]") {
		t.Fatalf("header must be redacted:\n%s", red)
	}
	// the current file no longer has that section
	merged, lost, err := MergeRedacted(red, []byte("[user]\n\tname = Alex\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lost) != 1 || !strings.HasPrefix(lost[0], "url ") {
		t.Fatalf("lost: %v", lost)
	}
	s := string(merged)
	if strings.Contains(s, Placeholder) || strings.Contains(s, "insteadOf") || strings.Contains(s, "pushInsteadOf") {
		t.Fatalf("the whole section must be dropped:\n%s", s)
	}
	if !strings.Contains(s, "[user]\n\tname = Alex\n") || !strings.Contains(s, "[core]\n\teditor = vi\n") {
		t.Fatalf("other sections stay:\n%s", s)
	}
}

func TestWriteFileAtomicClearsGroupAndOtherBits(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "credentials")
	if err := os.WriteFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Chmod(p, 0o644)
	if err := WriteFileAtomic(p, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("existing 0644 file: %v", fi.Mode())
	}
	os.Chmod(p, 0o755)
	WriteFileAtomic(p, []byte("c"), 0o600)
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o700 {
		t.Fatalf("owner bits are kept, group/other cleared: %v", fi.Mode())
	}
	q := filepath.Join(dir, "new")
	if err := WriteFileAtomic(q, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(q); fi.Mode().Perm() != 0o600 {
		t.Fatalf("new file: %v", fi.Mode())
	}
}

func TestIndexIgnoresEntriesWithUnexpectedIDs(t *testing.T) {
	s := openStore(t, 1)
	good := writeTemp(t, "config", "[profile acme]\nregion = us-east-1\n")
	e1, err := s.Snapshot(good, "~/.aws/config", api.BackupAWSConfig, api.ReasonMigrate, nil)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(s.Dir()), "outside.bak")
	if err := os.WriteFile(outside, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	evil := []api.BackupEntry{
		{ID: "../outside", Path: good, Kind: api.BackupAWSConfig},
		{ID: "../../x", Path: good, Kind: api.BackupAWSConfig},
		{ID: "/abs/path", Path: good, Kind: api.BackupAWSConfig},
		{ID: "20261104T021800Z-aws-config-zz", Path: good, Kind: api.BackupAWSConfig},
		{ID: "", Path: good, Kind: api.BackupAWSConfig},
	}
	b, _ := json.Marshal(append(evil, e1))
	if err := os.WriteFile(filepath.Join(s.Dir(), "index.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 || list[0].ID != e1.ID {
		t.Fatalf("%+v %v", list, err)
	}
	for _, id := range []string{"../outside", "../../x", "/abs/path"} {
		if _, err := s.FilePath(id); err == nil {
			t.Fatalf("%q must not resolve", id)
		}
		if _, _, err := s.Get(id); err == nil {
			t.Fatalf("%q must not resolve", id)
		}
	}
	// retention (keep 1) with more snapshots must not touch anything outside the directory
	for i := 0; i < 3; i++ {
		if _, err := s.Snapshot(good, "~/.aws/config", api.BackupAWSConfig, api.ReasonMigrate, nil); err != nil {
			t.Fatal(err)
		}
	}
	if b, err := os.ReadFile(outside); err != nil || string(b) != "keep me" {
		t.Fatalf("a file outside the backup directory was touched: %v", err)
	}
	after, _ := s.List()
	if len(after) != 1 {
		t.Fatalf("%d entries", len(after))
	}
}
