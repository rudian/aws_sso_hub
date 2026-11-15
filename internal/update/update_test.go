package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func keys(t *testing.T) (cur, next ed25519.PrivateKey, pubs []ed25519.PublicKey) {
	t.Helper()
	p1, k1, _ := ed25519.GenerateKey(rand.Reader)
	p2, k2, _ := ed25519.GenerateKey(rand.Reader)
	return k1, k2, []ed25519.PublicKey{p1, p2}
}

func manifestJSON(t *testing.T, mut func(*Manifest)) []byte {
	t.Helper()
	sum := sha256.Sum256([]byte("archive"))
	m := Manifest{Version: "1.0.0-rc.2", OS: "darwin", Arch: "arm64", File: "Codepass-darwin-arm64.zip", Size: 7, SHA256: hex.EncodeToString(sum[:]), Notes: "n"}
	if mut != nil {
		mut(&m)
	}
	b, _ := json.Marshal(m)
	return b
}

func TestVerifyManifest(t *testing.T) {
	cur, next, pubs := keys(t)
	_, evil, _ := ed25519.GenerateKey(rand.Reader)
	raw := manifestJSON(t, nil)

	if _, err := VerifyManifest(raw, SignManifest(cur, raw), pubs); err != nil {
		t.Fatalf("current key: %v", err)
	}
	if _, err := VerifyManifest(raw, SignManifest(next, raw), pubs); err != nil {
		t.Fatalf("next key (rotation): %v", err)
	}
	refuse := map[string]struct {
		raw []byte
		sig string
	}{
		"unpinned key":                     {raw, SignManifest(evil, raw)},
		"tampered body":                    {append(append([]byte{}, raw[:len(raw)-1]...), []byte(` ,"x":1}`)...), SignManifest(cur, raw)},
		"empty signature":                  {raw, ""},
		"garbage":                          {raw, "AAAA"},
		"signed raw without domain prefix": {raw, "AAAA"},
	}
	for name, c := range refuse {
		if _, err := VerifyManifest(c.raw, c.sig, pubs); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// unknown field, even correctly signed
	extra := []byte(strings.Replace(string(raw), `"notes"`, `"url":"http://evil","notes"`, 1))
	if _, err := VerifyManifest(extra, SignManifest(cur, extra), pubs); err == nil {
		t.Error("unknown field accepted")
	}
	// short digest would disable Wails' digest comparison
	for name, mut := range map[string]func(*Manifest){
		"empty sha":    func(m *Manifest) { m.SHA256 = "" },
		"short sha":    func(m *Manifest) { m.SHA256 = "abcd" },
		"path in file": func(m *Manifest) { m.File = "../x.zip" },
		"dot file":     func(m *Manifest) { m.File = ".hidden" },
		"bad semver":   func(m *Manifest) { m.Version = "rc2" },
		"zero size":    func(m *Manifest) { m.Size = 0 },
	} {
		b := manifestJSON(t, mut)
		if _, err := VerifyManifest(b, SignManifest(cur, b), pubs); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// domain separation: a bare signature over the manifest bytes (valid base64, valid length, made by a pinned
	// key) is not accepted, so a signature made for another purpose cannot be replayed as a manifest signature
	bare := base64.StdEncoding.EncodeToString(ed25519.Sign(cur, raw))
	if _, err := VerifyManifest(raw, bare, pubs); err == nil || !strings.Contains(err.Error(), "does not match any pinned key") {
		t.Errorf("signature without domain prefix: %v", err)
	}
}

func TestRules(t *testing.T) {
	m := &Manifest{Version: "1.0.0-rc.2", OS: "darwin", Arch: "arm64"}
	base := Rules{Tag: "v1.0.0-rc.2", Running: "1.0.0-rc.1", Highest: "1.0.0-rc.1", OS: "darwin", Arch: "arm64"}
	if err := m.Check(base); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Rules){
		"old archive under new tag": func(r *Rules) { r.Tag = "v1.0.0-rc.3" },
		"not newer than running":    func(r *Rules) { r.Running = "1.0.0-rc.2" },
		"older than running":        func(r *Rules) { r.Running = "1.0.0" },
		"below highest installed":   func(r *Rules) { r.Highest = "1.0.0-rc.5" },
		"wrong arch":                func(r *Rules) { r.Arch = "amd64" },
		"wrong os":                  func(r *Rules) { r.OS = "windows" },
		"tag not semver":            func(r *Rules) { r.Tag = "latest" },
		"tag with build metadata":   func(r *Rules) { r.Tag = "v1.0.0-rc.2+evil" },
		"running not semver":        func(r *Rules) { r.Running = "dev" },
		"stored highest invalid":    func(r *Rules) { r.Highest = "garbage" },
	}
	for name, mut := range cases {
		r := base
		mut(&r)
		if err := m.Check(r); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// prerelease ordering that matters for the rc channel
	for _, c := range []struct{ a, b string }{{"1.0.0-rc.10", "1.0.0-rc.2"}, {"1.0.0", "1.0.0-rc.9"}} {
		mm := &Manifest{Version: c.a, OS: "darwin", Arch: "arm64"}
		if err := mm.Check(Rules{Tag: "v" + c.a, Running: c.b, OS: "darwin", Arch: "arm64"}); err != nil {
			t.Errorf("%s should be newer than %s: %v", c.a, c.b, err)
		}
	}
	// the format the plan avoids: "rc10" is one identifier and sorts below "rc2"; tags must use "rc.N"
	mm := &Manifest{Version: "1.0.0-rc10", OS: "darwin", Arch: "arm64"}
	if err := mm.Check(Rules{Tag: "v1.0.0-rc10", Running: "1.0.0-rc2", OS: "darwin", Arch: "arm64"}); err == nil {
		t.Error("rc10 must sort below rc2 (documented hazard); the release script must write rc.N")
	}
}

func TestHighestNeverLowers(t *testing.T) {
	h := Highest{Path: filepath.Join(t.TempDir(), "s", "highest")}
	if v, err := h.Get(); v != "" || err != nil {
		t.Fatalf("first run: %q, %v", v, err)
	}
	for _, v := range []string{"1.0.0-rc.2", "1.0.0-rc.1", "1.0.0-rc.3", "garbage"} {
		_ = h.Record(v)
	}
	if got, err := h.Get(); got != "1.0.0-rc.3" || err != nil {
		t.Fatalf("highest = %q, %v", got, err)
	}
	_ = h.Record("1.0.0-rc.10")
	if got, _ := h.Get(); got != "1.0.0-rc.10" {
		t.Fatalf("rc.10 must raise rc.3, got %q", got)
	}
}

func TestHighestFailsClosed(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{"garbage": "not a version\n", "empty": "", "huge": strings.Repeat("1", 1000)} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		h := Highest{Path: p}
		if _, err := h.Get(); !errors.Is(err, ErrHighestUnreadable) {
			t.Errorf("%s: Get err = %v, want ErrHighestUnreadable", name, err)
		}
		if err := h.Record("1.0.0"); !errors.Is(err, ErrHighestUnreadable) {
			t.Errorf("%s: Record must not overwrite an unreadable file: %v", name, err)
		}
		if b, _ := os.ReadFile(p); string(b) != content {
			t.Errorf("%s: file was changed", name)
		}
	}
	// a directory where the file should be is unreadable too
	d := filepath.Join(dir, "adir")
	_ = os.Mkdir(d, 0o700)
	if _, err := (Highest{Path: d}).Get(); !errors.Is(err, ErrHighestUnreadable) {
		t.Errorf("directory: %v", err)
	}
}

// Plan 4.14 item 6: HandleHelperMode is the first statement of main(). Each file is its own subtest so that
// skipping main.go (before the wiring task adds the import) never hides the check of the test app.
func TestHandleHelperModeIsFirst(t *testing.T) {
	for _, f := range []string{"integration/testapp/main.go", "../../main.go"} {
		t.Run(f, func(t *testing.T) {
			if f == "../../main.go" && !mainImportsUpdater(t, f) {
				t.Skip("main.go does not import the updater yet: the wiring task (F1-W) adds updater.HandleHelperMode() as its first statement and this check then applies")
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, d := range file.Decls {
				fd, ok := d.(*ast.FuncDecl)
				if !ok || fd.Name.Name != "main" || fd.Recv != nil {
					continue
				}
				found = true
				if fd.Body == nil || len(fd.Body.List) == 0 {
					t.Fatalf("%s: main is empty", f)
				}
				es, ok := fd.Body.List[0].(*ast.ExprStmt)
				if !ok {
					t.Fatalf("%s: first statement of main is not a call", f)
				}
				call, ok := es.X.(*ast.CallExpr)
				if !ok {
					t.Fatalf("%s: first statement of main is not a call", f)
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					t.Fatalf("%s: first statement of main must be updater.HandleHelperMode()", f)
				}
				id, ok := sel.X.(*ast.Ident)
				if sel.Sel.Name != "HandleHelperMode" || !ok || id.Name != "updater" {
					t.Fatalf("%s: first statement of main must be updater.HandleHelperMode()", f)
				}
			}
			if !found {
				t.Fatalf("%s: no main()", f)
			}
		})
	}
}

func mainImportsUpdater(t *testing.T, path string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(b), "wails/v3/pkg/updater")
}
