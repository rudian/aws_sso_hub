#!/usr/bin/env bash
# Cases for scripts/scrub-check.sh. Every name here is invented.
# Run: scripts/scrub-check.test.sh
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
CHECK="$HERE/scrub-check.sh"
W="$(mktemp -d "${TMPDIR:-/tmp}/scrubtest.XXXXXX")"
trap 'rm -rf "$W"' EXIT
PASS=0; FAILN=0

LIST="$W/list.txt"
printf '# invented names\nZorblax Industries\nquuxington\nre:planet-[0-9]+-corp\n' > "$LIST"

REPO="$W/repo"
mkdir -p "$REPO"
git -C "$REPO" init -q
export GIT_AUTHOR_NAME="Test Person" GIT_AUTHOR_EMAIL=t@example.com GIT_COMMITTER_NAME="Test Person" GIT_COMMITTER_EMAIL=t@example.com
git -C "$REPO" config user.email t@example.com
git -C "$REPO" config user.name "Test Person"

clean_repo() { git -C "$REPO" rm -rq --cached . 2>/dev/null; find "$REPO" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +; }

# expect <name> <want-exit> <want-output-substring-or-empty> <env: list path or UNSET or MISSING> [args...]
expect() {
  name="$1"; want="$2"; sub="$3"; lst="$4"; shift 4
  case "$lst" in
    UNSET) out="$(env -u CODEPASS_SCRUB_LIST "$CHECK" --root "$REPO" "$@" 2>&1)"; rc=$? ;;
    *) out="$(CODEPASS_SCRUB_LIST="$lst" "$CHECK" --root "$REPO" "$@" 2>&1)"; rc=$? ;;
  esac
  ok=1
  [ "$rc" = "$want" ] || ok=0
  if [ -n "$sub" ]; then case "$out" in *"$sub"*) ;; *) ok=0 ;; esac; fi
  if [ "$ok" = 1 ]; then PASS=$((PASS + 1)); echo "ok   $name"
  else FAILN=$((FAILN + 1)); echo "FAIL $name (exit $rc, wanted $want, wanted output '$sub')"; echo "$out" | sed 's/^/     | /'; fi
}

put() { mkdir -p "$(dirname "$REPO/$1")"; printf '%s\n' "$2" > "$REPO/$1"; }

# ---- setup errors
put a.txt "hello"
expect "unset variable fails with a clear message" 2 "CODEPASS_SCRUB_LIST is not set" UNSET
expect "missing list file fails with a clear message" 2 "not a file" "$W/nope.txt"
: > "$W/empty.txt"; printf '# only a comment\n' > "$W/comments.txt"
expect "list with no entries fails" 2 "no entries" "$W/comments.txt"

# ---- clean tree, generic fixtures
clean_repo
put src/a.ts "const p = { account: '111122223333', mail: 'alex@acme.example.com', home: '/Users/alex/code', repo: 'acme_api' }"
put src/b.go "// Claude Code, claude, Codex, OpenCode; team XC34P8TNZ6 and Team ID 927JGANW46; com.rudian.awsssohub"
put src/c.ts "const email = 'someone@git-codecommit.us-east-1.amazonaws.com'; const k = 'AKIAEXAMPLE0000000000'"
expect "generic fixtures pass" 0 "clean" "$LIST"

# ---- block list
clean_repo
put src/x.ts "const owner = 'ZORBLAX industries'"
expect "block list: plain name, case-insensitive" 1 "src/x.ts:1: block-list" "$LIST"
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$REPO" 2>&1)"
case "$out" in *ZORBLAX*|*Zorblax*|*zorblax*) FAILN=$((FAILN + 1)); echo "FAIL output echoes the matched name" ;; *) PASS=$((PASS + 1)); echo "ok   output never echoes the matched name" ;; esac
clean_repo
put src/y.ts "repo planet-42-corp"
expect "block list: re: entry" 1 "src/y.ts:1: block-list" "$LIST"
clean_repo
put quuxington-notes.txt "nothing here"
expect "block list: file name is reported with the name masked" 1 "***-notes.txt" "$LIST"
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$REPO" 2>&1)"
case "$out" in *uuxington*) FAILN=$((FAILN + 1)); echo "FAIL file-name finding echoes the name" ;; *) PASS=$((PASS + 1)); echo "ok   file-name finding never echoes the name" ;; esac
clean_repo
printf 'Zorblax Industries \x00\x01\x02 binary' > "$REPO/pic.png"
expect "binary files are skipped" 0 "clean" "$LIST"
clean_repo
printf 'Zorblax Industries \x00\x01 bytes' > /dev/null
mkdir -p "$REPO/docs/screenshots"; printf 'Zorblax Industries \x00\x01\x02 png' > "$REPO/docs/screenshots/a-01.png"
expect "docs/screenshots: a binary file with a listed word stays clean" 0 "clean" "$LIST"
put docs/screenshots/readme.txt "Zorblax Industries"
expect "docs/screenshots: a text file with a listed name is scanned" 1 "docs/screenshots/readme.txt:1: block-list" "$LIST"
clean_repo
put docs/screenshots/quuxington-01.png "x"
expect "docs/screenshots: a listed file name is reported masked" 1 "***-01.png" "$LIST"
clean_repo
put src/z.ts "untracked file with quuxington"
expect "untracked files are scanned" 1 "src/z.ts:1: block-list" "$LIST"
printf 'src/z.ts\n' > "$REPO/.gitignore"
expect "ignored files are not scanned" 0 "clean" "$LIST"
rm -f "$REPO/.gitignore"

# ---- allowed product strings do not trip a list entry that overlaps them
clean_repo
printf 'Claude Code\nre:codex\n' > "$W/overlap.txt"
put src/p.ts "Run Claude Code, claude or Codex or OpenCode"
expect "allowed product strings are removed before matching" 0 "clean" "$W/overlap.txt"

# ---- built-in rules
clean_repo
put src/h.ts "const home = '/Us""ers/jordan/code'"
expect "home-folder path (macOS)" 1 "src/h.ts:1: home-path" "$LIST"
clean_repo
put src/h.ts "const home = '/ho""me/jordan/code'"
expect "home-folder path (Linux)" 1 "home-path" "$LIST"
clean_repo
put src/h.ts 'const home = "C:\\Us''ers\\jordan\\code"'
expect "home-folder path (Windows)" 1 "home-path" "$LIST"
clean_repo
put src/h.ts "x = /ho""me/.gitconfig and /Us""ers/ and /Us""ers/Shared/x"
expect "non-user home look-alikes pass" 0 "clean" "$LIST"

clean_repo
put src/i.ts "const acct = '1234""12341234'"
expect "unknown 12-digit account ID" 1 "account-id" "$LIST"
clean_repo
put src/i.ts "const ts = 1700000000000; hash = 'abc1234""12341234x'"
expect "13-digit timestamps and hash fragments pass" 0 "clean" "$LIST"

clean_repo
put src/t.txt "TeamIdentifier=ABCDE""12345"
expect "unknown Apple Team ID" 1 "team-id" "$LIST"
clean_repo
put src/t.txt "TeamIdentifier=XC34P8TNZ6 and team id: 927JGANW46"
expect "the two public Team IDs pass" 0 "clean" "$LIST"

clean_repo
put src/m.ts "mail('pat@somecompany"".io')"
expect "e-mail on a real-looking domain" 1 "email" "$LIST"
clean_repo
put src/m.ts "mail('pat@example.com'); mail('a@b.example.com'); git@github.com:x/y.git"
expect "example and public hosts pass" 0 "clean" "$LIST"

clean_repo
put src/k.ts "key = 'AKIA""ABCDEFGHIJKLMNOP'"
expect "access key shape" 1 "secret-shape" "$LIST"
clean_repo
put src/k.ts "tok = 'ghp_""0123456789abcdef0123456789abcdef0123'"
expect "GitHub token shape" 1 "secret-shape" "$LIST"

# AI attribution lines (built from pieces so this file does not match itself)
clean_repo
put NOTES.md "Co-Authored-""By: Some Bot <bot@example.com>"
expect "AI co-author line" 1 "NOTES.md:1: attribution" "$LIST"
clean_repo
put NOTES.md "Generated with [""Claude Code](https://example.com)"
expect "generated-with line" 1 "attribution" "$LIST"

# ---- commit messages
clean_repo
put ok.txt "fine"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "add ok file"
BASE="$(git -C "$REPO" rev-parse HEAD)"
put ok2.txt "fine"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "mention quuxington here"
expect "commit message with a listed name" 1 "commit-" "$LIST" --commits "$BASE..HEAD"
git -C "$REPO" commit -q --amend -m "plain message

Co-Authored-""By: Bot <bot@example.com>"
expect "commit message with attribution" 1 "attribution" "$LIST" --commits "$BASE..HEAD"
git -C "$REPO" commit -q --amend -m "plain message"
expect "clean commit range" 0 "clean" "$LIST" --commits "$BASE..HEAD"

git -C "$REPO" commit -q --amend --allow-empty -m "plain message" --author="Quuxington Fan <q@example.com>"
expect "author field with a listed name" 1 "(author)" "$LIST" --commits "$BASE..HEAD"


# commit history: a name added in one commit and removed in the next still ships
clean_repo
put ok.txt "fine"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "base2" --allow-empty
B2="$(git -C "$REPO" rev-parse HEAD)"
put leak.txt "owner quuxington"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "add a file"
rm -f "$REPO/leak.txt"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "remove the file"
expect "tree is clean after the name is removed" 0 "clean" "$LIST"
expect "name added in one commit and removed in the next" 1 "commit-" "$LIST" --commits "$B2..HEAD"
put quuxington-file.txt "x"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "file with a listed name"
git -C "$REPO" rm -q quuxington-file.txt && git -C "$REPO" commit -qm "drop it"
B3="$(git -C "$REPO" rev-parse HEAD~1)"
expect "listed file name in history" 1 "***-file.txt" "$LIST" --commits "$B3~1..HEAD"

# ---- more ID shapes
clean_repo
put src/i.ts "ids = 111122223333,1234""12341234"
expect "unknown account ID right after an allowed one (comma)" 1 "account-id" "$LIST"
clean_repo
put src/i.ts "ids = 111122223333 1234""12341234"
expect "unknown account ID right after an allowed one (space)" 1 "account-id" "$LIST"
clean_repo
put src/t.go 'leaf[subject.OU] = "ABCDE''12345"'
expect "team ID in a designated requirement" 1 "team-id" "$LIST"
clean_repo
put src/t.txt "Developer ID Installer: Some Name (ABCDE""12345)"
expect "team ID in a pkgutil line" 1 "team-id" "$LIST"
clean_repo
put src/t.sh "TEAM=ABCDE""12345"
expect "team ID in TEAM=" 1 "team-id" "$LIST"
clean_repo
put src/t.go 'leaf[subject.OU] = "XC34P8TNZ6"; TEAM=927JGANW46'
expect "public team IDs pass in every context" 0 "clean" "$LIST"
clean_repo
put src/d.ts "url = 'https://d-9876""543210.awsapps.com/start'"
expect "unknown directory ID" 1 "directory-id" "$LIST"
clean_repo
put src/d.ts "urls = ['https://d-1234567890.awsapps.com/start', 'd-1111111111', 'd-0000000000', 'd-1234567899']"
expect "generic directory IDs pass" 0 "clean" "$LIST"

# ---- block list robustness
clean_repo
put src/y.ts "repo planet-42-corp"
printf 'planet-[0-9]+-corp\nre:planet-[0-9]+-corp\nre:foo(\n' > "$W/badre.txt"
expect "invalid re: entry is a setup error, not a silent pass" 2 "invalid re:" "$W/badre.txt"
expect "--commits as the last argument" 2 "needs a value" "$LIST" --commits
expect "--commits with an empty value" 2 "needs a value" "$LIST" --commits ""
out="$(CODEPASS_SCRUB_LIST="$LIST" perl -e 'alarm 10; exec @ARGV' "$CHECK" --root 2>&1)"; rc=$?
if [ "$rc" = 2 ]; then PASS=$((PASS + 1)); echo "ok   --root as the last argument ends with a setup error"; else FAILN=$((FAILN + 1)); echo "FAIL --root as the last argument (exit $rc)"; fi

# ---- allowed product strings also apply to file names
clean_repo
printf 'claude\n' > "$W/claude.txt"
put hooks/.claude-plugin/plugin.json "{}"
expect "allowed product string in a file name" 0 "clean" "$W/claude.txt"

# ---- lock files keep the block list and home-path rules
clean_repo
put package-lock.json '{"resolved": "https://registry.quuxington.example/x.tgz"}'
expect "lock file: block list applies" 1 "package-lock.json:1: block-list" "$LIST"
clean_repo
put go.sum "x /Us""ers/jordan/y"
expect "lock file: home-path applies" 1 "go.sum:1: home-path" "$LIST"
clean_repo
put package-lock.json '{"integrity": "sha512-AKIA''ABCDEFGHIJKLMNOP"}'
expect "lock file: hashes are not scanned for secret shapes" 0 "clean" "$LIST"

# ---- review fixes: roots that cannot be listed, history shapes, masking, allowed spans, BOM, e-mail
clean_repo
put src/x.ts "owner quuxington"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "seed"
expect "a .git folder as root is a setup error" 2 "not a git work tree" "$LIST" --root "$REPO/.git"
BARE="$W/bare.git"; git clone -q --bare "$REPO" "$BARE"
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$BARE" 2>&1)"; rc=$?
if [ "$rc" = 2 ]; then PASS=$((PASS + 1)); echo "ok   a bare repository as root is a setup error"; else FAILN=$((FAILN + 1)); echo "FAIL bare root (exit $rc)"; fi
EMPTY="$W/emptyrepo"; mkdir -p "$EMPTY"; git -C "$EMPTY" init -q
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$EMPTY" 2>&1)"; rc=$?
if [ "$rc" = 2 ]; then PASS=$((PASS + 1)); echo "ok   a tree with no files is a setup error"; else FAILN=$((FAILN + 1)); echo "FAIL empty tree (exit $rc)"; fi
clean_repo
put src/u.txt "owner quuxington"; chmod 000 "$REPO/src/u.txt"
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$REPO" 2>&1)"; rc=$?
chmod 644 "$REPO/src/u.txt"
if [ "$(id -u)" = 0 ] || [ "$rc" = 2 ]; then PASS=$((PASS + 1)); echo "ok   an unreadable listed file is a setup error"; else FAILN=$((FAILN + 1)); echo "FAIL unreadable file (exit $rc)"; fi

# history: non-ASCII names, noprefix, merge commits
clean_repo
git -C "$REPO" commit -qm "base3" --allow-empty
B4="$(git -C "$REPO" rev-parse HEAD)"
put keep.txt "fine"; put "文档.txt" "owner quuxington"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "add"
git -C "$REPO" rm -q "文档.txt" && git -C "$REPO" commit -qm "rm"
expect "history: word added to a non-ASCII file name and removed" 1 "commit-" "$LIST" --commits "$B4..HEAD"
git -C "$REPO" config diff.noprefix true
expect "history: diff.noprefix=true does not hide added lines" 1 "commit-" "$LIST" --commits "$B4..HEAD"
git -C "$REPO" config --unset diff.noprefix
clean_repo
git -C "$REPO" commit -qm "base4" --allow-empty
B5="$(git -C "$REPO" rev-parse HEAD)"
git -C "$REPO" checkout -q -b side; put side.txt "x"; git -C "$REPO" add -A; git -C "$REPO" commit -qm "side"
git -C "$REPO" checkout -q -; put main.txt "y"; git -C "$REPO" add -A; git -C "$REPO" commit -qm "main"
git -C "$REPO" merge -q --no-ff --no-commit side
put evil.txt "owner quuxington"; git -C "$REPO" add -A; git -C "$REPO" commit -qm "merge"
git -C "$REPO" rm -q evil.txt; git -C "$REPO" commit -qm "drop"
expect "history: word added only in a merge commit" 1 "commit-" "$LIST" --commits "$B5..HEAD"
git -C "$REPO" branch -q -D side 2>/dev/null

# masking: grep-only syntax in a re: entry
printf 're:\\<quuxco\\>\n' > "$W/wb.txt"
clean_repo
put quuxco-notes.txt "x"
out="$(CODEPASS_SCRUB_LIST="$W/wb.txt" "$CHECK" --root "$REPO" 2>&1)"; rc=$?
case "$out" in *quuxco*) FAILN=$((FAILN + 1)); echo "FAIL word-boundary entry leaks a name"; echo "$out" | sed 's/^/     | /' ;; *) if [ "$rc" = 1 ]; then PASS=$((PASS + 1)); echo "ok   word-boundary re: entry never prints the name"; else FAILN=$((FAILN + 1)); echo "FAIL word-boundary entry (exit $rc)"; fi ;; esac
printf 're:quuxco**\n' > "$W/wb2.txt"
expect "re: entry Perl cannot compile is a setup error" 2 "" "$W/wb2.txt"

# allowed strings excuse a match only when the match lies inside them
clean_repo
printf 'Codexia Zorblabs\n' > "$W/cx.txt"
put src/q.txt "owner Codexia Zorblabs"
expect "entry that contains an allowed string still matches" 1 "block-list" "$W/cx.txt"
clean_repo
printf 'codepass-zorbtest\n' > "$W/mp.txt"
put src/q.txt "see github.com/rudian/codepass-zorbtest"
expect "entry that extends an allowed module path still matches" 1 "block-list" "$W/mp.txt"
clean_repo
printf '\xEF\xBB\xBFquuxington\nother\n' > "$W/bom.txt"
put src/q.txt "owner quuxington"
expect "list saved with a BOM keeps its first entry" 1 "block-list" "$W/bom.txt"

# e-mail domain escapes
clean_repo
put src/m.ts "mail('pat@quuxexample"".com')"
expect "domain that merely ends in example.com fails" 1 "email" "$LIST"

# ---- round 3: links, list encodings, history failures, odd paths, case
LNK="/Us""ers/jordan/quuxington"
clean_repo
put ok.txt "fine"
ln -s "$LNK" "$REPO/lnk"
expect "untracked symlink: home-path" 1 "lnk:1: home-path" "$LIST"
expect "untracked symlink: block list" 1 "lnk:1: block-list" "$LIST"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "link"
expect "tracked symlink: home-path and block list" 1 "home-path" "$LIST" --tracked
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$REPO" --tracked 2>&1)"
case "$out" in *block-list*) PASS=$((PASS + 1)); echo "ok   tracked symlink also reports block-list" ;; *) FAILN=$((FAILN + 1)); echo "FAIL tracked symlink block-list"; echo "$out" | sed 's/^/     | /' ;; esac
case "$out" in *uuxington*) FAILN=$((FAILN + 1)); echo "FAIL symlink finding echoes the name" ;; *) PASS=$((PASS + 1)); echo "ok   symlink finding never echoes the name" ;; esac
clean_repo
put ok.txt "fine"
ln -s "ok.txt" "$REPO/lnk"
expect "harmless symlink passes" 0 "clean" "$LIST"

# list encodings
printf 'quuxington\n' | iconv -f UTF-8 -t UTF-16 > "$W/u16.txt"
clean_repo; put src/x.ts "owner quuxington"
expect "UTF-16 list is a setup error" 2 "UTF-8" "$W/u16.txt"
printf 're:\nre:   \n' > "$W/emptyre.txt"
expect "list of empty re: lines is a setup error" 2 "usable entries" "$W/emptyre.txt"

# history that cannot be read
H="$W/histrepo"; mkdir -p "$H"; git -C "$H" init -q
git -C "$H" config user.email t@example.com; git -C "$H" config user.name "Test Person"
printf 'fine\n' > "$H/a.txt"; git -C "$H" add a.txt; git -C "$H" commit -qm one
printf 'owner quuxington\n' > "$H/b.txt"; git -C "$H" add b.txt; git -C "$H" commit -qm two
BL="$(git -C "$H" rev-parse HEAD:b.txt)"
rm -f "$H/.git/objects/$(printf '%s' "$BL" | cut -c1-2)/$(printf '%s' "$BL" | cut -c3-)"
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$H" --commits HEAD 2>&1)"; rc=$?
if [ "$rc" = 2 ]; then PASS=$((PASS + 1)); echo "ok   a missing blob makes --commits a setup error"; else FAILN=$((FAILN + 1)); echo "FAIL missing blob (exit $rc)"; echo "$out" | sed 's/^/     | /'; fi

# root commit with log.showRoot=false
R="$W/rootrepo"; mkdir -p "$R"; git -C "$R" init -q
git -C "$R" config user.email t@example.com; git -C "$R" config user.name "Test Person"
printf 'owner quuxington\n' > "$R/a.txt"; git -C "$R" add a.txt; git -C "$R" commit -qm root
printf 'fine\n' > "$R/b.txt"; git -C "$R" add b.txt; git -C "$R" rm -q a.txt; git -C "$R" commit -qm drop
git -C "$R" config log.showRoot false
out="$(CODEPASS_SCRUB_LIST="$LIST" "$CHECK" --root "$R" --commits HEAD 2>&1)"; rc=$?
if [ "$rc" = 1 ]; then PASS=$((PASS + 1)); echo "ok   log.showRoot=false does not hide the root commit"; else FAILN=$((FAILN + 1)); echo "FAIL showRoot (exit $rc)"; echo "$out" | sed 's/^/     | /'; fi

# paths with a line break or a tab
clean_repo
printf 'owner quuxington\n' > "$REPO/$(printf 'notes\nb.txt')"
expect "path with a line break is a setup error" 2 "tab or line break" "$LIST"
clean_repo
printf 'owner quuxington\n' > "$REPO/$(printf 'notes\tb.txt')"
expect "path with a tab is a setup error" 2 "tab or line break" "$LIST"
rm -f "$REPO"/notes*
clean_repo
put ok.txt "fine"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "pre"
BT="$(git -C "$REPO" rev-parse HEAD)"
printf 'x\n' > "$REPO/$(printf 'tab\tname.txt')"
git -C "$REPO" add -A && git -C "$REPO" commit -qm "tabbed"
expect "history: changed path with a tab is a setup error" 2 "tab or line break" "$LIST" --commits "$BT..HEAD"
git -C "$REPO" rm -q -r . >/dev/null 2>&1; git -C "$REPO" commit -qm "cleanup" --allow-empty

# case-insensitive home paths
clean_repo
put src/h.ts 'p = "c:\\us''ers\\jordan\\code"'
expect "lower-case Windows home path" 1 "home-path" "$LIST"
clean_repo
put src/h.ts 'p = "C:\\US''ERS\\jordan"'
expect "upper-case Windows home path" 1 "home-path" "$LIST"
clean_repo
put src/h.ts 'p = "C:\\us''ers\\ALEX\\code"'
expect "allowed home name matches case-insensitively" 0 "clean" "$LIST"

# secret shapes: key blocks and AWS secret values
clean_repo
put k.pem "-----BEGIN RSA PRIV""ATE KEY-----"
expect "private key block" 1 "k.pem:1: secret-shape" "$LIST"
clean_repo
put k.txt "-----BEGIN PRIV""ATE KEY-----"
expect "private key block (PKCS8)" 1 "secret-shape" "$LIST"
clean_repo
put c.ini "aws_secret_access_""key = abcdefghij0123456789abcdefghij0123456789"
expect "assigned AWS secret value" 1 "c.ini:1: secret-shape" "$LIST"
clean_repo
put c.ini "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
expect "AWS secret value marked EXAMPLE passes" 0 "clean" "$LIST"
clean_repo
put c.ini "aws_secret_access_key = <your secret here>"
expect "AWS secret placeholder passes" 0 "clean" "$LIST"
clean_repo
put m.ts "mail('pat@""users.noreply.github.com')"
expect "noreply address is not allowed" 1 "email" "$LIST"

# ---- the repository passes its own check (tracked files; the list holds an invented word and the AI vendor)
printf 'zzinvented-''word\nanthrop''ic\n' > "$W/self.txt"
out="$(CODEPASS_SCRUB_LIST="$W/self.txt" "$CHECK" --root "$HERE/.." --tracked 2>&1)"; rc=$?
if [ "$rc" = 0 ]; then PASS=$((PASS + 1)); echo "ok   the repository passes its own check"; else FAILN=$((FAILN + 1)); echo "FAIL the repository fails its own check"; echo "$out" | sed 's/^/     | /'; fi

echo
echo "scrub-check cases: $PASS passed, $FAILN failed"
[ "$FAILN" = 0 ]
