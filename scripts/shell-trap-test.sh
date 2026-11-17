#!/usr/bin/env bash
# Runs the whole Go test suite with HOME and ZDOTDIR pointing at a trap folder whose shell files each append a
# line to a marker file. The run passes only when no marker exists afterwards: no test started a shell that
# read its startup files (task F1-SG). The trap is a new temp folder; the owner's real home is never used.
set -u
cd "$(dirname "$0")/.." || exit 2

# Read the build locations from the real environment first, so the build cache is reused under the fake HOME.
GOCACHE_REAL=$(go env GOCACHE)
GOMODCACHE_REAL=$(go env GOMODCACHE)
GOPATH_REAL=$(go env GOPATH)
GOFLAGS_REAL=$(go env GOFLAGS)

TRAP=$(mktemp -d "${TMPDIR:-/tmp}/shell-trap.XXXXXX") || exit 2
MARKER="$TRAP/marker"
trap 'rm -rf "$TRAP"' EXIT
mkdir -p "$TRAP/home"
for f in .zshenv .zprofile .zshrc .zlogin .bash_profile .bashrc .profile; do
	printf 'echo "%s" >> "%s"\n' "$f" "$MARKER" > "$TRAP/home/$f"
done

# -count=1: a cached result runs nothing, so the trap could not see it (the cache ignores the inherited environment).
env -u BASH_ENV -u ENV \
	HOME="$TRAP/home" ZDOTDIR="$TRAP/home" \
	GOCACHE="$GOCACHE_REAL" GOMODCACHE="$GOMODCACHE_REAL" GOPATH="$GOPATH_REAL" GOFLAGS="$GOFLAGS_REAL" \
	go test -count=1 ./... "$@"
status=$?

if [ -e "$MARKER" ]; then
	echo "shell-trap-test: FAIL, a shell read its startup files:" >&2
	cat "$MARKER" >&2
	exit 1
fi
if [ $status -ne 0 ]; then
	echo "shell-trap-test: go test failed (exit $status); no marker was written" >&2
	exit $status
fi
echo "shell-trap-test: PASS, no startup file was read"
