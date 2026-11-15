package toolinstall

// pkgScript is the privileged half of the Homebrew.pkg flow. It is a constant of the binary: it is handed to
// osascript as an argument and never read from a file, so there is no script file for another process to swap.
//
// Arguments:  $1 the downloaded pkg (user-owned, untrusted)   $2 the expected Team ID
//
//	$3 the expected size in bytes, or "-"            $4 the expected sha256 (hex), or "-"
//	$5 mode: "install" or "verify"
//
// Exit codes (every one maps to a sentence of design 5.1, see classify.go):
//
//	0 ok            10 copy failed or not a regular file   11 disk full
//	12 not signed for distribution or the package is broken
//	13 signed by another Team ID    14 not notarized or Gatekeeper rejects
//	15 size or digest differs (a truncated download, a proxy page, a swapped file)
//	20 the prefix already exists    21 not running as root
//	30 installer failed             31 installer finished but brew is missing
//
// Rules this text keeps (checked by the tests in script_test.go):
//   - it runs under /usr/bin/env -i /bin/bash --noprofile --norc, so BASH_ENV, exported functions, SHELLOPTS and
//     PS4 of the caller never reach it; every tool below is called by absolute path;
//   - the header block (BASE ... INSTALLER) is the only part tests replace, line by line; the app never does;
//   - nothing after the copy opens the user's file again; the copy lives in a root-only folder under /var/root;
//   - every reason goes to stderr, because stdout is lost when the script fails.
const pkgScript = `PATH=/usr/bin:/bin:/usr/sbin:/sbin
export PATH
set -u
umask 077
unset BASH_ENV ENV CDPATH DEVELOPER_DIR PS4 IFS

BASE=/var/root
NEEDROOT=1
STRICTPARENT=1
PARENTUID=0
PREFIX=/opt/homebrew
BREW=/opt/homebrew/bin/brew
MAXBYTES=419430400
ID=/usr/bin/id
MKTEMP=/usr/bin/mktemp
STAT=/usr/bin/stat
HEAD=/usr/bin/head
TAIL=/usr/bin/tail
GREP=/usr/bin/grep
TR=/usr/bin/tr
CUT=/usr/bin/cut
RM=/bin/rm
CHMOD=/bin/chmod
CHOWN=/usr/sbin/chown
OPENSSL=/usr/bin/openssl
PKGUTIL=/usr/sbin/pkgutil
SPCTL=/usr/sbin/spctl
INSTALLER=/usr/sbin/installer

fail() { printf 'codepass-pkg: %s\n' "$2" >&2; exit "$1"; }

pkg_src=${1:-}
team_id=${2:-}
want_size=${3:--}
want_sha=${4:--}
mode=${5:-}

re_team='^[A-Z0-9]{10}$'
re_num='^[0-9]+$'
re_sha='^[0-9a-f]{64}$'
[[ -n $pkg_src ]] || fail 10 "no package path"
[[ $team_id =~ $re_team ]] || fail 10 "bad Team ID argument"
[[ $want_size == - || $want_size =~ $re_num ]] || fail 10 "bad size argument"
[[ $want_sha == - || $want_sha =~ $re_sha ]] || fail 10 "bad digest argument"
[[ $mode == install || $mode == verify ]] || fail 10 "bad mode"

if [[ $NEEDROOT == 1 ]]; then
  [[ $("$ID" -u) -eq 0 ]] || fail 21 "must run as root"
fi

# An existing folder may be a user-owned old install or planted links: root never installs into it.
if [[ $mode == install ]] && [[ -e $PREFIX || -L $PREFIX ]]; then
  fail 20 "$PREFIX already exists"
fi

# The folder above the prefix (/opt) must not be somewhere a user process can plant /opt/homebrew, or a link
# there, while the checks below run. Absent is fine (installer creates it as root); otherwise it must be a real
# folder owned by root that group and others cannot write to.
if [[ $mode == install && $STRICTPARENT == 1 ]]; then
  parent=${PREFIX%/*}
  if [[ -e $parent || -L $parent ]]; then
    [[ ! -L $parent && -d $parent ]] || fail 10 "$parent is not a real folder"
    pstat=$("$STAT" -f '%u %Lp' "$parent") || fail 10 "cannot read $parent"
    [[ ${pstat%% *} == "$PARENTUID" ]] || fail 10 "$parent is not owned by root"
    (( (8#${pstat##* } & 8#022) == 0 )) || fail 10 "$parent is writable by others"
  fi
fi

if [[ $want_size != - ]]; then
  (( want_size <= MAXBYTES )) || fail 15 "expected size $want_size is over the limit"
  limit=$((want_size + 1))
else
  limit=$((MAXBYTES + 1))
fi

# Refuse links and anything that is not a regular file BEFORE opening: opening a FIFO would wait for ever.
[[ ! -L $pkg_src ]] || fail 10 "not a regular file (symbolic link)"
[[ -f $pkg_src ]] || fail 10 "not a regular file"

work=$("$MKTEMP" -d "$BASE/.codepass-pkg.XXXXXXXX") || fail 10 "mktemp failed"
[[ -n $work && $work == "$BASE"/.codepass-pkg.* && -d $work ]] || fail 10 "mktemp gave an unexpected folder"
trap '"$RM" -rf "$work"' EXIT
copy=$work/Homebrew.pkg

# 1. Copy FIRST, from ONE open descriptor, at most the expected size plus one byte, into the root-only folder.
{ exec 3<"$pkg_src"; } 2>/dev/null || fail 10 "cannot open the download"
# The file may have been swapped for a device between the check above and the open: judge the open descriptor.
[[ -f /dev/fd/3 ]] || fail 10 "not a regular file"
if ! "$HEAD" -c "$limit" <&3 >"$copy" 2>"$work/cp.err"; then
  if "$GREP" -q "No space left" "$work/cp.err"; then fail 11 "disk full"; fi
  fail 10 "copy failed: $("$TR" '\n' ' ' <"$work/cp.err")"
fi
exec 3<&-
if [[ $NEEDROOT == 1 ]]; then "$CHOWN" root:wheel "$copy" || fail 10 "chown failed"; fi
"$CHMOD" 0400 "$copy" || fail 10 "chmod failed"

# 2. Verify THE COPY. Nothing below opens the user's file again.
got_size=$("$STAT" -f %z "$copy")
if [[ $want_size != - ]]; then
  [[ $got_size == "$want_size" ]] || fail 15 "size $got_size, expected $want_size"
else
  (( got_size <= MAXBYTES )) || fail 15 "the file is larger than $MAXBYTES bytes"
fi
got_sha=$("$OPENSSL" dgst -sha256 -r "$copy" | "$CUT" -d' ' -f1)
if [[ $want_sha != - && $got_sha != "$want_sha" ]]; then
  fail 15 "digest $got_sha, expected $want_sha"
fi

sig=$("$PKGUTIL" --check-signature "$copy" 2>&1)
sig_head=$(printf '%s\n' "$sig" | "$HEAD" -3 | "$TR" '\n' ' ')
[[ $sig == *"Status: signed by a developer certificate issued by Apple for distribution"* ]] || fail 12 "signature: $sig_head"
# The leaf certificate line looks like: "   1. Developer ID Installer: Name (TEAMID)"
re_leaf='^[[:space:]]+1\. Developer ID Installer: .*\(([A-Z0-9]{10})\)[[:space:]]*$'
leaf_team=
leaf=
while IFS= read -r line; do
  if [[ $line =~ $re_leaf ]]; then leaf_team=${BASH_REMATCH[1]}; leaf=$line; break; fi
done <<<"$sig"
[[ -n $leaf_team ]] || fail 12 "signature: no Developer ID Installer certificate"
[[ $leaf_team == "$team_id" ]] || fail 13 "Team ID is $leaf_team, expected $team_id"
[[ $sig == *"Notarization: trusted by the Apple notary service"* ]] || fail 14 "not notarized"

# Second, independent opinion from Gatekeeper. With Gatekeeper turned off spctl cannot give one: the answer is
# then defined as "the signature, Team ID and notarization checks above stand", and the output says so.
gatekeeper=on
gk=$("$SPCTL" --status 2>&1)
if [[ $gk == *"assessments disabled"* ]]; then
  gatekeeper=off
else
  assess=$("$SPCTL" -a -vv -t install "$copy" 2>&1) || fail 14 "spctl: $(printf '%s' "$assess" | "$TR" '\n' ' ')"
  [[ $assess == *"origin=Developer ID Installer:"*"($team_id)"* ]] || fail 14 "spctl: $(printf '%s' "$assess" | "$TR" '\n' ' ')"
fi
echo "verified: size=$got_size team=$team_id sha256=$got_sha gatekeeper=$gatekeeper"
[[ $mode == verify ]] && exit 0

# 3. Install the verified copy: the same file that was verified, in a folder only root can enter.
# The verification took seconds: look at the prefix again right before root writes into it.
[[ ! -e $PREFIX && ! -L $PREFIX ]] || fail 20 "$PREFIX already exists"
out=$("$INSTALLER" -pkg "$copy" -target / 2>&1)
rc=$?
if (( rc != 0 )); then
  printf 'codepass-pkg: installer failed (exit %s). Last lines:\n%s\n' "$rc" "$(printf '%s\n' "$out" | "$TAIL" -n 15)" >&2
  exit 30
fi
[[ -x $BREW ]] || fail 31 "the installer finished but brew is missing"
echo installed
`
