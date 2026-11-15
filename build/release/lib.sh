# Shared helpers for the release scripts. Sourced, never run.
# Nothing here knows an identity name, a notary profile, a key location or a home folder:
# those come from the environment or from the git-ignored build/release/local.env.

log()  { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
warn() { printf '[%s] WARNING: %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
die()  { printf '[%s] FAILED: %s\n' "$(date +%H:%M:%S)" "$*" >&2; exit 1; }

# need_var NAME  -> dies unless the variable is set and not empty.
need_var() {
  eval "[ -n \"\${$1:-}\" ]" || die "$1 is not set. Set it in the environment or in build/release/local.env (see docs/release.md)."
}

# abs_tool NAME  -> prints the absolute path of a tool found on PATH, or dies.
abs_tool() {
  local p
  p="$(command -v "$1" 2>/dev/null)" || die "required tool not found: $1"
  case "$p" in /*) printf '%s\n' "$p" ;; *) die "tool $1 resolved to a non absolute path: $p" ;; esac
}

# On macOS a developer-tool stub (git, make, clang, python3, xcrun) opens an install dialog when
# the Command Line Tools are missing. Check xcode-select first (standing checklist item 10).
require_dev_tools() {
  local d
  d="$(/usr/bin/xcode-select -p 2>/dev/null)" || die "Command Line Tools are not installed (xcode-select -p fails)"
  [ -n "$d" ] && [ "$d" != "/" ] && [ -d "$d" ] || die "xcode-select -p printed '$d'"
}

# Release version: 1.0.0 or 1.0.0-rc.N (a dot before the number: rc10 would sort below rc2). N >= 1, so that
# win_file_version is one-to-one (1.0.0 -> 1.0.0.0, rc.N -> 1.0.0.N).
# No leading zeros, as semver (and the app's x/mod/semver check) requires.
valid_version() { printf '%s' "$1" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-rc\.[1-9][0-9]*)?$'; }

# The tree must be clean: no change to a tracked file and no untracked file the build could compile.
# git runs on its own so that a failure (not a repository, dubious ownership, broken config) is a failure
# and never "clean". --untracked-files=all ignores the user's status.showUntrackedFiles setting.
# The two dependency links of a worktree are not source. git lists a symlink without a trailing slash
# (`?? .tools`) and files under a real folder with one, so both forms are matched.
tree_is_clean() {
  local root="$1" raw out
  raw="$(git -C "$root" status --porcelain --untracked-files=all --ignore-submodules=none 2>&1)" \
    || { printf 'git status failed in %s: %s\n' "$root" "$raw" >&2; return 1; }
  out="$(printf '%s\n' "$raw" | grep -Ev '^$|^\?\? (\.tools|frontend/node_modules)(/|$)' || true)"
  [ -z "$out" ] || { printf '%s\n' "$out" >&2; return 1; }
}

# phys_path P -> the physical path of P even when P (and some parents) do not exist yet.
phys_path() {
  local p="$1" rest=""
  while [ ! -d "$p" ]; do rest="/$(basename "$p")$rest"; p="$(dirname "$p")"; done
  printf '%s%s\n' "$(cd "$p" && pwd -P)" "$rest"
}

# plist_get FILE KEY
plist_get() { /usr/bin/plutil -extract "$2" raw -o - "$1" 2>/dev/null; }

# The designated requirement the updater enforces (plan 4.14 item 4). Arguments: bundle id, team id.
requirement() {
  printf 'anchor apple generic and identifier "%s" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "%s"' "$1" "$2"
}

# check_requirement APP BUNDLE_ID TEAM_ID -> codesign's own exit status: 0 when the bundle satisfies the
# requirement, 3 when it does not, 1 for a malformed requirement or other failure.
check_requirement() {
  /usr/bin/codesign --verify --deep --strict "-R=$(requirement "$2" "$3")" "$1" >/dev/null 2>&1
}

# Marks a pipeline step as done; require_steps fails if one required step never ran.
STEPS_FILE=""
step_done() { printf '%s\n' "$1" >>"$STEPS_FILE"; }
require_steps() {
  local s missing=""
  for s in "$@"; do grep -qx "$s" "$STEPS_FILE" || missing="$missing $s"; done
  [ -z "$missing" ] || die "steps were skipped:$missing"
}

# gh_signed_in GH -> 0 when the account gh will actually use on github.com is signed in. Plain
# `gh auth status` exits 1 when ANY stored account has a problem, which would stop a release for an
# expired token of an account the release never uses.
gh_signed_in() { "$1" auth status --active --hostname github.com >/dev/null 2>&1; }

# win_file_version VERSION -> the four-part Windows file version: 1.0.0 -> 1.0.0.0, 1.0.0-rc.2 -> 1.0.0.2
# (build/windows/info.json must say this; the number after the last dot is the rc number, 0 for a final).
# One-to-one because valid_version requires N >= 1. F1-20's "exe file version equals manifest version" gate
# builds on this mapping.
win_file_version() { case "$1" in *-rc.*) printf '%s.%s\n' "${1%%-rc.*}" "${1##*-rc.}" ;; *) printf '%s.0\n' "$1" ;; esac; }
