// Package update verifies and installs application updates (plan 4.14). A signed manifest per platform decides
// what a release is; the Wails updater is used only to find the release, stream the archive and swap the bundle.
// Every trust decision is made here, on the staged copy, before the swap. Everything fails closed.
package update

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/mod/semver"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// sigDomain is prepended to the manifest bytes before signing so a signature
// made for another purpose with the same key cannot be replayed as a manifest.
const sigDomain = "codepass-update-manifest-v1\n"

const maxManifestBytes = 256 << 10
const maxNotesBytes = 16 << 10

// Manifest is the signed document published next to each platform archive.
type Manifest struct {
	Version string `json:"version"` // "1.0.0-rc.2" (no leading v)
	OS      string `json:"os"`
	Arch    string `json:"arch"`
	File    string `json:"file"`   // archive base name
	Size    int64  `json:"size"`   // archive bytes
	SHA256  string `json:"sha256"` // hex of the archive
	Notes   string `json:"notes"`  // plain text
}

// SignManifest signs the exact bytes of a manifest (release tooling only).
func SignManifest(priv ed25519.PrivateKey, manifest []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, append([]byte(sigDomain), manifest...)))
}

// VerifyManifest checks sigB64 over raw against any pinned key (current and
// next, plan 4.14 item 2) and only then parses raw. Unknown fields are refused.
func VerifyManifest(raw []byte, sigB64 string, keys []ed25519.PublicKey) (*Manifest, error) {
	if len(raw) == 0 || len(raw) > maxManifestBytes {
		return nil, codedf(CodeManifestInvalid, "manifest: bad size")
	}
	if len(keys) == 0 {
		return nil, codedf(CodeNoKey, "manifest: no pinned keys")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, codedf(CodeManifestSignature, "manifest: malformed signature")
	}
	msg := append([]byte(sigDomain), raw...)
	ok := false
	for _, k := range keys {
		if len(k) == ed25519.PublicKeySize && ed25519.Verify(k, msg, sig) {
			ok = true // keep looping: same work whichever key matched
		}
	}
	if !ok {
		return nil, codedf(CodeManifestSignature, "manifest: signature does not match any pinned key")
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, codedf(CodeManifestInvalid, "manifest: %v", err)
	}
	if err := m.wellFormed(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) wellFormed() error {
	if !semver.IsValid("v" + m.Version) {
		return codedf(CodeManifestInvalid, "manifest: version %q is not semver", m.Version)
	}
	if m.OS == "" || m.Arch == "" {
		return codedf(CodeManifestInvalid, "manifest: os/arch missing")
	}
	if m.File == "" || m.File != filepath.Base(m.File) || strings.ContainsAny(m.File, `/\`) || strings.HasPrefix(m.File, ".") {
		return codedf(CodeManifestInvalid, "manifest: file %q is not a plain name", m.File)
	}
	if m.Size <= 0 || m.Size > 2<<30 {
		return codedf(CodeManifestInvalid, "manifest: size out of range")
	}
	// An empty or short digest would make the Wails updater skip its digest
	// comparison (it only compares when len(Digest) > 0), so insist on 32 bytes.
	if d, err := hex.DecodeString(m.SHA256); err != nil || len(d) != 32 {
		return codedf(CodeManifestInvalid, "manifest: sha256 must be 64 hex characters")
	}
	if len(m.Notes) > maxNotesBytes || !utf8.ValidString(m.Notes) || strings.ContainsRune(m.Notes, 0) {
		return codedf(CodeManifestInvalid, "manifest: notes are too long or not plain text")
	}
	return nil
}

// Rules are the version rules of plan 4.14 item 3.
type Rules struct {
	Tag     string // release tag as the host published it ("v1.0.0-rc.2")
	Running string // running app version
	Highest string // highest version ever installed (stored); "" if none
	OS      string
	Arch    string
}

// Check applies "version equals the tag, is newer than the running version, is
// at least the highest installed" plus the platform match.
func (m *Manifest) Check(r Rules) error {
	if m.OS != r.OS || m.Arch != r.Arch {
		return codedf(CodePlatform, "manifest is for %s/%s, not %s/%s", m.OS, m.Arch, r.OS, r.Arch)
	}
	tag := "v" + strings.TrimPrefix(r.Tag, "v")
	if !semver.IsValid(tag) || semver.Build(tag) != "" {
		return codedf(CodeVersionTag, "release tag %q is not plain semver", r.Tag)
	}
	if "v"+m.Version != tag {
		return codedf(CodeVersionTag, "manifest version %s does not equal release tag %s (re-published archive?)", m.Version, r.Tag)
	}
	run := "v" + strings.TrimPrefix(r.Running, "v")
	if !semver.IsValid(run) {
		return codedf(CodeVersionNotNewer, "running version %q is not semver", r.Running)
	}
	if semver.Compare(tag, run) <= 0 {
		return codedf(CodeVersionNotNewer, "manifest version %s is not newer than running %s", m.Version, r.Running)
	}
	if r.Highest != "" {
		hi := "v" + strings.TrimPrefix(r.Highest, "v")
		if !semver.IsValid(hi) {
			return codedf(CodeHighestUnreadable, "stored highest version %q is not semver", r.Highest)
		}
		if semver.Compare(tag, hi) < 0 {
			return codedf(CodeBelowHighest, "manifest version %s is below the highest installed %s", m.Version, r.Highest)
		}
	}
	return nil
}
