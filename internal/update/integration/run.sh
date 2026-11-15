#!/bin/bash
# macOS integration test for internal/update (F1-09). It runs the real updater inside signed test bundles against
# a fake GitHub releases API. It needs a Developer ID Application identity, so it runs on the owner's Mac and not in
# CI. Everything lives under $SP (a temp folder); nothing is installed outside it, nothing is notarized or sent
# anywhere (test bundles are signed with --timestamp=none), /Applications is never touched. Delete $SP when done.
#   DEVID_HASH=<sha1 of the Developer ID Application identity> SP=<tmp dir> \
#   [DEVEL_HASH=<sha1 of an Apple Development identity of the same team>] internal/update/integration/run.sh [scenario...]
set -uo pipefail
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/../../.." && pwd)
SP=${SP:?set SP to a temp folder}
: "${DEVID_HASH:?set DEVID_HASH}"
export DEVID_HASH
W=$SP/work                  # baked into the test binaries as workRoot
BID=com.rudian.awsssohub
HOSTILE_BID=com.example.other
PASS=0; FAIL=0
SERVER_PID=""

say() { printf '%s\n' "$*"; }
ok()  { PASS=$((PASS+1)); say "  PASS  $*"; }
bad() { FAIL=$((FAIL+1)); say "  FAIL  $*"; }

# ---- builds -----------------------------------------------------------------
mkdir -p "$SP/bin" "$SP/apps" "$SP/keys" "$SP/zips"
build() { # version
  [ -x "$SP/bin/testapp-$1" ] || (cd "$REPO" && go build -ldflags "-X main.version=$1 -X main.workRoot=$W" -o "$SP/bin/testapp-$1" ./internal/update/integration/testapp)
}
(cd "$REPO" && go build -o "$SP/bin/tool" ./internal/update/integration/tool) || exit 1
[ -f "$SP/keys/current.pub" ] || "$SP/bin/tool" keygen "$SP/keys"
# an unrelated key, never pinned
[ -f "$SP/keys/evil.priv" ] || { mkdir -p "$SP/evilk"; "$SP/bin/tool" keygen "$SP/evilk"; cp "$SP/evilk/current.priv" "$SP/keys/evil.priv"; }
for v in 1.0.0-rc.1 1.0.0-rc.2 1.0.0-rc.3; do build $v; done
# the requirement comes from the code under test, so the test cannot drift from what the app enforces
REQ=$("$SP/bin/tool" requirement)

app() { # name version signmode [bundle-id] [binary-version]  -> $SP/apps/name.app
  local name=$1 ver=$2 mode=$3 id=${4:-$BID} binv=${5:-$2}
  "$HERE/mkapp.sh" "$SP/bin/testapp-$binv" "$SP/apps/$name.app" "$ver" "$id" "$mode" >/dev/null 2>&1 || { say "mkapp $name failed"; exit 1; }
}
zipapp() { # name -> $SP/zips/<name>/Codepass-darwin-arm64.zip
  mkdir -p "$SP/zips/$1"; rm -f "$SP/zips/$1/Codepass-darwin-arm64.zip"
  ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl "$SP/apps/$1.app" "$SP/zips/$1/Codepass-darwin-arm64.zip"
}

# ---- fake server + site -----------------------------------------------------
fresh() {
  stop_server
  rm -rf "$W"; mkdir -p "$W/state" "$W/marker" "$W/site" "$W/install" "$W/rel"
  "$SP/bin/tool" serve -site "$W/site" -repo acme/codepass-test -addrfile "$W/addr" >"$W/server.log" 2>&1 &
  SERVER_PID=$!
  for _ in $(seq 50); do [ -s "$W/addr" ] && break; sleep 0.1; done
  printf '{"server":"%s","repo":"acme/codepass-test","keys":["%s","%s"],"bundleId":"%s","requirement":%s,"gatekeeper":false}\n' \
    "$(cat "$W/addr")" "$(cat "$SP/keys/current.pub")" "$(cat "$SP/keys/next.pub")" "$BID" \
    "$(python3 -c 'import json,sys;print(json.dumps(sys.argv[1]))' "$REQ")" > "$W/config.json"
}
stop_server() { [ -n "$SERVER_PID" ] && kill "$SERVER_PID" 2>/dev/null; SERVER_PID=""; }
# Wails' helper leaves $TMPDIR/wails-update-<pid>.log behind. Every Wails v3 app writes that name, so remove only
# the logs whose first line names this run's install folder ("helper start: target=<...>/install/Codepass.app").
rm_helper_logs() {
  local f
  for f in "${TMPDIR:-/tmp}"/wails-update-*.log; do
    [ -f "$f" ] && head -1 "$f" 2>/dev/null | grep -qF "target=$W/install/" && rm -f "$f"
  done
  return 0
}
cleanup() { stop_server; rm_helper_logs; }
trap cleanup EXIT

# release <tag> <archive-app-name> <manifest-version> [signing-key-name] [notes]
release() {
  local tag=$1 name=$2 mver=$3 key=${4:-current} notes=${5:-"Signed release notes for $3"}
  local d="$W/rel/$tag"; mkdir -p "$d"
  cp "$SP/zips/$name/Codepass-darwin-arm64.zip" "$d/"
  "$SP/bin/tool" manifest -version "$mver" -archive "$d/Codepass-darwin-arm64.zip" -notes "$notes" -out "$d/manifest-darwin-arm64.json"
  "$SP/bin/tool" sign -key "$SP/keys/$key.priv" -in "$d/manifest-darwin-arm64.json" -out "$d/manifest-darwin-arm64.json.sig"
}
publish() { "$SP/bin/tool" publish -site "$W/site" -tag "$1" "$W/rel/$1"/*; }

install_app() { ditto "$SP/apps/$1.app" "$W/install/Codepass.app"; }
snapshot() { # content hashes + inodes + names + signature validity of the installed app dir
  ( cd "$W/install" && find Codepass.app -print0 | sort -z | xargs -0 stat -f '%N %i %z %m' ; \
    find Codepass.app -type f -print0 | sort -z | xargs -0 shasum -a 256 ; ls -A ; \
    codesign --verify --deep --strict Codepass.app 2>&1; echo "csrc=$?" ) | shasum -a 256 | cut -c1-16
}
installed_version() { plutil -extract CFBundleShortVersionString raw -o - "$W/install/Codepass.app/Contents/Info.plist"; }
update() { # prints output; sets OUT and RC
  OUT=$("$W/install/Codepass.app/Contents/MacOS/testapp" "${1:-update}" 2>&1); RC=$?
}
strays() { ls -A "$W/install" | grep -v '^Codepass.app$' | tr '\n' ' '; }

expect_refused() { # <label> <substring of reason>
  local label=$1 want=$2 before=$3
  local after; after=$(snapshot)
  say "  output: $OUT"
  [ "$RC" = 3 ] && ok "$label: exit 3 (refused)" || bad "$label: exit $RC, wanted 3"
  case "$OUT" in *"$want"*) ok "$label: reason contains \"$want\"";; *) bad "$label: reason lacks \"$want\"";; esac
  [ "$before" = "$after" ] && ok "$label: installed app byte-identical (hash $after), same inodes" || bad "$label: INSTALLED APP CHANGED ($before -> $after)"
  [ -z "$(strays)" ] && ok "$label: no stray staging/backup next to the app" || bad "$label: strays: $(strays)"
  sleep 1
  [ ! -s "$W/marker/launches.log" ] && ok "$label: nothing relaunched" || bad "$label: relaunch happened: $(cat "$W/marker/launches.log")"
  [ "$(installed_version)" = "$INSTALLED" ] && ok "$label: version still $INSTALLED" || bad "$label: version now $(installed_version)"
}
expect_swapped() { # <label> <new version>
  local label=$1 new=$2
  say "  output: $OUT"
  [ "$RC" = 0 ] && ok "$label: updater process handed over (exit 0)" || bad "$label: exit $RC"
  for _ in $(seq 100); do grep -q "started $new" "$W/marker/launches.log" 2>/dev/null && break; sleep 0.2; done
  grep -q "started $new" "$W/marker/launches.log" 2>/dev/null && ok "$label: relaunched as $new ($(tr '\n' ';' < "$W/marker/launches.log"))" || bad "$label: no relaunch marker"
  [ "$(installed_version)" = "$new" ] && ok "$label: installed Info.plist version $new" || bad "$label: version $(installed_version)"
  codesign --verify --deep --strict -R="$REQ" "$W/install/Codepass.app" 2>&1 && ok "$label: installed bundle satisfies the requirement" || bad "$label: installed bundle fails requirement"
  [ -z "$(strays)" ] && ok "$label: no .bak / wails-update-* left" || bad "$label: strays: $(strays)"
  [ "$(cat "$W/state/highest")" = "$new" ] && ok "$label: stored highest = $new" || bad "$label: highest = $(cat "$W/state/highest" 2>/dev/null)"
}

# ---- apps used by the scenarios ---------------------------------------------
app rc1 1.0.0-rc.1 devid
app rc2 1.0.0-rc.2 devid
app rc2-adhoc 1.0.0-rc.2 adhoc
app rc2-unsigned 1.0.0-rc.2 none
"$HERE/mkapp.sh" "$SP/bin/testapp-1.0.0-rc.2" "$SP/apps/rc2-wrongid.app" 1.0.0-rc.2 $HOSTILE_BID devid >/dev/null 2>&1
"$HERE/mkapp.sh" "$SP/bin/testapp-1.0.0-rc.2" "$SP/apps/rc2-oldplist.app" 1.0.0-rc.1 $BID devid >/dev/null 2>&1   # binary rc.2, plist+seal say rc.1
"$HERE/mkapp.sh" "$SP/bin/testapp-1.0.0-rc.2" "$SP/apps/rc2-tamper.app" 1.0.0-rc.2 $BID devid >/dev/null 2>&1
echo "tampered after signing" > "$SP/apps/rc2-tamper.app/Contents/Resources/data.txt"
"$HERE/mkapp.sh" "$SP/bin/testapp-1.0.0-rc.3" "$SP/apps/rc3.app" 1.0.0-rc.3 $BID devid >/dev/null 2>&1
if [ -n "${DEVEL_HASH:-}" ]; then export DEVEL_HASH; app rc2-devel 1.0.0-rc.2 devel; fi
for n in rc1 rc2 rc2-adhoc rc2-unsigned rc2-wrongid rc2-oldplist rc2-tamper rc3; do zipapp $n; done
[ -n "${DEVEL_HASH:-}" ] && zipapp rc2-devel
# a zip built without --norsrc --noextattr: add the AppleDouble entry such a zip carries
mkdir -p "$SP/zips/rc2-appledouble"; cp "$SP/zips/rc2/Codepass-darwin-arm64.zip" "$SP/zips/rc2-appledouble/"
python3 - "$SP/zips/rc2-appledouble/Codepass-darwin-arm64.zip" <<'PY'
import sys, zipfile
with zipfile.ZipFile(sys.argv[1], "a") as z:
    z.writestr("rc2.app/Contents/MacOS/._testapp", b"\x00\x05\x16\x07")
PY

# ---- scenarios --------------------------------------------------------------
scn_good() {
  say "[G1] good update rc.1 -> rc.2 (key: current)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2 current "Signed notes: fixes."; publish v1.0.0-rc.2
  update; expect_swapped G1 1.0.0-rc.2
  case "$OUT" in *'notes="Signed notes: fixes."'*) ok "G1: release notes come from the signed manifest, not the API body";; *) bad "G1: notes: $OUT";; esac
}
scn_rotate() {
  say "[G2] good update, manifest signed with the NEXT key (rotation)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2 next; publish v1.0.0-rc.2
  update; expect_swapped G2 1.0.0-rc.2
}
scn_old_archive() {
  say "[R1] old signed archive + old signed manifest re-published under a NEW tag (installed rc.2)"
  fresh; install_app rc2; INSTALLED=1.0.0-rc.2
  release v1.0.0-rc.3 rc1 1.0.0-rc.1; publish v1.0.0-rc.3      # genuine rc.1 manifest, signed long ago
  b=$(snapshot); update; expect_refused R1 "does not equal release tag" "$b"
}
scn_old_same_tag() {
  say "[R1b] old release re-published under its own old tag (installed rc.2)"
  fresh; install_app rc2; INSTALLED=1.0.0-rc.2
  release v1.0.0-rc.1 rc1 1.0.0-rc.1; publish v1.0.0-rc.1
  b=$(snapshot); update
  # inner Wails semver filter says "up to date" before our code runs: exit 4 is also a safe outcome
  if [ "$RC" = 4 ]; then say "  output: $OUT"; ok "R1b: treated as up to date, app untouched=$([ "$b" = "$(snapshot)" ] && echo yes || echo NO)"; else expect_refused R1b "not newer" "$b"; fi
}
scn_below_highest() {
  say "[R1c] correctly signed rc.2, but this machine has already run rc.5 (stored highest)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  echo 1.0.0-rc.5 > "$W/state/highest"
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R1c "below the highest installed" "$b"
}
scn_adhoc() {
  say "[R2] validly signed manifest over an AD-HOC signed bundle"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2-adhoc 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R2 "codesign requirement failed" "$b"
}
scn_unsigned() {
  say "[R2b] validly signed manifest over a bundle with only the linker signature (no bundle seal)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2-unsigned 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R2b "codesign requirement failed" "$b"
}
scn_devel() {
  say "[R2c] validly signed manifest over a bundle signed with an Apple Development certificate of the same team"
  if [ -z "${DEVEL_HASH:-}" ]; then say "  SKIP  set DEVEL_HASH to run this one"; return; fi
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2-devel 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R2c "codesign requirement failed" "$b"
}
scn_appledouble() {
  say "[R10] zip with an AppleDouble ._* entry (built without --norsrc --noextattr)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2-appledouble 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R10 "AppleDouble" "$b"
}
scn_tamper_manifest() {
  say "[R3a] manifest edited after signing (notes changed)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; sed -i '' 's/Signed release notes/EVIL release notes/' "$W/rel/v1.0.0-rc.2/manifest-darwin-arm64.json"; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R3a "signature does not match" "$b"
  say "[R3b] manifest signed by a key that is not pinned"
  fresh; install_app rc1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2 evil; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R3b "signature does not match" "$b"
  say "[R3c] .sig asset missing (signature stripped)"
  fresh; install_app rc1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; rm "$W/rel/v1.0.0-rc.2/manifest-darwin-arm64.json.sig"; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R3c "no signed manifest" "$b"
  say "[R3d] no manifest at all (what the stock GitHub provider would accept)"
  fresh; install_app rc1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; rm "$W/rel/v1.0.0-rc.2"/manifest*; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R3d "no signed manifest" "$b"
  say "[R3e] garbage signature"
  fresh; install_app rc1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; echo "AAAA" > "$W/rel/v1.0.0-rc.2/manifest-darwin-arm64.json.sig"; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R3e "malformed signature" "$b"
}
scn_tamper_archive() {
  say "[R4a] archive bytes flipped after the manifest was signed (same size)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2
  python3 - "$W/rel/v1.0.0-rc.2/Codepass-darwin-arm64.zip" <<'PY'
import sys
p=sys.argv[1]; b=bytearray(open(p,'rb').read()); b[len(b)//2]^=0xFF; open(p,'wb').write(b)
PY
  publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R4a "digest mismatch" "$b"
  say "[R4b] archive padded (size differs from signed size)"
  fresh; install_app rc1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; head -c 4096 /dev/zero >> "$W/rel/v1.0.0-rc.2/Codepass-darwin-arm64.zip"; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R4b "exceeds signed size" "$b"
}
scn_bundle() {
  say "[R5] Developer-ID signed, honest manifest, but wrong bundle identifier ($HOSTILE_BID)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2-wrongid 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R5 "bundle identifier" "$b"
  say "[R6] manifest says rc.2, bundle inside says rc.1 (old bundle inside a new manifest)"
  fresh; install_app rc1
  release v1.0.0-rc.2 rc2-oldplist 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R6 "does not equal manifest version" "$b"
  say "[R7] bundle modified after signing (sealed resource changed), manifest honest to that archive"
  fresh; install_app rc1
  release v1.0.0-rc.2 rc2-tamper 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused R7 "codesign requirement failed" "$b"
}
scn_down() {
  say "[R8] release API answers 500"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; publish v1.0.0-rc.2; touch "$W/site/DOWN"
  b=$(snapshot); update; expect_refused R8 "api 500" "$b"
}
scn_control_ungated() {
  say "[C1] CONTROL: same ad-hoc archive, Wails updater with our provider but NO bundle gate (original is ad-hoc too)"
  fresh; INSTALLED=1.0.0-rc.1
  app rc1-adhoc 1.0.0-rc.1 adhoc; ditto "$SP/apps/rc1-adhoc.app" "$W/install/Codepass.app"
  release v1.0.0-rc.2 rc2-adhoc 1.0.0-rc.2; publish v1.0.0-rc.2
  update ungated
  say "  output: $OUT"
  for _ in $(seq 100); do grep -q "started 1.0.0-rc.2" "$W/marker/launches.log" 2>/dev/null && break; sleep 0.2; done
  if [ "$(installed_version)" = "1.0.0-rc.2" ]; then ok "C1: confirmed, the Wails updater alone swapped in an ad-hoc bundle"; else bad "C1: ungated did not swap (version $(installed_version)); log: $(cat "${TMPDIR:-/tmp}"/wails-update-*.log 2>/dev/null | tail -5)"; fi
}

scn_control_downgrade() {
  say "[C2] CONTROL: installed app is Developer ID signed; update is ad-hoc; Wails updater alone (no bundle gate)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2-adhoc 1.0.0-rc.2; publish v1.0.0-rc.2
  update ungated
  say "  output: $OUT"
  for _ in $(seq 100); do grep -q "started 1.0.0-rc.2" "$W/marker/launches.log" 2>/dev/null && break; sleep 0.2; done
  if [ "$(installed_version)" = "1.0.0-rc.2" ]; then ok "C2: confirmed, Developer ID install was replaced by an ad-hoc bundle (signature valid, so Wails accepts)"; else bad "C2: ungated did not swap (version $(installed_version))"; fi
}
scn_gatekeeper() {
  say "[G3] spctl -a -t exec switched on: an un-notarized Developer ID bundle is refused (proves the check bites; real releases are notarized)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  sed -i '' 's/"gatekeeper":false/"gatekeeper":true/' "$W/config.json"
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; publish v1.0.0-rc.2
  b=$(snapshot); update; expect_refused G3 "gatekeeper assessment failed" "$b"
}

scn_readonly() {
  say "[R9] install folder not writable (standard user, app in /Applications owned by admin)"
  fresh; install_app rc1; INSTALLED=1.0.0-rc.1
  release v1.0.0-rc.2 rc2 1.0.0-rc.2; publish v1.0.0-rc.2
  chmod a-w "$W/install"
  b=$(snapshot); update; chmod u+w "$W/install"; expect_refused R9 "not-writable" "$b"
}

ALL="good rotate old_archive old_same_tag below_highest adhoc devel appledouble unsigned tamper_manifest tamper_archive bundle down readonly gatekeeper control_ungated control_downgrade"
for s in ${@:-$ALL}; do "scn_$s"; done
stop_server
say; say "RESULT: $PASS passed, $FAIL failed"
[ "$FAIL" = 0 ]
