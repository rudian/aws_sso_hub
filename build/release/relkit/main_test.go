package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fixed seed makes the signature a known-answer vector for the format the app verifies.
func testKey() ed25519.PrivateKey { return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, 32)) }

func writeKey(t *testing.T, dir string, priv ed25519.PrivateKey) string {
	t.Helper()
	p := filepath.Join(dir, "k.priv")
	if err := os.WriteFile(p, []byte(hex.EncodeToString(priv)), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func build(t *testing.T, version string) (dir, archive, manifest, sig, key string) {
	t.Helper()
	dir = t.TempDir()
	archive = filepath.Join(dir, "Codepass-"+version+"-darwin-arm64.zip")
	os.WriteFile(archive, []byte("not really a zip"), 0o644)
	manifest, sig = filepath.Join(dir, "m.json"), filepath.Join(dir, "m.json.sig")
	key = writeKey(t, dir, testKey())
	var sink bytes.Buffer
	if err := run([]string{"manifest", "-version", version, "-archive", archive, "-out", manifest}, &sink); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sign", "-key", key, "-in", manifest, "-out", sig}, &sink); err != nil {
		t.Fatal(err)
	}
	return
}

func pubHex() string { return hex.EncodeToString(testKey().Public().(ed25519.PublicKey)) }

func TestRoundTrip(t *testing.T) {
	_, archive, manifest, sig, _ := build(t, "1.0.0-rc.2")
	var out bytes.Buffer
	if err := run([]string{"verify", "-pub", pubHex(), "-in", manifest, "-sig", sig, "-archive", archive}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "version=1.0.0-rc.2") {
		t.Fatalf("output %q", out.String())
	}
}

func TestSignatureFormatIsDomainSeparatedBase64(t *testing.T) {
	_, _, manifest, sig, _ := build(t, "1.0.0")
	raw, _ := os.ReadFile(manifest)
	sb, _ := os.ReadFile(sig)
	s, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sb)))
	if err != nil || len(s) != ed25519.SignatureSize {
		t.Fatal("signature is not 64 bytes of base64")
	}
	pub := testKey().Public().(ed25519.PublicKey)
	if !ed25519.Verify(pub, append([]byte("codepass-update-manifest-v1\n"), raw...), s) {
		t.Fatal("signature is not over domain + manifest bytes")
	}
	if ed25519.Verify(pub, raw, s) {
		t.Fatal("signature must not verify without the domain prefix")
	}
}

func TestRefusals(t *testing.T) {
	dir, archive, manifest, sig, key := build(t, "1.0.0")
	var sink bytes.Buffer
	verify := func(pub, m, s, a string) error {
		return run([]string{"verify", "-pub", pub, "-in", m, "-sig", s, "-archive", a}, &sink)
	}
	// wrong key
	other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{9}, 32)).Public().(ed25519.PublicKey)
	if verify(hex.EncodeToString(other), manifest, sig, archive) == nil {
		t.Fatal("wrong key accepted")
	}
	// second pinned key is accepted
	if err := verify(hex.EncodeToString(other)+","+pubHex(), manifest, sig, archive); err != nil {
		t.Fatalf("second key: %v", err)
	}
	// tampered manifest
	raw, _ := os.ReadFile(manifest)
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, bytes.Replace(raw, []byte(`"1.0.0"`), []byte(`"1.0.1"`), 1), 0o644)
	if verify(pubHex(), bad, sig, archive) == nil {
		t.Fatal("tampered manifest accepted")
	}
	// changed archive
	os.WriteFile(archive, []byte("different"), 0o644)
	if verify(pubHex(), manifest, sig, archive) == nil {
		t.Fatal("changed archive accepted")
	}
	// refuses to sign an unknown field
	extra := filepath.Join(dir, "extra.json")
	os.WriteFile(extra, bytes.Replace(raw, []byte("{"), []byte(`{"extra":1,`), 1), 0o644)
	if run([]string{"sign", "-key", key, "-in", extra, "-out", filepath.Join(dir, "x.sig")}, &sink) == nil {
		t.Fatal("signed a manifest with an unknown field")
	}
}

func TestVersionRules(t *testing.T) {
	for _, v := range []string{"v1.0.0", "1.0", "1.0.0+build", "01.0.0", "1.00.0", "1.0.0-rc.01", "1.0.0-", "1.0.0-rc..1", ""} {
		m := Manifest{Version: v, OS: "darwin", Arch: "arm64", File: "a.zip", Size: 1, SHA256: strings.Repeat("a", 64)}
		if m.wellFormed() == nil {
			t.Errorf("version %q accepted", v)
		}
	}
	for _, v := range []string{"1.0.0", "1.0.0-rc.1", "1.0.0-rc.10", "0.0.1", "1.0.0-rc.0", "1.0.0-beta"} {
		m := Manifest{Version: v, OS: "darwin", Arch: "arm64", File: "a.zip", Size: 1, SHA256: strings.Repeat("a", 64)}
		if err := m.wellFormed(); err != nil {
			t.Errorf("version %q refused: %v", v, err)
		}
	}
}

func TestBadFileNamesAndDigest(t *testing.T) {
	base := Manifest{Version: "1.0.0", OS: "darwin", Arch: "arm64", File: "a.zip", Size: 1, SHA256: strings.Repeat("a", 64)}
	for _, f := range []string{"../a.zip", "a/b.zip", ".hidden", ""} {
		m := base
		m.File = f
		if m.wellFormed() == nil {
			t.Errorf("file %q accepted", f)
		}
	}
	m := base
	m.SHA256 = "abcd"
	if m.wellFormed() == nil {
		t.Error("short sha256 accepted")
	}
}

func TestKeygenNeverOverwrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	var out bytes.Buffer
	if err := run([]string{"keygen", "-dir", dir}, &out); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(filepath.Join(dir, "current.priv"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode %v", fi.Mode().Perm())
	}
	if run([]string{"keygen", "-dir", dir}, &out) == nil {
		t.Fatal("second keygen overwrote a key")
	}
	// the pub subcommand agrees with the pub file
	var pub bytes.Buffer
	if err := run([]string{"pub", "-key", filepath.Join(dir, "current.priv")}, &pub); err != nil {
		t.Fatal(err)
	}
	pb, _ := os.ReadFile(filepath.Join(dir, "current.pub"))
	if strings.TrimSpace(pub.String()) != strings.TrimSpace(string(pb)) {
		t.Fatal("pub mismatch")
	}
	if strings.Contains(out.String(), "current.priv") == false {
		t.Fatal("keygen should say where the private key is")
	}
}

func TestNotesRule(t *testing.T) {
	for name, notes := range map[string]string{"NUL": "a\x00b", "bad utf8": "a\xffb", "too long": strings.Repeat("x", maxNotesBytes+1)} {
		m := Manifest{Version: "1.0.0", OS: "darwin", Arch: "arm64", File: "a.zip", Size: 1, SHA256: strings.Repeat("a", 64), Notes: notes}
		if m.wellFormed() == nil {
			t.Errorf("notes accepted: %s", name)
		}
	}
	dir := t.TempDir()
	nf := filepath.Join(dir, "n.txt")
	os.WriteFile(nf, []byte("a\x00b"), 0o644)
	ar := filepath.Join(dir, "a.zip")
	os.WriteFile(ar, []byte("x"), 0o644)
	if run([]string{"manifest", "-version", "1.0.0", "-archive", ar, "-notes-file", nf, "-out", filepath.Join(dir, "m.json")}, new(bytes.Buffer)) == nil {
		t.Fatal("manifest written with a NUL byte in the notes")
	}
}

// Known-answer vector: a fixed seed, fixed archive bytes and fixed notes give exactly these manifest bytes and
// this signature. A renamed field, a changed order, a different domain string or another encoding breaks it.
func TestGolden(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "Codepass-1.0.0-rc.1-darwin-arm64.zip")
	os.WriteFile(archive, []byte("golden archive bytes"), 0o644)
	notes := filepath.Join(dir, "notes.txt")
	os.WriteFile(notes, []byte("Golden notes\n"), 0o644)
	manifest := filepath.Join(dir, "m.json")
	sig := filepath.Join(dir, "m.json.sig")
	key := writeKey(t, dir, testKey())
	var sink bytes.Buffer
	if err := run([]string{"manifest", "-version", "1.0.0-rc.1", "-archive", archive, "-notes-file", notes, "-out", manifest}, &sink); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"sign", "-key", key, "-in", manifest, "-out", sig}, &sink); err != nil {
		t.Fatal(err)
	}
	gotM, _ := os.ReadFile(manifest)
	gotS, _ := os.ReadFile(sig)
	const wantM = "{\n  \"version\": \"1.0.0-rc.1\",\n  \"os\": \"darwin\",\n  \"arch\": \"arm64\",\n  \"file\": \"Codepass-1.0.0-rc.1-darwin-arm64.zip\",\n  \"size\": 20,\n  \"sha256\": \"721d3702ec406d7318731f154b9b12d037d11f0b86ceeecdbf7c7b4154f7818f\",\n  \"notes\": \"Golden notes\"\n}\n"
	const wantS = "eE0/mtK+yXmKB0N4lpeeM79MgLPk/64oKz+arO7d16urlcg9YU8xGCw6qGf0f4qUJa7vxlCTV8NNSiRIncHqBQ==\n"
	if string(gotM) != wantM {
		t.Errorf("manifest bytes changed:\n%q", gotM)
	}
	if string(gotS) != wantS {
		t.Errorf("signature changed:\n%q", gotS)
	}
}

func TestKeygenRefusesGitWorkTree(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	dir := filepath.Join(repo, "keys", "current")
	if err := run([]string{"keygen", "-dir", dir}, &out); err == nil {
		t.Fatal("keygen wrote a private key inside a git work tree")
	}
	if _, err := os.Stat(dir); err == nil {
		t.Fatal("the folder was created inside the work tree")
	}
	if err := run([]string{"keygen", "-dir", filepath.Join(t.TempDir(), "k")}, &out); err != nil {
		t.Fatalf("keygen outside a work tree failed: %v", err)
	}
}
