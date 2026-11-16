#!/usr/bin/env bash
# Launch smoke test of a signed build: build/release/smoke.sh PATH/TO/Codepass.app
#
# Run it with EVERY installed copy quit (Codepass and the old AWS SSO Hub share one bundle ID and one
# single-instance ID: with another copy running, the new one hands over and exits at once, which hides a
# launch failure). The script refuses to start while a copy is running; it never quits one for you.
# Automatic part: the app stays up and opens a window. Guided part: you confirm browser sign-in,
# a keychain read and "Open in terminal". Any "no" fails the test.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/lib.sh"

APP="${1:-}"; [ -d "$APP/Contents/MacOS" ] || die "usage: smoke.sh PATH/TO/Codepass.app"
APP="$(cd "$APP" && pwd -P)"
EXE="$APP/Contents/MacOS/$(plist_get "$APP/Contents/Info.plist" CFBundleExecutable)"
[ -x "$EXE" ] || die "no executable in the bundle"

# 1. no other copy may be running
running="$(/usr/bin/pgrep -fl 'Codepass\.app/Contents/MacOS|AWS SSO Hub\.app/Contents/MacOS' || true)"
# A dev build (wails3 dev: bin/codepass) or a renamed bundle shares the single-instance ID too.
running="$(printf '%s\n%s\n' "$running" "$(/usr/bin/pgrep -lx 'codepass|aws-sso-hub' || true)" | sed '/^$/d')"
[ -z "$running" ] || { printf '%s\n' "$running" >&2; die "another copy is running. Quit it yourself, then run this again."; }

# 2. launch through LaunchServices, as a double click does: the app is then its own responsible process
# (macOS attributes the Automation prompt to Codepass, not to this terminal) and gets launchd's environment,
# not this shell's PATH. Then find the pid of exactly this executable.
log "launching $APP"
/usr/bin/open -n "$APP" || die "open could not launch the app"
PID=""
for _ in $(seq 1 100); do
  PID="$(/bin/ps -axo pid=,comm= | /usr/bin/awk -v exe="$EXE" '{ pid=$1; $1=""; sub(/^ +/, ""); if ($0 == exe) { print pid; exit } }')"
  [ -z "$PID" ] || break
  sleep 0.1
done
[ -n "$PID" ] || die "the app did not appear as a process within 10 seconds ($EXE)"
cleanup() { [ -z "$PID" ] || ! kill -0 "$PID" 2>/dev/null || kill "$PID" 2>/dev/null || true; }
trap cleanup EXIT

# 3. it must stay up and open a window
for _ in 1 2 3 4 5 6 7 8 9 10; do
  sleep 1
  kill -0 "$PID" 2>/dev/null || die "the app exited during launch (another copy probably took over through the single-instance handoff: quit every Codepass / AWS SSO Hub, dev builds included)"
done
wins="$(/usr/bin/osascript -e "tell application \"System Events\" to count windows of (first process whose unix id is $PID)" 2>/dev/null || echo unknown)"
case "$wins" in
  unknown) warn "cannot count windows (Accessibility permission for your terminal?); confirm the window by eye below" ;;
  0) die "the app is running but opened no window" ;;
  *) log "window count: $wins" ;;
esac

ask() { local a; printf '%s [y/N] ' "$1" >&2; read -r a </dev/tty; [ "$a" = y ] || [ "$a" = Y ] || die "not confirmed: $1"; }
ask "Is the Codepass window visible and drawn (not blank)?"
ask "Sign in on a portal: did the browser open and the sign-in complete?"
ask "Is a credential or the saved sign-in read back from the keychain without an error?"
ask "Open a repository and choose Open in terminal: did Terminal or iTerm open (a macOS Automation prompt first is expected, and it must name Codepass, not your terminal)?"
log "SMOKE TEST PASSED for $APP"
