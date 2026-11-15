package toolinstall

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/rudian/codepass/internal/api"
)

// HomebrewTeamID is the Apple Team ID that signs Homebrew.pkg. It is pinned instead of a certificate: the
// certificate changed between releases (5.1.13 and 7.0.13) and the Team ID did not. A normal Codepass release
// changes this constant if Homebrew ever moves to another team.
const HomebrewTeamID = "927JGANW46"

// MaxPkgBytes caps every copy of the package (a release is about 154 MB).
const MaxPkgBytes = 400 << 20

// PkgutilPath is the only pkgutil the app runs.
const PkgutilPath = "/usr/sbin/pkgutil"

// Failure is a failure with its cause. It is an error so steps can return it directly.
type Failure struct {
	Cause  api.FailureCause
	Detail string // the tool's own words; shown in the log, never in a sentence of the UI
	Code   int    // the tool's exit code, -1 when none
}

func (f *Failure) Error() string { return string(f.Cause) + ": " + f.Detail }

func fail(cause api.FailureCause, format string, a ...any) *Failure {
	return &Failure{Cause: cause, Detail: fmt.Sprintf(format, a...), Code: -1}
}

var reLeaf = regexp.MustCompile(`^\s+1\. Developer ID Installer: .*\(([A-Z0-9]{10})\)\s*$`)

const (
	sigDistribution = "Status: signed by a developer certificate issued by Apple for distribution"
	sigNotarized    = "Notarization: trusted by the Apple notary service"
)

// CheckSignatureOutput judges the output of `pkgutil --check-signature`. It is the Go twin of the checks in the
// privileged script: both must say the same for the same text (the tests feed both the same recorded outputs).
func CheckSignatureOutput(out, team string) *Failure {
	head := firstLines(out, 3)
	if !strings.Contains(out, sigDistribution) {
		return fail(api.CauseBadSignature, "signature: %s", head)
	}
	leaf := ""
	for _, l := range strings.Split(out, "\n") {
		if m := reLeaf.FindStringSubmatch(l); m != nil {
			leaf = m[1]
			break
		}
	}
	if leaf == "" {
		return fail(api.CauseBadSignature, "signature: no Developer ID Installer certificate")
	}
	if leaf != team {
		return fail(api.CauseUnknownSigner, "Team ID is %s, expected %s", leaf, team)
	}
	if !strings.Contains(out, sigNotarized) {
		return fail(api.CauseNotNotarized, "not notarized")
	}
	return nil
}

func firstLines(s string, n int) string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
			if len(out) == n {
				break
			}
		}
	}
	return strings.Join(out, " ")
}

// PreCheckInput is what the unprivileged check compares the download with.
type PreCheckInput struct {
	Path   string
	Team   string
	Size   int64  // <= 0: unknown (the rate-limited fallback)
	Sha256 string // "": unknown
}

// PreCheck is the visible "verify signature" step. It runs as the user before any password is asked: regular
// file, size, digest, then pkgutil's verdict and the Team ID. A truncated file or a proxy's web page is a
// damaged download, not a wrong signature.
func PreCheck(ctx context.Context, pkgutil string, in PreCheckInput) *Failure {
	if pkgutil == "" {
		pkgutil = PkgutilPath
	}
	st, err := os.Lstat(in.Path)
	if err != nil {
		return fail(api.CauseDamagedDownload, "cannot read the download: %v", err)
	}
	if !st.Mode().IsRegular() {
		return fail(api.CauseDamagedDownload, "the download is not a regular file")
	}
	if st.Size() > MaxPkgBytes {
		return fail(api.CauseDamagedDownload, "the download is larger than %d bytes", MaxPkgBytes)
	}
	if in.Size > 0 && st.Size() != in.Size {
		return fail(api.CauseDamagedDownload, "size %d, expected %d", st.Size(), in.Size)
	}
	f, err := os.Open(in.Path)
	if err != nil {
		return fail(api.CauseDamagedDownload, "cannot read the download: %v", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, MaxPkgBytes+1)); err != nil {
		return fail(api.CauseDamagedDownload, "cannot read the download: %v", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); in.Sha256 != "" && !strings.EqualFold(got, in.Sha256) {
		return fail(api.CauseDamagedDownload, "digest %s, expected %s", got, in.Sha256)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, pkgutil, "--check-signature", in.Path)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	out, _ := cmd.CombinedOutput() // a non-zero exit is judged by the text, like the script does
	if ctx.Err() != nil {
		return fail(api.CauseNetwork, "pkgutil did not answer in time (it asks Apple about the notarization)")
	}
	return CheckSignatureOutput(string(out), in.Team)
}
