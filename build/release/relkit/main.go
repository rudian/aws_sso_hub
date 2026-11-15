// Command relkit is the release-side helper for the Codepass release pipeline (build/release/release.sh).
//
//	relkit keygen   -dir D                         writes D/current.priv and D/current.pub (hex), refuses to overwrite
//	relkit pub      -key F                         prints the public key (hex) of a private key file
//	relkit manifest -version V -archive F -os darwin -arch arm64 [-notes-file F] -out F
//	relkit sign     -key F -in F -out F            detached signature over the manifest bytes (base64)
//	relkit verify   -pub HEX[,HEX] -in F -sig F    checks a signature and the manifest's own fields
//
// Only the standard library is used. The signature format is the one the app verifies (plan 4.14 item 1):
// ed25519 over sigDomain + the exact manifest bytes, base64 encoded. The format is pinned by a golden test that
// holds a known-answer vector (TestGolden), so a change here that the app would not accept fails in CI.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// sigDomain is prepended to the manifest bytes before signing (same value as internal/update).
const sigDomain = "codepass-update-manifest-v1\n"

const maxNotesBytes = 16 << 10

// semverRe is a plain semver with an optional dotted pre-release: 1.0.0, 1.0.0-rc.2. No build metadata.
// Numeric identifiers have no leading zero (1.0.0-rc.01 is refused), as in semver and x/mod/semver.
var semverRe = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(0|[1-9][0-9]*|[0-9A-Za-z]*[A-Za-z][0-9A-Za-z]*)(\.(0|[1-9][0-9]*|[0-9A-Za-z]*[A-Za-z][0-9A-Za-z]*))*)?$`)

// Manifest is the document published next to each platform archive. Field names and order match internal/update.
type Manifest struct {
	Version string `json:"version"`
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
	Notes   string `json:"notes"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "relkit:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("usage: relkit keygen|pub|manifest|sign|verify")
	}
	rest := args[1:]
	switch args[0] {
	case "keygen":
		fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
		dir := fs.String("dir", "", "directory for the key pair (created 0700)")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return keygen(*dir, out)
	case "pub":
		fs := flag.NewFlagSet("pub", flag.ContinueOnError)
		key := fs.String("key", "", "private key file")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		priv, err := readPrivate(*key)
		if err != nil {
			return err
		}
		fmt.Fprintln(out, hex.EncodeToString(priv.Public().(ed25519.PublicKey)))
		return nil
	case "manifest":
		fs := flag.NewFlagSet("manifest", flag.ContinueOnError)
		ver := fs.String("version", "", "version without leading v")
		arc := fs.String("archive", "", "release archive")
		goos := fs.String("os", "darwin", "")
		arch := fs.String("arch", "arm64", "")
		notes := fs.String("notes-file", "", "plain text release notes (default: a one line text)")
		outf := fs.String("out", "", "")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return writeManifest(*ver, *arc, *goos, *arch, *notes, *outf)
	case "sign":
		fs := flag.NewFlagSet("sign", flag.ContinueOnError)
		key := fs.String("key", "", "")
		in := fs.String("in", "", "")
		outf := fs.String("out", "", "")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return signFile(*key, *in, *outf)
	case "verify":
		fs := flag.NewFlagSet("verify", flag.ContinueOnError)
		pub := fs.String("pub", "", "hex public key(s), comma separated")
		in := fs.String("in", "", "")
		sig := fs.String("sig", "", "")
		archive := fs.String("archive", "", "optional: archive whose size and sha256 must match the manifest")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		m, err := verifyFile(*pub, *in, *sig, *archive)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "ok version=%s os=%s arch=%s file=%s size=%d\n", m.Version, m.OS, m.Arch, m.File, m.Size)
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func keygen(dir string, out io.Writer) error {
	if dir == "" {
		return errors.New("keygen: -dir is required")
	}
	if g := insideGitWorkTree(dir); g != "" {
		return fmt.Errorf("keygen: %s is inside a git work tree (%s); a private key there can be committed by accident", dir, g)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	privPath, pubPath := filepath.Join(dir, "current.priv"), filepath.Join(dir, "current.pub")
	for _, p := range []string{privPath, pubPath} {
		if _, err := os.Lstat(p); err == nil {
			return fmt.Errorf("keygen: %s already exists; refusing to overwrite a key", p)
		}
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.WriteFile(privPath, []byte(hex.EncodeToString(priv)+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(pubPath, []byte(hex.EncodeToString(pub)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "private key: %s (mode 0600; never commit it, never put it in CI)\npublic key (hex): %s\n", privPath, hex.EncodeToString(pub))
	return nil
}

func readPrivate(path string) (ed25519.PrivateKey, error) {
	if path == "" {
		return nil, errors.New("-key is required")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return nil, errors.New("private key file is not a 64 byte hex ed25519 key")
	}
	return ed25519.PrivateKey(raw), nil
}

func writeManifest(version, archive, goos, arch, notesFile, outPath string) error {
	if outPath == "" || archive == "" {
		return errors.New("manifest: -archive and -out are required")
	}
	b, err := os.ReadFile(archive)
	if err != nil {
		return err
	}
	notes := "Codepass " + version
	if notesFile != "" {
		nb, err := os.ReadFile(notesFile)
		if err != nil {
			return err
		}
		notes = strings.TrimSpace(string(nb))
	}
	sum := sha256.Sum256(b)
	m := Manifest{Version: version, OS: goos, Arch: arch, File: filepath.Base(archive), Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:]), Notes: notes}
	if err := m.wellFormed(); err != nil {
		return err
	}
	j, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outPath, append(j, '\n'), 0o644)
}

func signBytes(priv ed25519.PrivateKey, manifest []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(sigDomain), manifest...)))
}

func signFile(keyPath, in, outPath string) error {
	priv, err := readPrivate(keyPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	// Never sign something the app would refuse.
	if err := parseManifest(data, new(Manifest)); err != nil {
		return err
	}
	if outPath == "" {
		return errors.New("sign: -out is required")
	}
	return os.WriteFile(outPath, []byte(signBytes(priv, data)+"\n"), 0o644)
}

func parseManifest(raw []byte, m *Manifest) error {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(m); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	return m.wellFormed()
}

func (m *Manifest) wellFormed() error {
	if !semverRe.MatchString(m.Version) {
		return fmt.Errorf("manifest: version %q is not plain semver (use 1.0.0-rc.2, with a dot before the number)", m.Version)
	}
	if m.OS == "" || m.Arch == "" {
		return errors.New("manifest: os/arch missing")
	}
	if m.File == "" || m.File != filepath.Base(m.File) || strings.ContainsAny(m.File, `/\`) || strings.HasPrefix(m.File, ".") {
		return fmt.Errorf("manifest: file %q is not a plain name", m.File)
	}
	if m.Size <= 0 || m.Size > 2<<30 {
		return errors.New("manifest: size out of range")
	}
	if d, err := hex.DecodeString(m.SHA256); err != nil || len(d) != 32 {
		return errors.New("manifest: sha256 must be 64 hex characters")
	}
	// Same rule as the app: bounded, valid UTF-8, no NUL.
	if len(m.Notes) > maxNotesBytes || !utf8.ValidString(m.Notes) || strings.ContainsRune(m.Notes, 0) {
		return errors.New("manifest: notes are too long or not plain text")
	}
	return nil
}

func verifyFile(pubList, in, sigPath, archive string) (*Manifest, error) {
	var keys []ed25519.PublicKey
	for _, h := range strings.Split(pubList, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		k, err := hex.DecodeString(h)
		if err != nil || len(k) != ed25519.PublicKeySize {
			return nil, errors.New("verify: public key is not 32 byte hex")
		}
		keys = append(keys, ed25519.PublicKey(k))
	}
	if len(keys) == 0 {
		return nil, errors.New("verify: -pub is required")
	}
	raw, err := os.ReadFile(in)
	if err != nil {
		return nil, err
	}
	sb, err := os.ReadFile(sigPath)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sb)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, errors.New("verify: malformed signature")
	}
	ok := false
	for _, k := range keys {
		if ed25519.Verify(k, append([]byte(sigDomain), raw...), sig) {
			ok = true
		}
	}
	if !ok {
		return nil, errors.New("verify: signature does not match the public key")
	}
	var m Manifest
	if err := parseManifest(raw, &m); err != nil {
		return nil, err
	}
	if archive != "" {
		ab, err := os.ReadFile(archive)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(ab)
		if int64(len(ab)) != m.Size || hex.EncodeToString(sum[:]) != m.SHA256 || filepath.Base(archive) != m.File {
			return nil, errors.New("verify: archive does not match the manifest (name, size or sha256)")
		}
	}
	return &m, nil
}

// insideGitWorkTree returns the folder holding a .git entry above (or at) dir, or "". dir may not exist yet:
// the nearest existing parent is resolved and walked up.
func insideGitWorkTree(dir string) string {
	p, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
			break
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
	for {
		if _, err := os.Lstat(filepath.Join(p, ".git")); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return ""
		}
		p = parent
	}
}
