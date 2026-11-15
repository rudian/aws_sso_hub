#!/usr/bin/env bash
# Codepass release pipeline (plan 4.14 item 7). Runs on the owner's Mac.
#
#   build/release/release.sh [--repo OWNER/NAME] [--out DIR] VERSION      build, sign, notarize, upload a DRAFT
#   build/release/release.sh --publish [--repo OWNER/NAME] VERSION        publish the draft after the smoke test
#   build/release/release.sh --dry-run [--app PATH] [--out DIR] VERSION   no Apple service, no real key, no upload
#
# VERSION is 1.0.0 or 1.0.0-rc.N (no leading zeros). The build runs in a pristine export of HEAD inside the
# output folder, so the checkout is never modified and the bundle is exactly the committed tree. Steps: checks,
# build, re-sign (executable, then bundle), notarize, staple, archive from the stapled app, list and extract the
# archive and verify, manifest, sign, upload as a draft. Every step either passes or stops the script, and the
# script checks that no step was skipped BEFORE anything is uploaded. Nothing is published by the first form:
# run smoke.sh on the printed app, then run --publish, which re-verifies the draft's assets and publishes it.
#
# --dry-run   the bundle is signed ad hoc, notary and staple are reported as skipped, a throwaway ed25519 key
#             (never CODEPASS_SIGNING_KEY_FILE) signs the manifest, the upload is only printed. It also proves
#             the requirement check is live by asserting that it refuses the ad-hoc bundle.
# --app PATH  (dry run only) use an existing bundle instead of building (no Windows build either, unless
#             --windows-exe PATH names a ready exe to package).
# --out DIR   base folder; a fresh codepass-release.XXXXXX folder is made inside it (default $TMPDIR).
# Settings come from the environment or build/release/local.env; CODEPASS_NO_LOCAL_ENV=1 skips that file and
# CODEPASS_LOCAL_ENV=PATH reads another one.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT="$(cd "$HERE/../.." && pwd -P)"
# shellcheck source=lib.sh
. "$HERE/lib.sh"
if [ "${CODEPASS_NO_LOCAL_ENV:-}" != 1 ]; then
  LOCAL_ENV="${CODEPASS_LOCAL_ENV:-$HERE/local.env}"
  if [ -f "$LOCAL_ENV" ]; then . "$LOCAL_ENV"; fi
fi

DRY=0; PUBLISH=0; APP_IN=""; WIN_EXE_IN=""; REPO="${CODEPASS_RELEASE_REPO:-}"; VERSION=""
OUT_BASE="${CODEPASS_OUT_DIR:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --dry-run) DRY=1 ;;
    --publish) PUBLISH=1 ;;
    --app) APP_IN="${2:?--app needs a path}"; shift ;;
    --windows-exe) WIN_EXE_IN="${2:?--windows-exe needs a path}"; shift ;;
    --repo) REPO="${2:?--repo needs owner/name}"; shift ;;
    --out) OUT_BASE="${2:?--out needs a directory}"; shift ;;
    -h|--help) sed -n '2,25p' "$0"; exit 0 ;;
    -*) die "unknown option $1" ;;
    *) [ -z "$VERSION" ] || die "two versions given"; VERSION="$1" ;;
  esac
  shift
done
[ -n "$VERSION" ] || die "usage: release.sh [--dry-run] [--app PATH] [--repo OWNER/NAME] [--out DIR] VERSION | --publish VERSION"
[ "$DRY" = 0 ] || [ "$PUBLISH" = 0 ] || die "--publish is a real step; it cannot be combined with --dry-run"
[ "$DRY" = 1 ] || [ -z "$APP_IN" ] || die "--app is only for --dry-run: a real release builds from the committed tree"
[ -z "$WIN_EXE_IN" ] || [ -n "$APP_IN" ] || die "--windows-exe goes with --app (dry run only)"
valid_version "$VERSION" || die "version '$VERSION' must look like 1.0.0 or 1.0.0-rc.1 (a dot before the number)"
TAG="v$VERSION"
case "$VERSION" in *-rc.*) PRERELEASE=1 ;; *) PRERELEASE=0 ;; esac
[ "$(uname -s)" = Darwin ] || die "the release runs on macOS"
# Before anything that runs git or go (go build execs git for VCS stamping): the Command Line Tools must exist.
require_dev_tools

NOTARY_TIMEOUT="${NOTARY_TIMEOUT:-30m}"
GH="${GH:-gh}"
WAILS3="${WAILS3:-$ROOT/.tools/bin/wails3}"
[ -x "$WAILS3" ] || WAILS3="$(command -v wails3 || true)"
ARCH=arm64; OS=darwin
APP_NAME="Codepass"                       # bundle folder name; the executable inside is "codepass"
BUNDLE_ID="$(sed -n 's/^const BundleID = "\(.*\)"$/\1/p' "$ROOT/internal/version/version.go")"
[ -n "$BUNDLE_ID" ] || die "cannot read BundleID from internal/version/version.go"

# build_relkit DIR -> DIR/relkit, built from the checkout's own code.
build_relkit() { ( cd "$ROOT" && go build -o "$1/relkit" ./build/release/relkit ) || die "cannot build relkit"; }

# ---------------------------------------------------------------- publish: the second step of a release
if [ "$PUBLISH" = 1 ]; then
  [ -n "$REPO" ] || die "no target repository: pass --repo OWNER/NAME or set CODEPASS_RELEASE_REPO"
  need_var CODEPASS_EXPECT_PUBKEY
  PT="$(mktemp -d "${TMPDIR:-/tmp}/codepass-publish.XXXXXX")"; trap 'rm -rf "$PT"' EXIT
  gh_signed_in "$GH" || die "gh is not signed in (gh auth status --active --hostname github.com)"
  info="$("$GH" release view "$TAG" --repo "$REPO" --json isDraft,assets \
    --jq '(if .isDraft then "draft" else "published" end), (.assets[].name)')" || die "cannot read release $TAG in $REPO"
  [ "$(printf '%s\n' "$info" | head -1)" = draft ] || die "release $TAG is not a draft (already published?)"
  for want in "Codepass-$VERSION-$OS-$ARCH.zip" "manifest-$OS-$ARCH.json" "manifest-$OS-$ARCH.json.sig" \
              "Codepass-$VERSION-windows-amd64.zip" "manifest-windows-amd64.json" "manifest-windows-amd64.json.sig"; do
    printf '%s\n' "$info" | grep -qx "$want" || die "the draft has no asset $want"
  done
  "$GH" release download "$TAG" --repo "$REPO" --dir "$PT" || die "cannot download the draft's assets"
  build_relkit "$PT"
  m="" ; wm=""
  for t in "$OS-$ARCH" windows-amd64; do
    r="$("$PT/relkit" verify -pub "$CODEPASS_EXPECT_PUBKEY" -in "$PT/manifest-$t.json" -sig "$PT/manifest-$t.json.sig" \
      -archive "$PT/Codepass-$VERSION-$t.zip")" || die "the draft's $t manifest does not verify against CODEPASS_EXPECT_PUBKEY"
    case "$r" in "ok version=$VERSION "*) ;; *) die "the draft's $t manifest says: $r" ;; esac
    m="$m$r; "
  done
  log "draft verified: $m"
  "$GH" release edit "$TAG" --repo "$REPO" --draft=false
  log "PUBLISHED $TAG"
  exit 0
fi

# Output goes into a fresh folder outside the repository; the check runs before anything is created.
[ -n "$OUT_BASE" ] || OUT_BASE="${TMPDIR:-/tmp}"
case "/$OUT_BASE/" in */../*) die "--out must not contain a '..' component ($OUT_BASE)" ;; esac
case "$(phys_path "$OUT_BASE")/" in "$ROOT"/*) die "the output folder must be outside the repository ($OUT_BASE)" ;; esac
mkdir -p "$OUT_BASE"
OUT_DIR="$(mktemp -d "$(cd "$OUT_BASE" && pwd -P)/codepass-release.XXXXXX")"
# Recheck the physical folder: a ".." after a missing component is not resolved by phys_path.
case "$(cd "$OUT_DIR" && pwd -P)/" in "$ROOT"/*) rm -rf "$OUT_DIR"; die "the output folder must be outside the repository ($OUT_BASE)" ;; esac
STEPS_FILE="$OUT_DIR/steps.log"; : >"$STEPS_FILE"
log "output folder: $OUT_DIR"

# ---------------------------------------------------------------- 1. checks
HEAD0="$(git -C "$ROOT" rev-parse HEAD)" || die "cannot read HEAD"
if [ "$DRY" = 1 ]; then
  tree_is_clean "$ROOT" || warn "the working tree is not clean (allowed only in a dry run; a real release refuses, and builds only what is committed)"
else
  need_var CODEPASS_SIGN_IDENTITY; need_var CODEPASS_TEAM_ID; need_var CODEPASS_NOTARY_PROFILE
  need_var CODEPASS_SIGNING_KEY_FILE; need_var CODEPASS_EXPECT_PUBKEY
  [ -z "${CODEPASS_ENTITLEMENTS:-}" ] || die "CODEPASS_ENTITLEMENTS is only for --dry-run: a real release signs only build/darwin/entitlements.plist from the committed tree"
  tree_is_clean "$ROOT" || die "the working tree is not clean; commit or stash first (nothing is released from a dirty tree)"
  [ -n "$REPO" ] || die "no target repository: pass --repo OWNER/NAME or set CODEPASS_RELEASE_REPO"
  case "$CODEPASS_SIGN_IDENTITY" in "Developer ID Application:"*) ;; *) die "the signing identity must be a Developer ID Application identity" ;; esac
  /usr/bin/security find-identity -v -p codesigning | grep -qF "\"$CODEPASS_SIGN_IDENTITY\"" \
    || die "the signing identity is not in the keychain (security find-identity -v -p codesigning)"
  [ -f "$CODEPASS_SIGNING_KEY_FILE" ] || die "signing key file not found"
  KEY_REAL="$(cd "$(dirname "$CODEPASS_SIGNING_KEY_FILE")" && pwd -P)/$(basename "$CODEPASS_SIGNING_KEY_FILE")"
  case "$KEY_REAL" in "$ROOT"/*) die "the signing key must live outside the repository" ;; esac
  [ "$(/usr/bin/stat -f %Lp "$KEY_REAL")" = 600 ] || die "the signing key file must have mode 600"
  gh_signed_in "$GH" || die "gh is not signed in (gh auth status --active --hostname github.com); checked now so that it fails before the build"
fi
# A placeholder that is not a Team ID: in a dry run the requirement only has to be refused.
TEAM_ID="${CODEPASS_TEAM_ID:-TEST-TEAM}"
[ "$DRY" = 1 ] && TEAM_ID="TEST-TEAM"
step_done checks

# ---------------------------------------------------------------- 2. build, in a pristine export of HEAD
SRCDIR="$ROOT"
if [ -n "$APP_IN" ]; then
  [ -d "$APP_IN" ] || die "--app: $APP_IN is not a bundle"
  APP_SRC="$APP_IN"; log "using the given bundle (no build, no source version check)"
else
  SRCDIR="$OUT_DIR/src"; mkdir -p "$SRCDIR"
  git -C "$ROOT" archive HEAD | tar -x -C "$SRCDIR" || die "cannot export HEAD"
  # The source must say the version being released: the running app reads internal/version, the updater
  # compares the bundle's version with the manifest's.
  src_ver="$(sed -n 's/^const Version = "\(.*\)"$/\1/p' "$SRCDIR/internal/version/version.go")"
  [ "$src_ver" = "$VERSION" ] || die "internal/version Version is '$src_ver', not '$VERSION'. Bump it (and build/darwin/Info.plist, build/config.yml, build/windows/info.json), commit, and run again."
  plist_ver="$(plist_get "$SRCDIR/build/darwin/Info.plist" CFBundleVersion)"
  plist_short="$(plist_get "$SRCDIR/build/darwin/Info.plist" CFBundleShortVersionString)"
  [ "$plist_ver" = "$VERSION" ] && [ "$plist_short" = "$VERSION" ] || die "build/darwin/Info.plist says '$plist_ver' / '$plist_short', not '$VERSION'"
  cfg_ver="$(sed -n '/^info:/,/^[A-Za-z]/s/^  version: *"\{0,1\}\([^" #]*\)"\{0,1\}.*$/\1/p' "$SRCDIR/build/config.yml" | head -1)"
  [ "$cfg_ver" = "$VERSION" ] || die "build/config.yml info.version is '$cfg_ver', not '$VERSION'"
  # build/windows/info.json: checked before the build, like the other version files
  want_fv="$(win_file_version "$VERSION")"
  for k in file_version product_version; do
    got="$(sed -n "s/^[[:space:]]*\"$k\": *\"\([^\"]*\)\".*/\1/p" "$SRCDIR/build/windows/info.json" | head -1)"
    [ "$got" = "$want_fv" ] || die "build/windows/info.json $k is '$got', not '$want_fv'. Bump it, commit, and run again."
  done
  for k in FileVersion ProductVersion; do
    got="$(sed -n "s/^[[:space:]]*\"$k\": *\"\([^\"]*\)\".*/\1/p" "$SRCDIR/build/windows/info.json" | head -1)"
    [ "$got" = "$want_fv" ] || die "build/windows/info.json $k is '$got', not '$want_fv'"
  done
  [ -n "$WAILS3" ] && [ -x "$WAILS3" ] || die "wails3 not found (set WAILS3)"
  log "build: wails3 task package (in $SRCDIR)"
  ( cd "$SRCDIR" && PATH="$(dirname "$WAILS3"):$PATH" "$WAILS3" task package ) >"$OUT_DIR/build.log" 2>&1 \
    || { tail -30 "$OUT_DIR/build.log" >&2; die "build failed (full log: $OUT_DIR/build.log)"; }
  APP_SRC="$SRCDIR/bin/$APP_NAME.app"
  # Windows exe, cross-built in the same export with the production flags (build/windows/Taskfile.yml).
  log "build: wails3 task windows:build ARCH=amd64 (in $SRCDIR)"
  ( cd "$SRCDIR" && PATH="$(dirname "$WAILS3"):$PATH" "$WAILS3" task windows:build ARCH=amd64 ) >"$OUT_DIR/build-windows.log" 2>&1 \
    || { tail -30 "$OUT_DIR/build-windows.log" >&2; die "windows build failed (full log: $OUT_DIR/build-windows.log)"; }
  WIN_EXE="$SRCDIR/bin/codepass.exe"
  [ -s "$WIN_EXE" ] || die "no windows executable at $WIN_EXE"
fi
if [ -n "$APP_IN" ]; then WIN_EXE="$WIN_EXE_IN"; fi
WIN=0; [ -z "${WIN_EXE:-}" ] || WIN=1
if [ "$WIN" = 1 ]; then
  [ -f "$WIN_EXE" ] || die "windows executable not found: $WIN_EXE"
  [ "$(head -c 2 "$WIN_EXE")" = MZ ] || die "$WIN_EXE is not a Windows executable"
else
  log "windows: SKIPPED (--app without --windows-exe; test only)"
fi
[ -d "$APP_SRC/Contents/MacOS" ] || die "no bundle at $APP_SRC"
step_done build

# ---------------------------------------------------------------- 3. stage a clean copy and check identity
APP="$OUT_DIR/stage/$APP_NAME.app"
mkdir -p "$OUT_DIR/stage"
/usr/bin/ditto --noextattr --norsrc "$APP_SRC" "$APP"
/usr/bin/xattr -cr "$APP"
EXE_NAME="$(plist_get "$APP/Contents/Info.plist" CFBundleExecutable)"
[ -n "$EXE_NAME" ] && [ -f "$APP/Contents/MacOS/$EXE_NAME" ] || die "bundle has no executable named by CFBundleExecutable"
junk="$(find "$APP" \( -name '._*' -o -name '.DS_Store' \) -print | head -3)"
[ -z "$junk" ] || die "the bundle holds metadata files that would break the signature once archived: $junk"
[ "$(plist_get "$APP/Contents/Info.plist" CFBundleIdentifier)" = "$BUNDLE_ID" ] || die "bundle identifier is not $BUNDLE_ID"
[ "$(plist_get "$APP/Contents/Info.plist" CFBundleVersion)" = "$VERSION" ] || die "bundle version is not $VERSION"
[ "$(plist_get "$APP/Contents/Info.plist" CFBundleShortVersionString)" = "$VERSION" ] || die "bundle short version is not $VERSION"
[ "$(/usr/bin/lipo -archs "$APP/Contents/MacOS/$EXE_NAME")" = "$ARCH" ] || die "executable is not $ARCH only"
step_done stage

# ---------------------------------------------------------------- 4. sign: executable first, then the bundle
if [ "$DRY" = 1 ]; then
  SIGN=(--force --sign -)
  log "sign: AD HOC (dry run)"
else
  SIGN=(--force --options runtime --timestamp --sign "$CODEPASS_SIGN_IDENTITY")
  log "sign: hardened runtime, secure timestamp"
fi
# Optional entitlements: inert unless build/darwin/entitlements.plist is committed (needed only if the
# smoke test shows 'Open in terminal' requires com.apple.security.automation.apple-events).
# CODEPASS_ENTITLEMENTS is a test hook for --dry-run only: a real release signs exactly the committed file.
ENT="$SRCDIR/build/darwin/entitlements.plist"
[ "$DRY" = 0 ] || ENT="${CODEPASS_ENTITLEMENTS:-$ENT}"
if [ -f "$ENT" ]; then SIGN+=(--entitlements "$ENT"); log "sign: with entitlements $ENT"; fi
/usr/bin/codesign "${SIGN[@]}" "$APP/Contents/MacOS/$EXE_NAME"
/usr/bin/codesign "${SIGN[@]}" "$APP"
/usr/bin/codesign --verify --deep --strict "$APP"        # --deep only to verify
/usr/bin/codesign -d --entitlements - --xml "$APP" >"$OUT_DIR/entitlements.txt" 2>&1 || true
if [ "$DRY" = 1 ]; then
  # Proof that the gate below is live: the requirement must refuse an ad-hoc bundle.
  rc=0; check_requirement "$APP" "$BUNDLE_ID" "$TEAM_ID" || rc=$?
  [ "$rc" = 3 ] || die "the requirement check must refuse the ad-hoc bundle with codesign status 3, got $rc"
  log "requirement check correctly refuses the ad-hoc bundle (status 3)"
else
  check_requirement "$APP" "$BUNDLE_ID" "$TEAM_ID" || die "the signed bundle does not satisfy the updater's requirement"
fi
step_done sign

# ---------------------------------------------------------------- 5. notarize and staple
if [ "$DRY" = 1 ]; then
  log "notarize: SKIPPED (dry run)"; log "staple: SKIPPED (dry run)"
else
  NZIP="$OUT_DIR/notarize.zip"
  /usr/bin/ditto -c -k --keepParent "$APP" "$NZIP"
  log "notarize: submitting (timeout $NOTARY_TIMEOUT)"
  /usr/bin/xcrun notarytool submit "$NZIP" --keychain-profile "$CODEPASS_NOTARY_PROFILE" \
    --wait --timeout "$NOTARY_TIMEOUT" --output-format json >"$OUT_DIR/notary.json" 2>"$OUT_DIR/notary.err" \
    || {
      cat "$OUT_DIR/notary.err" >&2; [ -s "$OUT_DIR/notary.json" ] && cat "$OUT_DIR/notary.json" >&2
      # Apple does not document the exit status of a finished Invalid/Rejected submission: keep the log whenever an id exists.
      fid="$(/usr/bin/plutil -extract id raw -o - "$OUT_DIR/notary.json" 2>/dev/null || true)"
      if [ -n "$fid" ]; then
        /usr/bin/xcrun notarytool log "$fid" --keychain-profile "$CODEPASS_NOTARY_PROFILE" "$OUT_DIR/notary-log.json" \
          && log "notary log kept: $OUT_DIR/notary-log.json" || warn "could not fetch the notary log"
      fi
      die "notarytool submit failed, was rejected or timed out"
    }
  SUB_ID="$(/usr/bin/plutil -extract id raw -o - "$OUT_DIR/notary.json")" || die "no submission id in the notary output"
  STATUS="$(/usr/bin/plutil -extract status raw -o - "$OUT_DIR/notary.json")" || die "no status in the notary output"
  # The log is kept whatever the status.
  /usr/bin/xcrun notarytool log "$SUB_ID" --keychain-profile "$CODEPASS_NOTARY_PROFILE" "$OUT_DIR/notary-log.json" \
    || warn "could not fetch the notary log"
  [ "$STATUS" = "Accepted" ] || die "notarization status is '$STATUS', not Accepted (log: $OUT_DIR/notary-log.json)"
  rm -f "$NZIP"
  step_done notarize
  /usr/bin/xcrun stapler staple "$APP"
  /usr/bin/xcrun stapler validate "$APP"
  /usr/sbin/spctl -a -vvv -t exec "$APP" 2>&1 | tee "$OUT_DIR/spctl-stapled.txt" | grep -q "source=Notarized Developer ID" \
    || die "Gatekeeper did not report 'Notarized Developer ID' for the stapled app"
  step_done staple
fi

# ---------------------------------------------------------------- 6. archive from the stapled app
ARCHIVE="$OUT_DIR/Codepass-$VERSION-$OS-$ARCH.zip"
/usr/bin/ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl "$APP" "$ARCHIVE"
step_done archive

# ---------------------------------------------------------------- 7. list, extract and verify what will be uploaded
# The listing comes first: it shows the archive as the app's own extractor sees it, entry by entry.
# ditto would merge ._* entries back into extended attributes and hide them; unzip writes them as files,
# which is what the app's extractor does.
LIST="$(/usr/bin/zipinfo -1 "$ARCHIVE")" || die "cannot list the archive"
printf '%s\n' "$LIST" | grep -Eq '(^|/)\._' && die "the archive holds AppleDouble (._*) entries"
printf '%s\n' "$LIST" | grep -Eq '(^|/)__MACOSX(/|$)' && die "the archive holds a __MACOSX entry"
printf '%s\n' "$LIST" | grep -Eq '(^|/)\.DS_Store$' && die "the archive holds a .DS_Store entry"
tops="$(printf '%s\n' "$LIST" | awk -F/ 'NF && $1 != "" {print $1}' | sort -u)"
[ "$tops" = "$APP_NAME.app" ] || die "the archive must hold exactly one top-level entry, $APP_NAME.app; it holds: $(printf '%s' "$tops" | tr '\n' ' ')"
VER_DIR="$OUT_DIR/verify"; mkdir -p "$VER_DIR"
/usr/bin/unzip -q "$ARCHIVE" -d "$VER_DIR" || die "cannot extract the archive"
X="$VER_DIR/$APP_NAME.app"
[ -d "$X" ] || die "the archive does not unpack to $APP_NAME.app"
junk="$(find "$VER_DIR" \( -name '._*' -o -name '__MACOSX' -o -name '.DS_Store' \) -print | head -3)"
[ -z "$junk" ] || die "the extracted archive holds metadata entries: $junk"
/usr/bin/codesign --verify --deep --strict "$X" || die "the extracted app fails codesign --verify"
[ "$(plist_get "$X/Contents/Info.plist" CFBundleVersion)" = "$VERSION" ] || die "extracted bundle version differs from $VERSION"
if [ "$DRY" = 1 ]; then
  rc=0; check_requirement "$X" "$BUNDLE_ID" "$TEAM_ID" || rc=$?
  [ "$rc" = 3 ] || die "the requirement check must refuse the ad-hoc archive with codesign status 3, got $rc"
else
  check_requirement "$X" "$BUNDLE_ID" "$TEAM_ID" || die "the extracted app does not satisfy the updater's requirement"
  /usr/bin/xcrun stapler validate "$X" || die "the extracted app has no valid ticket"
  /usr/sbin/spctl -a -t exec "$X" || die "Gatekeeper refuses the extracted app"
fi
step_done verify-archive

# ---------------------------------------------------------------- 8. manifest and signature
build_relkit "$OUT_DIR"; RELKIT="$OUT_DIR/relkit"
if [ "$DRY" = 1 ]; then
  # Always a throwaway key: a dry run never signs with the real one, whatever the environment says.
  "$RELKIT" keygen -dir "$OUT_DIR/test-key" >/dev/null
  KEY="$OUT_DIR/test-key/current.priv"; log "manifest: signed with a throwaway test key"
  # Optional cross-check of the real key's PUBLIC half only; nothing is signed with it.
  if [ -n "${CODEPASS_EXPECT_PUBKEY:-}" ] && [ -n "${CODEPASS_SIGNING_KEY_FILE:-}" ] && [ -f "$CODEPASS_SIGNING_KEY_FILE" ]; then
    [ "$("$RELKIT" pub -key "$CODEPASS_SIGNING_KEY_FILE")" = "$CODEPASS_EXPECT_PUBKEY" ] \
      || die "the signing key file's public half is not CODEPASS_EXPECT_PUBKEY"
    log "the real signing key matches CODEPASS_EXPECT_PUBKEY (not used in a dry run)"
  fi
else
  KEY="$KEY_REAL"
fi
PUB="$("$RELKIT" pub -key "$KEY")"
if [ "$DRY" = 0 ]; then
  [ "$PUB" = "$CODEPASS_EXPECT_PUBKEY" ] || die "the signing key's public half is not CODEPASS_EXPECT_PUBKEY"
fi
log "signing key fingerprint (public, hex): $PUB"
NOTES="$OUT_DIR/notes.txt"
if [ -f "$SRCDIR/docs/release-notes/$VERSION.txt" ]; then cp "$SRCDIR/docs/release-notes/$VERSION.txt" "$NOTES"; else printf 'Codepass %s\n' "$VERSION" >"$NOTES"; fi
MANIFEST="$OUT_DIR/manifest-$OS-$ARCH.json"; SIG="$MANIFEST.sig"
"$RELKIT" manifest -version "$VERSION" -archive "$ARCHIVE" -os "$OS" -arch "$ARCH" -notes-file "$NOTES" -out "$MANIFEST"
"$RELKIT" sign -key "$KEY" -in "$MANIFEST" -out "$SIG"
"$RELKIT" verify -pub "$PUB" -in "$MANIFEST" -sig "$SIG" -archive "$ARCHIVE" >/dev/null || die "the manifest does not verify"
step_done manifest

# ---------------------------------------------------------------- 8b. windows: zip, manifest, signature (unsigned exe, owner decision 3)
ASSETS=("$ARCHIVE" "$MANIFEST" "$SIG")
if [ "$WIN" = 1 ]; then
  WARCHIVE="$OUT_DIR/Codepass-$VERSION-windows-amd64.zip"
  WSTAGE="$OUT_DIR/stage-windows"; mkdir -p "$WSTAGE"; cp "$WIN_EXE" "$WSTAGE/codepass.exe"
  ( cd "$WSTAGE" && /usr/bin/zip -X -q "$WARCHIVE" codepass.exe ) || die "cannot zip the windows exe"
  [ "$(/usr/bin/zipinfo -1 "$WARCHIVE")" = codepass.exe ] || die "the windows archive must hold exactly codepass.exe"
  WMANIFEST="$OUT_DIR/manifest-windows-amd64.json"; WSIG="$WMANIFEST.sig"
  "$RELKIT" manifest -version "$VERSION" -archive "$WARCHIVE" -os windows -arch amd64 -notes-file "$NOTES" -out "$WMANIFEST"
  "$RELKIT" sign -key "$KEY" -in "$WMANIFEST" -out "$WSIG"
  "$RELKIT" verify -pub "$PUB" -in "$WMANIFEST" -sig "$WSIG" -archive "$WARCHIVE" >/dev/null || die "the windows manifest does not verify"
  ASSETS+=("$WARCHIVE" "$WMANIFEST" "$WSIG")
  step_done windows
fi

# ---------------------------------------------------------------- 9. nothing skipped, then upload a DRAFT
# The guard runs before the upload so that it can stop the release it exists to stop.
if [ "$DRY" = 1 ]; then
  require_steps checks build stage sign archive verify-archive manifest
  [ "$WIN" = 0 ] || require_steps windows
else
  require_steps checks build stage sign notarize staple archive verify-archive manifest windows
fi
BODY="$OUT_DIR/release-body.txt"
cat >"$BODY" <<BODYEOF
Codepass $VERSION. Release notes are shown by the app from the signed manifest.
Built from commit $HEAD0

If an update was interrupted and Codepass.app is missing (the old copy is Codepass.app.bak, usually in /Applications):
  cd /Applications
  ls -d Codepass.app Codepass.app.bak
  mv Codepass.app.bak Codepass.app        # only if Codepass.app is missing
If Codepass.app exists but does not start: quit the app, then
  mv Codepass.app Codepass.app.broken && mv Codepass.app.bak Codepass.app
Then download this release again.
BODYEOF
CREATE=("$GH" release create "$TAG" --repo "${REPO:-OWNER/NAME}" --draft --title "$TAG" --notes-file "$BODY")
# The tag must point at the built commit. A local tag of that name has to equal HEAD0. When the target is this
# source repository (origin), --target pins the new tag to HEAD0 instead of the default branch's tip.
if tg="$(git -C "$ROOT" rev-parse -q --verify "refs/tags/$TAG^{commit}" 2>/dev/null)"; then
  [ "$tg" = "$HEAD0" ] || die "the local tag $TAG points at $tg, not at the built commit $HEAD0"
fi
origin_url="$(git -C "$ROOT" remote get-url origin 2>/dev/null || true)"
case "${origin_url%.git}" in *"/${REPO:-OWNER/NAME}"|*":${REPO:-OWNER/NAME}") CREATE+=(--target "$HEAD0") ;; esac
[ "$PRERELEASE" = 1 ] && CREATE+=(--prerelease)
CREATE+=("${ASSETS[@]}")
if [ "$DRY" = 1 ]; then
  printf 'WOULD RUN:'; printf ' %q' "${CREATE[@]}"; printf '\nWOULD RUN (after smoke.sh passes):'
  printf ' %q' "$0" --publish --repo "${REPO:-OWNER/NAME}" "$VERSION"; printf '\n'
  log "upload: SKIPPED (dry run)"
else
  [ "$(git -C "$ROOT" rev-parse HEAD)" = "$HEAD0" ] || die "HEAD moved during the release; nothing is uploaded"
  tree_is_clean "$ROOT" || die "the working tree became dirty during the release; nothing is uploaded"
  gh_signed_in "$GH" || die "gh is not signed in (gh auth status --active --hostname github.com)"
  "${CREATE[@]}"
  # Check what GitHub holds. The release stays a draft: publishing is the separate --publish step.
  names="$("$GH" release view "$TAG" --repo "$REPO" --json assets --jq '.assets[].name')" || die "cannot read the draft back"
  for a in "${ASSETS[@]}"; do printf '%s\n' "$names" | grep -qx "$(basename "$a")" || die "asset $(basename "$a") is missing from the draft"; done
  step_done upload
  require_steps upload
fi

log "DONE $TAG ($([ "$DRY" = 1 ] && echo dry run || echo draft uploaded, not published))"
log "archive:  $ARCHIVE"
log "manifest: $MANIFEST"
log "next: quit every installed Codepass / AWS SSO Hub, then run build/release/smoke.sh \"$X\""
[ "$DRY" = 1 ] || log "after the smoke test passes: build/release/release.sh --publish --repo $REPO $VERSION"
[ "$DRY" = 1 ] || log "keep $OUT_DIR/notary-log.json with the release record"
