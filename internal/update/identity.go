package update

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
)

// What the app is, for the bundle gate and the release channel (plan 4.14). The bundle ID is the app's original
// one on purpose (plan 4.2) and is part of the code requirement, so it must not change without a hand-over release.
const (
	BundleID             = "com.rudian.awsssohub"
	TeamID               = "XC34P8TNZ6"
	ProductionRepository = "rudian/codepass" // asset URLs are accepted only under this name
)

// DesignatedRequirement is the full designated requirement of plan 4.14 item 4. The two certificate OIDs make
// the Developer ID chain mandatory: the shorter form (anchor, identifier, Team ID) is also met by the team's Apple
// Development and Apple Distribution certificates.
func DesignatedRequirement(bundleID, teamID string) string {
	return fmt.Sprintf(`anchor apple generic and identifier %q and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = %q`, bundleID, teamID)
}

// ProductionGate is the gate every shipped build uses.
func ProductionGate() Gate {
	return Gate{BundleID: BundleID, Requirement: DesignatedRequirement(BundleID, TeamID), RequireGatekeeper: true}
}

// pinnedKeyHex holds the production public keys, hex encoded: the current key and the next one, so the key can be
// rotated without a hand-over release. They are filled in by the key ceremony of docs/release.md (F1-10). While
// they are empty the production channel refuses to build a provider: there is no safe default.
var pinnedKeyHex = [2]string{"", ""}

// ProductionKeys returns the pinned production keys, or an error while none are pinned.
func ProductionKeys() ([]ed25519.PublicKey, error) { return parseKeys(pinnedKeyHex[:]) }

func parseKeys(hexKeys []string) ([]ed25519.PublicKey, error) {
	var keys []ed25519.PublicKey
	for _, h := range hexKeys {
		if h == "" {
			continue
		}
		raw, err := hex.DecodeString(h)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, errors.New("update: a pinned public key is malformed")
		}
		keys = append(keys, ed25519.PublicKey(raw))
	}
	if len(keys) == 0 {
		return nil, errors.New("update: no public key is pinned in this build")
	}
	return keys, nil
}

// testKeyHex holds the test channel's public keys, pinned in the binary like the production ones (filled in by the
// key ceremony of F1-10). A key from settings or an imported file would be a trust anchor a download or an import
// could replace.
var testKeyHex = [2]string{"", ""}

// TestChannel is the private test repository with the pinned test keys. Settings choose only the repository; the
// token comes from the TokenStore. It refuses without a token (the repository is private) and while no test key is pinned.
func TestChannel(repo, token string) (Channel, error) {
	if !reRepo.MatchString(repo) {
		return Channel{}, errors.New("update: the test repository must be written owner/name")
	}
	if token == "" {
		return Channel{}, errors.New("update: enter the test channel's read-only token first")
	}
	keys, err := parseKeys(testKeyHex[:])
	if err != nil {
		return Channel{}, err
	}
	return Channel{Name: "test", Repository: repo, Prerelease: true, Keys: keys, Token: token}, nil
}

var reRepo = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// ParseKeys decodes hex ed25519 public keys. It is for tests and the integration harness only: the app must never
// feed it keys from settings or an imported file; use TestChannel and ProductionChannel.
func ParseKeys(hexKeys []string) ([]ed25519.PublicKey, error) { return parseKeys(hexKeys) }

// Channel says where releases come from.
type Channel struct {
	Name       string // "production" or "test"
	Repository string // "owner/repo"
	Prerelease bool   // walk the release list (rc builds) instead of /releases/latest
	Keys       []ed25519.PublicKey
	Token      string // read-only token for a private test repository; "" for the public production repository
	BaseURL    string // API base; "" means https://api.github.com (tests point it at a fake server)
}

// ProductionChannel is the public repository with the pinned keys and no token.
func ProductionChannel() (Channel, error) {
	keys, err := ProductionKeys()
	if err != nil {
		return Channel{}, err
	}
	return Channel{Name: "production", Repository: ProductionRepository, Keys: keys}, nil
}
