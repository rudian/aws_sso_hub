#!/usr/bin/env bash
# Codepass scrub check: fails when the tree (or a range of commits) holds a
# real name, an ID shape, a home-folder path or an AI attribution line.
#
#   scripts/scrub-check.sh                     scan the tracked and untracked, non-ignored files
#   scripts/scrub-check.sh --tracked           scan tracked files only
#   scripts/scrub-check.sh --commits <range>   also scan, for each commit of a range, its message,
#                                              author and committer fields, changed file names and added lines
#   scripts/scrub-check.sh --root <dir>        scan another git work tree (used by the tests)
#
# The block list of real names is NOT in the repository. Point the environment
# variable CODEPASS_SCRUB_LIST at a file with one entry per line:
#   plain text    case-insensitive substring
#   re:<regex>    case-insensitive extended regular expression
#   # comment     and blank lines are ignored
#
# Findings print as "path:line: rule". The matched text is never printed and a
# block-list match inside a path is replaced by ***, so a CI log does not leak
# the names either. Exit codes: 0 clean, 1 findings, 2 setup error.
#
# Built-in rules (no list needed):
#   attribution   AI co-author / "generated with" / session lines
#   home-path     /Users/<name>, /home/<name>, C:\Users\<name> other than the generic fixture names
#   account-id    12-digit numbers other than the generic fixture account IDs
#   team-id       a 10-character Apple Team ID other than the two public ones the code pins
#   directory-id  an AWS SSO directory ID (d-xxxxxxxxxx) other than the generic fixture ones (one repeated digit, d-1234567890)
#   email         addresses outside example domains and the public hosts the fixtures use
#   secret-shape  access-key and token shapes that are not obviously fake
#
# Skipped: binary files. Symlinks are scanned as their link text. Lock files and go.sum get only the
# block list and the home-path rule. Allowed product strings are removed from a
# line (and from a file name) before the block list sees it.

set -u
export LC_ALL=C

ROOT=""
RANGE=""
TRACKED=0
fail_setup() { echo "scrub-check: $*" >&2; exit 2; }
while [ $# -gt 0 ]; do
  case "$1" in
    --root|--commits)
      [ $# -ge 2 ] && [ -n "$2" ] || fail_setup "$1 needs a value"
      if [ "$1" = --root ]; then ROOT="$2"; else RANGE="$2"; fi
      shift 2 ;;
    --tracked) TRACKED=1; shift ;;
    -h|--help) sed -n '2,34p' "$0"; exit 0 ;;
    *) echo "scrub-check: unknown argument: $1" >&2; exit 2 ;;
  esac
done

# ---- allow lists (generic fixture data and public product strings) ----------
ALLOW_PRODUCT='Claude Code|claude|Codex|OpenCode|XC34P8TNZ6|927JGANW46|com\.rudian\.awsssohub|github\.com/rudian/codepass|rudian/codepass'
ALLOW_ACCOUNT_IDS=' 111122223333 444455556666 777788889999 999900001111 111111111111 222222222222 444444444444 123456789012 000000000000 '
ALLOW_TEAM_IDS=' XC34P8TNZ6 927JGANW46 '
ALLOW_DIR_IDS=' d-1234567890 d-1234567899 '   # plus any ID made of one repeated digit
ALLOW_HOME_NAMES=' alex Shared Public Guest runner runneradmin user username you me name example someone test tester '
ALLOW_EMAIL_DOMAIN='(^|\.)example\.(com|org|net)$|^github\.com$|^gitlab\.com$|^bitbucket\.org$|\.amazonaws\.com$|\.awsapps\.com$|^localhost$'
export ALLOW_ACCOUNT_IDS ALLOW_TEAM_IDS ALLOW_DIR_IDS ALLOW_PRODUCT ALLOW_EMAIL_DOMAIN

# Attribution patterns are written in pieces so this file does not match itself.
ATTR_RE='Co-Authored-''By:|Generated with \[?Claude|Claude-''Session:|claude\.ai/code/session|noreply@anthrop''ic\.com'

command -v git  >/dev/null 2>&1 || fail_setup "git is required"
command -v perl >/dev/null 2>&1 || fail_setup "perl is required"

LIST="${CODEPASS_SCRUB_LIST:-}"
[ -n "$LIST" ] || fail_setup "CODEPASS_SCRUB_LIST is not set. Point it at the block list file kept outside the repository (one name per line)."
[ -f "$LIST" ] || fail_setup "CODEPASS_SCRUB_LIST points at '$LIST', which is not a file."
[ -r "$LIST" ] || fail_setup "CODEPASS_SCRUB_LIST file '$LIST' is not readable."

if [ -z "$ROOT" ]; then
  ROOT="$(cd "$(dirname "$0")/.." && pwd)"
fi
[ "$(git -C "$ROOT" rev-parse --is-inside-work-tree 2>/dev/null)" = true ] || fail_setup "'$ROOT' is not a git work tree."

T="$(mktemp -d "${TMPDIR:-/tmp}/scrub.XXXXXX")" || fail_setup "cannot create a temporary folder"
trap 'rm -rf "$T"' EXIT

perl -ne 'exit 1 if /\0/' "$LIST" || fail_setup "the block list must be UTF-8 text (it contains NUL bytes, e.g. UTF-16)."

# Split the list into fixed strings and regular expressions.
tr -d '\r' < "$LIST" | perl -pe 's/^\xEF\xBB\xBF// if $. == 1' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' | grep -v '^#' | grep -v '^$' > "$T/entries" || true
grep -v '^re:' "$T/entries" > "$T/fixed" || true
grep '^re:' "$T/entries" | sed 's/^re://' | grep -v '^$' > "$T/regex" || true
[ -s "$T/entries" ] || fail_setup "the block list '$LIST' has no entries."
[ -s "$T/fixed" ] || [ -s "$T/regex" ] || fail_setup "the block list has no usable entries (only empty re: lines)."
if [ -s "$T/regex" ]; then
  rc=0; grep -E -f "$T/regex" </dev/null >/dev/null 2>&1 || rc=$?
  [ "$rc" -le 1 ] || fail_setup "the block list has an invalid re: entry."
  # the masking and span checks use Perl, so every re: entry must compile there too
  perl -e 'open(my $f, "<", $ARGV[0]) or exit 2; while (<$f>) { chomp; next unless length; exit 1 unless eval { qr/$_/i } }' "$T/regex" \
    || fail_setup "the block list has a re: entry that Perl cannot compile (use syntax both grep -E and Perl accept)."
fi

# grep wrapper: output to a file; a status above 1 (bad pattern, read error) is a setup error.
# xgrep <outfile> <grep args...>
xgrep() {
  out="$1"; shift
  rc=0; grep "$@" > "$out" 2>/dev/null || rc=$?
  [ "$rc" -le 1 ] || fail_setup "grep failed (status $rc); check the block list."
}

# mask <path>: replace every block-list match with ***
cat > "$T/mask.pl" <<'PL'
my ($fx, $rx, $path) = @ARGV;
my @p;
if (open(my $f, '<', $fx)) { while (<$f>) { chomp; push @p, quotemeta($_) if length } }
if (open(my $r, '<', $rx)) { while (<$r>) { chomp; next unless length; my $q = eval { qr/$_/i }; push @p, $q if $q } }
if (@p) { my $re = join('|', map { "(?:$_)" } @p); $path =~ s/$re/***/gi; }
print $path;
PL
mask() { perl "$T/mask.pl" "$T/fixed" "$T/regex" "$1"; }

# safe_path <path>: the masked path, or a placeholder when a listed name still matches the masked text
safe_path() {
  m="$(mask "$1")"
  printf '%s\n' "$m" > "$T/mp"
  if [ -s "$T/fixed" ]; then
    rc=0; grep -q -i -F -f "$T/fixed" "$T/mp" 2>/dev/null || rc=$?
    [ "$rc" = 1 ] || { printf '(path withheld)'; return 0; }
  fi
  if [ -s "$T/regex" ]; then
    rc=0; grep -q -i -E -f "$T/regex" "$T/mp" 2>/dev/null || rc=$?
    [ "$rc" = 1 ] || { printf '(path withheld)'; return 0; }
  fi
  printf '%s' "$m"
}

FOUND=0
report() { printf '%s:%s: %s\n' "$(safe_path "$2")" "$3" "$1"; FOUND=$((FOUND + 1)); }

# decide.pl <fixed> <regex> <candidates> <text>: of the lines grep flagged, print the numbers of those with a
# list match that is NOT entirely inside an allowed product string. A line where Perl finds no match at all
# (the two engines disagree) cannot be decided and counts as a finding.
cat > "$T/decide.pl" <<'PL'
my ($fx, $rx, $cand, $txt) = @ARGV;
my @p;
if (open(my $f, '<', $fx)) { while (<$f>) { chomp; push @p, qr/\Q$_\E/i if length } }
if (open(my $r, '<', $rx)) { while (<$r>) { chomp; next unless length; my $q = eval { qr/$_/i }; push @p, $q if $q } }
my %c;
open(my $cf, '<', $cand) or exit 2;
while (<$cf>) { chomp; $c{$_} = 1 if length }
my $allow = qr/(?:$ENV{ALLOW_PRODUCT})/i;
open(my $tf, '<', $txt) or exit 2;
while (my $line = <$tf>) {
  next unless $c{$.};
  chomp $line;
  my @spans;
  while ($line =~ /$allow/g) { push @spans, [$-[0], $+[0]] }
  my ($seen, $bad) = (0, 0);
  for my $re (@p) {
    while ($line =~ /(?=($re))/g) {
      my ($s, $e) = ($-[0], $-[0] + length($1));
      $seen = 1;
      my $in = 0;
      for my $sp (@spans) { if ($s >= $sp->[0] && $e <= $sp->[1]) { $in = 1; last } }
      $bad = 1 unless $in;
    }
  }
  print "$.\n" if $bad || !$seen;
}
PL

# block_nums <textfile> <outfile>: line numbers of block-list findings
block_nums() {
  : > "$T/cand"
  if [ -s "$T/fixed" ]; then
    xgrep "$T/g.out" -n -i -F -f "$T/fixed" "$1"; cut -d: -f1 < "$T/g.out" >> "$T/cand"
  fi
  if [ -s "$T/regex" ]; then
    xgrep "$T/g.out" -n -i -E -f "$T/regex" "$1"; cut -d: -f1 < "$T/g.out" >> "$T/cand"
  fi
  : > "$2"
  [ -s "$T/cand" ] || return 0
  perl "$T/decide.pl" "$T/fixed" "$T/regex" "$T/cand" "$1" > "$2" || fail_setup "block-list check failed"
}

# lines_to_report <rule> <numbers-file> <meta-file>
lines_to_report() {
  rule="$1"; nums="$2"; meta="$3"
  [ -s "$nums" ] || return 0
  sort -un "$nums" | awk 'NR==FNR { a[$1]; next } FNR in a { print $0 }' - "$meta" > "$T/hits"
  while IFS="$(printf '\t')" read -r path ln; do report "$rule" "$path" "$ln"; done < "$T/hits"
}

# run_rules <textfile> <metafile> <mode> ; mode all | lock (block list and home-path) | block (block list only)
run_rules() {
  txt="$1"; meta="$2"; mode="$3"

  # block list
  block_nums "$txt" "$T/nums"
  lines_to_report "block-list" "$T/nums" "$meta"
  [ "$mode" = block ] && return 0

  # home-folder paths
  # POSIX forms are case-sensitive (an IAM console URL holds /home#/users/details); Windows paths are not
  : > "$T/g.all"
  xgrep "$T/g.out" -noE '(/Users|/home)/[A-Za-z0-9_][A-Za-z0-9_.-]*' "$txt"; cat "$T/g.out" >> "$T/g.all"
  xgrep "$T/g.out" -noiE '[A-Za-z]:\\+Users\\+[A-Za-z0-9_][A-Za-z0-9_.-]*' "$txt"; cat "$T/g.out" >> "$T/g.all"
  awk -F: -v allow="$ALLOW_HOME_NAMES" 'BEGIN { allow = tolower(allow) } {
      m = $0; sub(/^[0-9]+:/, "", m); n = m; sub(/^.*[\/\\]/, "", n); n = tolower(n)
      if (index(allow, " " n " ") == 0) print $1 }' "$T/g.all" > "$T/nums"
  lines_to_report "home-path" "$T/nums" "$meta"
  [ "$mode" = lock ] && return 0

  # attribution (checked on the raw text: a "Claude" allowance does not excuse these lines)
  xgrep "$T/g.out" -n -i -E "$ATTR_RE" "$txt"; cut -d: -f1 < "$T/g.out" > "$T/nums"
  lines_to_report "attribution" "$T/nums" "$meta"

  # 12-digit account IDs (lookarounds, so IDs next to each other are each seen)
  perl -ne 'while (/(?<![0-9A-Za-z])([0-9]{12})(?![0-9A-Za-z])/g) { print "$.\n" if index($ENV{ALLOW_ACCOUNT_IDS}, " $1 ") < 0 }' "$txt" > "$T/nums"
  lines_to_report "account-id" "$T/nums" "$meta"

  # Apple Team IDs, in the contexts the code and the installers use
  perl -ne '
    my $re = qr{
        (?i:team[ _-]?(?:id|identifier))[^A-Za-z0-9]{0,6}
      | \bTEAM(?:_ID)?[ \t]*=[ \t]*"?
      | subject\.OU\]?\s*=\s*"?
      | (?:Developer\ ID\ (?:Application|Installer)|Apple\ (?:Development|Distribution)):[^\n]*?\(
    }x;
    while (/$re([A-Z0-9]{10})(?![A-Za-z0-9])/g) {
      my $id = $1;
      print "$.\n" if $id =~ /[0-9]/ && index($ENV{ALLOW_TEAM_IDS}, " $id ") < 0;
    }' "$txt" > "$T/nums"
  lines_to_report "team-id" "$T/nums" "$meta"

  # AWS SSO directory IDs
  perl -ne 'while (/(?<![A-Za-z0-9_])(d-[0-9a-f]{10})(?![A-Za-z0-9_])/g) { print "$.\n" if index($ENV{ALLOW_DIR_IDS}, " $1 ") < 0 && $1 !~ /^d-([0-9])\1{9}$/ }' "$txt" > "$T/nums"
  lines_to_report "directory-id" "$T/nums" "$meta"

  # e-mail addresses
  xgrep "$T/g.out" -noE '[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}' "$txt"
  awk -F: '{
      m = $0; sub(/^[0-9]+:/, "", m); d = tolower(m); sub(/^.*@/, "", d)
      if (d !~ ENVIRON["ALLOW_EMAIL_DOMAIN"]) print $1 }' "$T/g.out" > "$T/nums"
  lines_to_report "email" "$T/nums" "$meta"

  # secret shapes (obvious fakes carry EXAMPLE or are too short to be real)
  xgrep "$T/g.out" -noE '\b(AKIA|ASIA)[A-Z0-9]{16}\b|\bghp_[A-Za-z0-9]{36}\b|\bgithub_pat_[A-Za-z0-9_]{40,}\b|\bglpat-[A-Za-z0-9_-]{20,}\b|\bxox[bpa]-[A-Za-z0-9-]{20,}\b' "$txt"
  awk -F: '{ m = $0; sub(/^[0-9]+:/, "", m); if (toupper(m) !~ /EXAMPLE/) print $1 }' "$T/g.out" > "$T/nums"
  xgrep "$T/g.out" -noiE -e 'aws_secret_access_key["'"'"']?[[:space:]]*[=:][[:space:]]*["'"'"']?[A-Za-z0-9/+]{40}' "$txt"
  awk -F: '{ m = $0; sub(/^[0-9]+:/, "", m); if (toupper(m) !~ /EXAMPLE/) print $1 }' "$T/g.out" >> "$T/nums"
  xgrep "$T/g.out" -noE -e '-----BEGIN [A-Z ]*PRIVATE KEY-----' "$txt"; cut -d: -f1 < "$T/g.out" >> "$T/nums"
  lines_to_report "secret-shape" "$T/nums" "$meta"
}

# name_check <file-with-one-path-per-line> : file names against the block list
name_check() {
  block_nums "$1" "$T/namehits"
  sort -un "$T/namehits" > "$T/namehits.u"
  while IFS= read -r n; do
    report "block-list (file name)" "$(sed -n "${n}p" "$1")" 0
  done < "$T/namehits.u"
}

# ---- the work tree ------------------------------------------------------------
# Two groups: normal files get every rule; lock files and go.sum only the block list and home-path.
LOCK_RE='(^|/)(package-lock\.json|go\.sum|yarn\.lock|pnpm-lock\.yaml)$'
if [ "$TRACKED" = 1 ]; then
  git -C "$ROOT" ls-files -z > "$T/files.z" 2>/dev/null || fail_setup "git ls-files failed in '$ROOT'"
else
  git -C "$ROOT" ls-files -co --exclude-standard -z > "$T/files.z" 2>/dev/null || fail_setup "git ls-files failed in '$ROOT'"
fi
[ -s "$T/files.z" ] || fail_setup "no files to scan in '$ROOT'"
perl -0ne 'exit 1 if /[\t\n\r]/' "$T/files.z" || fail_setup "a listed path contains a tab or line break; rename it."
tr '\0' '\n' < "$T/files.z" > "$T/files" || true
: > "$T/meta"; : > "$T/text"; : > "$T/lmeta"; : > "$T/ltext"
while IFS= read -r f; do
  p="$ROOT/$f"
  if [ -L "$p" ]; then
    # a link is published as a blob holding its target: scan that text with every rule
    printf '%s\t1\n' "$f" >> "$T/meta"
    printf '%s\n' "$(readlink "$p" | tr '\n\r' '  ')" >> "$T/text"
    continue
  fi
  [ -f "$p" ] || continue
  rc=0; grep -qI '' "$p" 2>/dev/null || rc=$?
  [ "$rc" -le 1 ] || fail_setup "cannot read a listed file: $(safe_path "$f")"
  [ "$rc" = 0 ] || continue
  if printf '%s\n' "$f" | grep -qE "$LOCK_RE"; then M="$T/lmeta"; X="$T/ltext"; else M="$T/meta"; X="$T/text"; fi
  awk -v f="$f" -v m="$M" -v x="$X" '{ printf "%s\t%d\n", f, FNR >> m; print $0 >> x }' "$p"
done < "$T/files"

run_rules "$T/text" "$T/meta" all
run_rules "$T/ltext" "$T/lmeta" lock
name_check "$T/files"

# ---- commits: messages, author fields, changed file names, added lines --------
if [ -n "$RANGE" ]; then
  git -C "$ROOT" rev-parse --verify --quiet "${RANGE%%..*}" >/dev/null 2>&1 || [ "${RANGE#*..}" != "$RANGE" ] || fail_setup "bad commit range '$RANGE'"
  # fixed diff shape: no quoted non-ASCII names, a/ and b/ prefixes, merges shown against the first parent
  HIST=(-c core.quotePath=false -c diff.noprefix=false -c diff.mnemonicPrefix=false)
  git -C "$ROOT" log --format='%H' "$RANGE" 2>/dev/null > "$T/commits" || fail_setup "git log failed for '$RANGE'"

  # message bodies: every rule
  : > "$T/meta"; : > "$T/text"
  while IFS= read -r sha; do
    git -C "$ROOT" log -1 --format='%B' "$sha" | while IFS= read -r l; do
      printf 'commit-%s\t0\n' "$(printf '%s' "$sha" | cut -c1-10)" >> "$T/meta"
      printf '%s\n' "$l" >> "$T/text"
    done
  done < "$T/commits"
  run_rules "$T/text" "$T/meta" all

  # author and committer fields: block list only (a real author address is expected there)
  : > "$T/meta"; : > "$T/text"
  while IFS= read -r sha; do
    git -C "$ROOT" log -1 --format='%an <%ae>%n%cn <%ce>' "$sha" | while IFS= read -r l; do
      printf 'commit-%s (author)\t0\n' "$(printf '%s' "$sha" | cut -c1-10)" >> "$T/meta"
      printf '%s\n' "$l" >> "$T/text"
    done
  done < "$T/commits"
  run_rules "$T/text" "$T/meta" block

  # changed file names of every commit
  git -C "$ROOT" "${HIST[@]}" log --root --src-prefix=a/ --dst-prefix=b/ --no-ext-diff --no-textconv --diff-merges=first-parent --name-only -z --format='commit %H%x00' "$RANGE" > "$T/names.z" 2>/dev/null || fail_setup "git log failed for '$RANGE'"
  perl -0ne 'BEGIN { $sha = "" } chomp; s/^\n+//; if (/^commit ([0-9a-f]{40})$/) { $sha = substr($1, 0, 10); next } next unless length; exit 3 if /[\t\n\r]/; print "commit-$sha:$_\n"' "$T/names.z" > "$T/cfiles" \
    || fail_setup "a changed path in '$RANGE' contains a tab or line break; it cannot be checked."
  name_check "$T/cfiles"

  # added lines of every commit (a name added and removed again still ships in history)
  : > "$T/meta"; : > "$T/text"; : > "$T/lmeta"; : > "$T/ltext"
  git -C "$ROOT" "${HIST[@]}" log --root -p --src-prefix=a/ --dst-prefix=b/ --diff-merges=first-parent --no-ext-diff --no-textconv --no-color --format='commit %H' "$RANGE" > "$T/log.p" 2>/dev/null || fail_setup "git log failed for '$RANGE' (a missing object?)"
  awk -v m="$T/meta" -v x="$T/text" -v lm="$T/lmeta" -v lx="$T/ltext" '
      /^commit [0-9a-f]{40}$/ { sha = substr($2, 1, 10); hdr = 0; hunk = 0; next }
      /^diff --git / { hdr = 1; hunk = 0; path = "?"; next }
      hdr && /^\+\+\+ / {
        p = substr($0, 5); sub(/\t.*$/, "", p)
        if (p ~ /^"b\//) { path = substr(p, 4); sub(/"$/, "", path) }
        else if (p ~ /^b\//) path = substr(p, 3)
        next }
      hdr && /^@@ / { hdr = 0; hunk = 1; next }
      hunk && /^\+/ {
        lock = (path ~ /(^|\/)(package-lock\.json|go\.sum|yarn\.lock|pnpm-lock\.yaml)$/)
        if (lock) { printf "commit-%s:%s\t0\n", sha, path >> lm; print substr($0, 2) >> lx }
        else { printf "commit-%s:%s\t0\n", sha, path >> m; print substr($0, 2) >> x }
      }' "$T/log.p"
  run_rules "$T/text" "$T/meta" all
  run_rules "$T/ltext" "$T/lmeta" lock
fi

if [ "$FOUND" -gt 0 ]; then
  echo "scrub-check: $FOUND finding(s). Matched text is not printed; open the file at the line." >&2
  exit 1
fi
echo "scrub-check: clean"
exit 0
