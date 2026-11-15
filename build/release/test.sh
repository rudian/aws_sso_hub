#!/usr/bin/env bash
# CI test for the release pipeline, no Apple account needed: builds a tiny fake bundle and runs
# release.sh --dry-run on it (ad-hoc signature, throwaway ed25519 key, upload only printed), then checks the
# output and the refusals. release.sh runs isolated from the machine: no local.env, and every CODEPASS_*
# variable, GH, WAILS3 and the output variables unset, so it behaves the same on the release Mac. macOS only (codesign, ditto); elsewhere it reports a skip. The manifest and
# signature logic is covered by the Go tests in build/release/relkit.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT="$(cd "$HERE/../.." && pwd -P)"
[ "$(uname -s)" = Darwin ] || { echo "SKIP: macOS only"; exit 0; }
. "$HERE/lib.sh"

T="$(mktemp -d "${TMPDIR:-/tmp}/codepass-release-test.XXXXXX")"
trap '[ -n "${KEEP:-}" ] || rm -rf "$T"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
BID="$(sed -n 's/^const BundleID = "\(.*\)"$/\1/p' "$ROOT/internal/version/version.go")"

mkapp() { # mkapp DIR VERSION [extra]
  local app="$1/Codepass.app"
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  ( cd "$T" && cat >main.go <<'GO'
package main

func main() {}
GO
    go build -o "$app/Contents/MacOS/codepass" main.go )
  cat >"$app/Contents/Info.plist" <<PL
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>codepass</string>
<key>CFBundleIdentifier</key><string>$BID</string>
<key>CFBundleVersion</key><string>$2</string>
<key>CFBundleShortVersionString</key><string>$2</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>
PL
  [ -z "${3:-}" ] || "$3" "$app"
}

# the fake executable must be arm64 only
export GOOS=darwin GOARCH=arm64 CGO_ENABLED=0
mkapp "$T/a" 9.9.9-rc.1 2>/dev/null || { mkdir -p "$T/a"; mkapp "$T/a" 9.9.9-rc.1; }

# release.sh in isolation (see the header). Extra environment goes first: rel VAR=value ... -- args
rel() {
  local uargs=() v extra=()
  while [ $# -gt 0 ] && [ "$1" != -- ]; do extra+=("$1"); shift; done
  [ $# -eq 0 ] || shift
  for v in $(env | sed -n 's/^\(CODEPASS_[A-Za-z0-9_]*\)=.*/\1/p'); do uargs+=(-u "$v"); done
  env ${uargs[@]+"${uargs[@]}"} -u GH -u WAILS3 -u OUT_DIR -u NOTARY_TIMEOUT CODEPASS_NO_LOCAL_ENV=1 ${extra[@]+"${extra[@]}"} "$HERE/release.sh" "$@"
}
outdir() { sed -n 's/.*output folder: //p' <<<"$1" | head -1; }

# a throwaway "real" key that a dry run must never sign with
FIX="$T/fixture-key"; ( cd "$ROOT" && go run ./build/release/relkit keygen -dir "$FIX" ) >/dev/null
FIXPUB="$(cat "$FIX/current.pub")"

mkdir -p "$T/w"; ( cd "$T" && GOOS=windows GOARCH=amd64 go build -o "$T/w/codepass.exe" main.go )
export WINEXE="$T/w/codepass.exe"
log=$(rel CODEPASS_SIGNING_KEY_FILE="$FIX/current.priv" CODEPASS_EXPECT_PUBKEY="$FIXPUB" GH=gh -- \
  --dry-run --app "$T/a/Codepass.app" --windows-exe "$WINEXE" --out "$T/out" --repo acme/codepass-test 9.9.9-rc.1 2>&1) || { echo "$log"; fail "dry run failed"; }
OUT="$(outdir "$log")"; [ -d "$OUT" ] || fail "no output folder reported"
case "$OUT" in "$ROOT"/*) fail "output inside the repository" ;; esac
echo "$log" | grep -q 'DONE v9.9.9-rc.1' || fail "no DONE line"
echo "$log" | grep -q 'notarize: SKIPPED' || fail "dry run must say notarize was skipped"
echo "$log" | grep -q 'WOULD RUN: gh release create v9.9.9-rc.1 --repo acme/codepass-test --draft' || fail "upload command not printed"
echo "$log" | grep -q -- '--prerelease' || fail "rc must be a prerelease"
echo "$log" | grep -q -- '--publish' || fail "the publish step must be printed as a separate command"
for f in Codepass-9.9.9-rc.1-darwin-arm64.zip manifest-darwin-arm64.json manifest-darwin-arm64.json.sig release-body.txt Codepass-9.9.9-rc.1-windows-amd64.zip manifest-windows-amd64.json manifest-windows-amd64.json.sig; do
  [ -s "$OUT/$f" ] || fail "missing $f"
done
grep -q 'mv Codepass.app.bak Codepass.app' "$OUT/release-body.txt" || fail "release body lacks the recovery steps"
unzip -l "$OUT/Codepass-9.9.9-rc.1-darwin-arm64.zip" | grep -qE '\._|__MACOSX' && fail "archive holds metadata entries"
grep -q '"version": "9.9.9-rc.1"' "$OUT/manifest-darwin-arm64.json" || fail "manifest version"
[ "$(unzip -Z1 "$OUT/Codepass-9.9.9-rc.1-windows-amd64.zip")" = codepass.exe ] || fail "windows archive content"
grep -q '"os": "windows"' "$OUT/manifest-windows-amd64.json" || fail "windows manifest os"
"$OUT/relkit" verify -pub "$(cat "$OUT/test-key/current.pub")" -in "$OUT/manifest-windows-amd64.json" -sig "$OUT/manifest-windows-amd64.json.sig" \
  -archive "$OUT/Codepass-9.9.9-rc.1-windows-amd64.zip" >/dev/null || fail "windows manifest does not verify"
echo "$log" | grep -q 'manifest-windows-amd64.json.sig' || fail "windows assets are not in the printed upload command"
# key material never lands in the log; the dry run signs with its own throwaway key, never the configured one
[ -s "$OUT/test-key/current.priv" ] || fail "the throwaway test key is missing"
echo "$log" | grep -qF "$(cat "$OUT/test-key/current.priv")" && fail "private key leaked into the log"
"$OUT/relkit" verify -pub "$(cat "$OUT/test-key/current.pub")" -in "$OUT/manifest-darwin-arm64.json" -sig "$OUT/manifest-darwin-arm64.json.sig" >/dev/null \
  || fail "the manifest is not signed by the throwaway key"
"$OUT/relkit" verify -pub "$FIXPUB" -in "$OUT/manifest-darwin-arm64.json" -sig "$OUT/manifest-darwin-arm64.json.sig" >/dev/null 2>&1 \
  && fail "a dry run signed with CODEPASS_SIGNING_KEY_FILE"
# a configured key whose public half is not the expected one is reported, without signing
rel CODEPASS_SIGNING_KEY_FILE="$FIX/current.priv" CODEPASS_EXPECT_PUBKEY="$(printf '%064d' 0)" -- \
  --dry-run --app "$T/a/Codepass.app" --out "$T/outx" 9.9.9-rc.1 >"$T/r.out" 2>&1 && fail "accepted a wrong CODEPASS_EXPECT_PUBKEY"

# Names unique to this run. Nothing is deleted from the repository: if one of them appears, the test fails.
OUT_IN="$ROOT/out-test.$$"
[ ! -e "$OUT_IN" ] && [ ! -e "$OUT_IN-dd" ] && [ ! -e "$OUT_IN-2" ] && [ ! -e "$OUT_IN-3" ] || fail "$OUT_IN* already exists"
# refusals: each must fail loudly AND for the stated reason (a refusal that fails for another reason proves nothing)
refuse() { # refuse "reason" "expected text" args...
  local why="$1" want="$2"; shift 2
  case "$want" in *" "*) ;; -*) fail "refuse: expected text missing for: $why";; esac
  if rel -- "$@" >"$T/r.out" 2>&1; then fail "accepted: $why"; fi
  grep -q FAILED "$T/r.out" || { cat "$T/r.out"; fail "no loud failure for: $why"; }
  grep -qF -- "$want" "$T/r.out" || { cat "$T/r.out"; fail "refused for the wrong reason: $why (expected: $want)"; }
}
VMSG="must look like 1.0.0 or 1.0.0-rc.1"
refuse "version rc10"        "$VMSG" --dry-run --app "$T/a/Codepass.app" --out "$T/o2" 9.9.9-rc10
refuse "version with v"      "$VMSG" --dry-run --app "$T/a/Codepass.app" --out "$T/o3" v9.9.9-rc.1
refuse "bundle version"      "bundle version is not 9.9.9-rc.2" --dry-run --app "$T/a/Codepass.app" --out "$T/o4" 9.9.9-rc.2
refuse "leading zero major"  "$VMSG" --dry-run --app "$T/a/Codepass.app" --out "$T/o7" 01.0.0
refuse "leading zero minor"  "$VMSG" --dry-run --app "$T/a/Codepass.app" --out "$T/o7" 1.00.0
refuse "leading zero rc"     "$VMSG" --dry-run --app "$T/a/Codepass.app" --out "$T/o7" 1.0.0-rc.01
refuse "rc.0"                "$VMSG" --dry-run --app "$T/a/Codepass.app" --out "$T/o7" 1.0.0-rc.0
refuse "out inside repo"     "must be outside the repository" --dry-run --app "$T/a/Codepass.app" --out "$OUT_IN" 9.9.9-rc.1
[ ! -e "$OUT_IN" ] || fail "the output folder inside the repository was created"
refuse "out with .. after a missing folder" "'..' component" --dry-run --app "$T/a/Codepass.app" --out "$T/no-such-dir/../../$(basename "$ROOT")/$(basename "$OUT_IN")-dd" 9.9.9-rc.1
[ ! -e "$OUT_IN-dd" ] || fail "the output folder inside the repository was created (..)"
ln -s "$ROOT" "$T/link"
refuse "out inside repo, via a symlinked checkout" "must be outside the repository" --dry-run --app "$T/a/Codepass.app" --out "$T/link/$(basename "$OUT_IN")-2" 9.9.9-rc.1
[ ! -e "$OUT_IN-2" ] || fail "the output folder inside the repository was created (symlink)"
if "$T/link/build/release/release.sh" --dry-run --app "$T/a/Codepass.app" --out "$T/link/$(basename "$OUT_IN")-3" 9.9.9-rc.1 >"$T/r.out" 2>&1; then fail "symlinked checkout accepted an output folder inside the repository"; fi
grep -qF "must be outside the repository" "$T/r.out" || { cat "$T/r.out"; fail "symlinked checkout refused for the wrong reason"; }
[ ! -e "$OUT_IN-3" ] || fail "output written inside the repository (symlink)"
refuse "real run, no identity" "CODEPASS_SIGN_IDENTITY is not set" --out "$T/o5" 9.9.9-rc.1
refuse "real run with --app"   "--app is only for --dry-run" --app "$T/a/Codepass.app" --out "$T/o8" 9.9.9-rc.1
refuse "publish with dry run"  "--publish is a real step" --publish --dry-run --repo acme/x 9.9.9-rc.1
# the same output base can be used twice: every run gets its own folder
rel -- --dry-run --app "$T/a/Codepass.app" --out "$T/out" 9.9.9-rc.1 >"$T/r2.out" 2>&1 || { cat "$T/r2.out"; fail "second run in the same --out failed"; }
[ "$(outdir "$(cat "$T/r2.out")")" != "$OUT" ] || fail "a second run reused the first run's folder"

# a bundle with an AppleDouble file is refused before it is signed
mkdir -p "$T/b"; cp -R "$T/a/Codepass.app" "$T/b/Codepass.app"
echo "x" >"$T/b/Codepass.app/Contents/Resources/._junk"
refuse "AppleDouble file in the bundle" "metadata files that would break the signature" --dry-run --app "$T/b/Codepass.app" --out "$T/o6" 9.9.9-rc.1

# valid_version: a direct table (the dry run only sees it through the fake bundle's version)
for v in 1.0.0 1.0.0-rc.1 1.0.0-rc.10 0.1.0; do valid_version "$v" || fail "valid_version refused $v"; done
for v in 1.0.0-rc10 v1.0.0 01.0.0 1.00.0 1.0.0-rc.01 1.0.0-rc.0 1.0.0-beta 1.0.0-rc.1+b 1.0 1.0.0.0 ""; do
  ! valid_version "$v" || fail "valid_version accepted '$v'"
done
[ "$(win_file_version 1.0.0)" = 1.0.0.0 ] && [ "$(win_file_version 1.0.0-rc.12)" = 1.0.0.12 ] || fail "win_file_version mapping"

# tree_is_clean: fails closed, sees untracked source whatever the user's git setting, sees a deleted file
R="$T/repo"; mkdir -p "$R"; ( cd "$R" && git init -q && git config user.email t@t && git config user.name t \
  && mkdir -p frontend/dist && : >frontend/dist/.gitkeep && echo 'package x' >a.go && git add -A && git commit -qm init )
tree_is_clean "$R" 2>/dev/null || fail "a clean repository was reported dirty"
mkdir -p "$T/nogit"; echo 'package x' >"$T/nogit/a.go"
tree_is_clean "$T/nogit" 2>/dev/null && fail "a folder that is not a repository was reported clean"
( cd "$R" && git config status.showUntrackedFiles no && echo 'package y' >b.go )
tree_is_clean "$R" 2>/dev/null && fail "an untracked .go file was invisible with status.showUntrackedFiles=no"
rm "$R/b.go"
rm "$R/frontend/dist/.gitkeep"
tree_is_clean "$R" 2>/dev/null && fail "a deleted tracked placeholder was not seen"
# (this is what an in-tree build did to the checkout; release.sh now builds in an export of HEAD)

# tree_is_clean: the two dependency links of a worktree are not source; a link under any other name is
R2="$T/repo2"; mkdir -p "$R2/frontend" "$T/deps/a" "$T/deps/b" "$T/deps/c"
( cd "$R2" && git init -q && git config user.email t@t && git config user.name t && echo 'package x' >a.go && git add -A && git commit -qm init \
  && ln -s "$T/deps/a" .tools && ln -s "$T/deps/b" frontend/node_modules )
tree_is_clean "$R2" 2>/dev/null || fail "the .tools and frontend/node_modules links were reported dirty"
( cd "$R2" && ln -s "$T/deps/c" other )
tree_is_clean "$R2" 2>/dev/null && fail "a symlink under another name was reported clean"
rm "$R2/other"

# entitlements: build/darwin/entitlements.plist (CODEPASS_ENTITLEMENTS overrides the path) is applied, dry run too
cat >"$T/ent.plist" <<'PL'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>com.apple.security.automation.apple-events</key><true/></dict></plist>
PL
elog=$(rel CODEPASS_ENTITLEMENTS="$T/ent.plist" -- --dry-run --app "$T/a/Codepass.app" --out "$T/oe" 9.9.9-rc.1 2>&1) || { echo "$elog"; fail "dry run with entitlements failed"; }
EOUT="$(outdir "$elog")"
grep -q 'automation.apple-events' "$EOUT/entitlements.txt" || fail "the entitlement is not in the signature"
grep -q 'automation.apple-events' "$OUT/entitlements.txt" && fail "an entitlement appeared although none is configured"

# a real run refuses CODEPASS_ENTITLEMENTS (a real bundle signs only the committed file)
refuse_env() { local want="$1"; shift; if rel "$@" >"$T/r.out" 2>&1; then fail "accepted: $want"; fi; grep -qF -- "$want" "$T/r.out" || { cat "$T/r.out"; fail "wrong reason, expected: $want"; }; }
refuse_env "CODEPASS_ENTITLEMENTS is only for --dry-run" CODEPASS_ENTITLEMENTS="$T/ent.plist" CODEPASS_SIGN_IDENTITY="Developer ID Application: X (T)" CODEPASS_TEAM_ID=T CODEPASS_NOTARY_PROFILE=p \
  CODEPASS_SIGNING_KEY_FILE="$FIX/current.priv" CODEPASS_EXPECT_PUBKEY="$FIXPUB" GH=gh -- --repo acme/x --out "$T/o9" 9.9.9-rc.1

# --publish, with a stub gh: verifies the draft's assets against the expected key, then publishes
STUB="$T/gh"; GLOG="$T/gh.log"
cat >"$STUB" <<GHSTUB
#!/usr/bin/env bash
echo "\$*" >>"$GLOG"
case "\$1 \$2" in
  "auth status") exit 0 ;;
  "release view") printf 'draft\nCodepass-9.9.9-rc.1-darwin-arm64.zip\nmanifest-darwin-arm64.json\nmanifest-darwin-arm64.json.sig\nCodepass-9.9.9-rc.1-windows-amd64.zip\nmanifest-windows-amd64.json\nmanifest-windows-amd64.json.sig\n' ;;
  "release download") while [ \$# -gt 0 ]; do [ "\$1" = --dir ] && d="\$2"; shift; done; cp "$OUT"/Codepass-*.zip "$OUT"/manifest-*.json* "\$d"/ ;;
  "release edit") exit 0 ;;
  *) exit 1 ;;
esac
GHSTUB
chmod +x "$STUB"
TESTPUB="$(cat "$OUT/test-key/current.pub")"
rel GH="$STUB" CODEPASS_EXPECT_PUBKEY="$FIXPUB" -- --publish --repo acme/x 9.9.9-rc.1 >"$T/p.out" 2>&1 && fail "published a draft signed by another key"
grep -q 'release edit' "$GLOG" && fail "edit ran although verification failed"
rel GH="$STUB" CODEPASS_EXPECT_PUBKEY="$TESTPUB" -- --publish --repo acme/x 9.9.9-rc.1 >"$T/p.out" 2>&1 || { cat "$T/p.out"; fail "publish of a verified draft failed"; }
grep -q 'release edit v9.9.9-rc.1 --repo acme/x --draft=false' "$GLOG" || fail "the draft was not published"

echo "release pipeline dry-run tests passed"
