# Spike S0-04: tool installers (Homebrew.pkg, brew, winget)

Date 2026-11-16. Machine: the owner's Mac (arm64, macOS 27.2.1 build 26B131, Xcode 27.2 installed, admin account, no MDM). Branch `spike/installers` (local only, not pushed). Code is in `spike/installers/` of the worktree `.claude/worktrees/wf_39cfdb02-5e6-1`. Scratch downloads were in the session scratchpad and a throwaway folder under `~/Library/Caches` (deleted afterwards).

## 1. Summary of decisions

1. **Homebrew.pkg flow: use the privileged script (copy, verify the copy, install the copy), embedded in the app and passed to `osascript` as an argument.** It has no swap window. The `open -W` alternative cannot be proven free of one; keep it only as a documented fallback.
2. **The "install into a folder the user owns" path is NOT viable. Do not offer it.** Tested: a prefix of any normal home-folder length makes brew build from source (needs Command Line Tools, which on a clean Mac means a "developer tools" dialog and an admin password anyway), and Homebrew's own installer refuses such a prefix. See section 5. The owner decision "add the own-folder path if S0-04 proves it" resolves to: **not proven, disproven.**
3. **A non-admin Mac account can still use the pkg**, with an administrator's name and password typed once into the macOS prompt; Homebrew's postinstall then makes the console user the owner of `/opt/homebrew`. That matches the design's "ask an administrator" text. Needs a clean-VM run (section 11).
4. **Pin the Team ID `927JGANW46`, not a certificate fingerprint.** The signing certificate changed between releases; the Team ID did not.
5. **The AWS CLI winget package has a user-scope installer** (`%LocalAppData%\Programs\Amazon\AWSCLIV2`). The plan says Windows has "no per-user AWS CLI installer"; the manifest says otherwise. Needs a Windows run.
6. The package names are confirmed. One surprise: `brew install gopls` does not install Go.

## 2. The package: download and verification

Latest release read from the GitHub API (read-only):

```
tag 7.0.13, published 2026-11-09T08:34:38Z
Homebrew.pkg  154292112 bytes  digest sha256:f5dae55ec63149b50f9945fd3431a92e4ebd5614961d88e462563f2a1b7314e1
```

Downloaded with curl in 2.6 s. `ls -l` gives 154292112 (matches); `shasum -a 256` matches the API digest.

```
$ pkgutil --check-signature Homebrew.pkg
   Status: signed by a developer certificate issued by Apple for distribution
   Notarization: trusted by the Apple notary service
   Signed with a trusted timestamp on: 2026-11-09 08:30:26 +0000
   1. Developer ID Installer: Patrick Linnane (927JGANW46)   Expires: 2029-07-02
      SHA256 Fingerprint: 3C 5D AA 66 55 A8 89 1F C1 E0 C0 60 8A EB 6B 3D 06 54 9E 05 A6 7C 1D C9 03 CE A8 D5 21 20 AA C0
   2. Developer ID Certification Authority   3. Apple Root CA
$ spctl -a -vv -t install Homebrew.pkg
Homebrew.pkg: accepted
source=Notarized Developer ID
origin=Developer ID Installer: Patrick Linnane (927JGANW46)
$ xcrun stapler validate Homebrew.pkg
Homebrew.pkg does not have a ticket stapled to it.     (exit 65)
```

- **Team ID is 927JGANW46**, and it belongs to an individual (the Homebrew lead maintainer), not an organisation. An older release (5.1.13, 2026-05-21) is signed by the same Team ID with a different certificate (expiry 2027-02-01 versus 2029-07-02). So pin the Team ID, never the fingerprint, and keep the pinned value in a config the signed update file can change, in case Homebrew moves to an organisation team.
- **Not stapled.** Gatekeeper and `pkgutil` learn about the notarization from Apple online. On a Mac whose proxy blocks Apple's notarization hosts, `spctl` may fail or hang. Unverified (section 11).
- Every release from 5.1.13 to 7.0.13 (40 consecutive, weekly) carries `Homebrew.pkg`; size grew from 133 MB to 154 MB. The app must read size and digest from the API at download time (script argument 3).
- `pkgutil --check-signature` **does** verify the payload checksum, not just the signature on the table of contents (tested in section 3).
- Pkg metadata (`pkgutil --expand`): `hostArchitectures="arm64"`, `allowed-os-versions min=15.0.0`, `install-location=/opt/homebrew`, `auth="root"`, `installKBytes=196433` (192 MB), 4167 files, preinstall and postinstall scripts with 600 s timeouts.
- Postinstall (read in full): runs as root; picks the **console user** (`stat -f %Su /dev/console`; fails with "No valid user for Homebrew installation. Log in before install" if there is none) or a root-owned 0600 `/var/tmp/.homebrew_pkg_user.plist` override; refuses symlinked prefix directories; `chown -R` the prefix to that user; for a non-admin whose group is `staff` removes group/other write; seeds the API cache as the user; writes `/etc/paths.d/homebrew`. It calls `xcode-select --print-path` on purpose ("use system tools without triggering developer tool installation prompts"), so installing the pkg never raises the developer-tools dialog.
- Disk: pkg 147 MiB download + 147 MiB root copy + 192 MB installed = about 490 MB for the Homebrew step. git + awscli and their 25 dependencies are 467 MB installed on this Mac (`du` of the 27 Cellar entries). With bottle downloads the design's "about 1.2 GB" is a sound number.

## 3. The privileged script and how it was tested

File: `spike/installers/homebrew-pkg-install.sh`. Design:

1. The text is a Go string constant in the app. The app calls `osascript -e 'on run argv' -e 'do shell script "/bin/bash -c " & quoted form of (item 1 of argv) & " codepass-pkg " & quoted form of (item 2 of argv) & ... with prompt "..." with administrator privileges' -e 'end run' -- "$SCRIPT" "$PKG" "$TEAM" "$SIZE" "$MODE"`. The script is never read from a file on disk, so there is no script file to swap.
2. As root it stops if `/opt/homebrew` exists (exit 20).
3. `mktemp -d /var/root/.codepass-pkg.XXXXXXXX` (atomic, random name, mode 0700, inside root's home, which is `drwxr-x--- root wheel` here).
4. It opens the user's file **once** (`exec 3<`), copies from that descriptor into the root folder, then `chown root:wheel` and `chmod 0400`.
5. Everything after reads only the copy: size equals the expected size; `pkgutil --check-signature` says "signed by a developer certificate issued by Apple for distribution"; the leaf line `1. Developer ID Installer: ... (TEAMID)` ends in the pinned Team ID; "Notarization: trusted by the Apple notary service"; `spctl -a -vv -t install` agrees (`origin=Developer ID Installer: ... (TEAMID)`).
6. `installer -pkg <the copy> -target /`, then check `/opt/homebrew/bin/brew` exists; a `trap` removes the folder.

Exit codes: 0 ok, 10 copy failed, 11 disk full, 12 not signed or damaged, 13 wrong Team ID, 14 not notarized or Gatekeeper rejects, 15 size mismatch, 20 prefix exists, 21 not root, 30 installer failed, 31 brew missing afterwards.

Tested unprivileged (`CODEPASS_TEST_WORKDIR` replaces `/var/root` by a temp dir; `bash spike/installers/test-verify.sh Homebrew.pkg`):

```
good pkg                   exit=0   verified: size=154292112 team=927JGANW46 sha256=f5dae55e...
wrong expected size        exit=15  size 154292112, expected 154292111
wrong expected Team ID     exit=13  Team ID is not ABCDE12345: 1. Developer ID Installer: Patrick Linnane (927JGANW46)
1 byte flipped in payload  exit=12  signature: Status: package is invalid (checksum did not verify)
1 byte flipped near start  exit=12  signature: Could not open package
re-flattened (unsigned)    exit=12  signature: Status: no signature
truncated download         exit=12  signature: Status: package is invalid (checksum did not verify)
proxy HTML error page      exit=12  signature: Could not open package
```

A truncated file or a proxy error page also gives exit 12, so the app must check size (known from the API) and HTTP status before the privileged step and say "download damaged, try again" rather than "signature wrong".

osascript wrapper (`spike/installers/test-osascript.sh`, run without the administrator clause so no dialog appears):

- A path with spaces, quotes, `$(touch ...)`, backticks and `;` arrived intact as one argument and created no files.
- Exit 0: output is returned with newlines turned into carriage returns (test showed `argc=3arg1=[...]`). Do not parse stdout by lines.
- Non-zero exit: osascript exits 1 and prints `<line>:<col>: execution error: <the script's stderr> (<exit code>)`. **Stdout is lost on failure**, so the script writes its reasons to stderr (it does). The exit code is the number in the last parentheses (`\((-?\d+)\)\s*$`).
- User cancelling the password window: `execution error: User canceled. (-128)` (reproduced with `osascript -e 'error "User canceled." number -128'`, not with a real dialog). Security framework codes from `Authorization.h` in the SDK: -60005 denied (wrong password), -60006 cancelled, -60007 no interaction possible.
- The AppleScript with `with administrator privileges` compiles (`osacompile` OK). It was **not run**: that would pop a password window on the owner's daily Mac and install software.
- **No streaming.** `do shell script` returns only when the script ends. Verify and install cannot be shown as separate live steps. Recommendation: run a quick unprivileged `pkgutil --check-signature` plus Team ID check on the downloaded file before the password prompt (this is the visible "verifying signature" step and fails before any password is asked), then repeat the full check on the root copy.

## 4. Swap-window review (the task's review question)

**Privileged script: no window.**
- A swap of the download before the copy only makes verification of the copy fail. The copy is made from one open file descriptor.
- The copy lives in a 0700 root folder inside `/var/root`. A same-user process cannot list, open or replace it, and `installer` reads that same file.
- The script text cannot be swapped: it is an argument of `osascript`. If it were kept in a user-writable file (for example beside an app in `~/Applications`), that file would be a root-code-execution hole. Rule: never `do shell script` a file path; embed the text.
- Path arguments go through `quoted form of`; the injection test passed.
- An existing user-owned `/opt/homebrew` (old install, planted symlink inside it) would make root `installer` write into a user-controlled tree; the script refuses (exit 20). `/opt` is root-owned (`drwxr-xr-x root wheel`), so a user cannot pre-create `/opt/homebrew`.
- `/var/tmp/.homebrew_pkg_user.plist` (install-user override) is honoured only when root-owned, 0600, regular, no ACL (postinstall source); `/var/tmp` is sticky. It did not exist here.
- Residual, not closable by the app: the installed prefix is owned by the user, so any later same-user process can edit brew. Homebrew says it "provides no security guarantees for installations where users with write permissions to the Homebrew prefix are considered untrusted" (docs.brew.sh/Installation). The root copy protects the one moment root runs package scripts.
- Residual: the password prompt text comes from the `with prompt` string and any process can show a look-alike dialog. Not specific to this design.

**`open -W` alternative: a window cannot be excluded.** `open -W Homebrew.pkg` hands the user-owned file to Installer.app, which runs as the user. Apple does not document whether the package is copied to a protected place before its scripts run, and a file owned by the user can be replaced by any process of that user (`chflags uchg` can be undone by the owner). Whether the signature is re-checked right before install is not documented. Other costs: the user sees the full wizard (the pkg ships WELCOME, LICENSE and CONCLUSION pages); the exit status of `open -W` only says the app closed (cancel and success look the same, so success must be checked by looking for `/opt/homebrew/bin/brew`); if Installer.app is already open for another package `-W` waits on the app, not the document (use `-n`). Not run here (it opens a GUI on the owner's Mac). Verdict: fallback only, for a Mac where the `osascript` prompt is unavailable.

## 5. Homebrew into a folder the user owns (non-admin accounts)

Sources read:
- docs.brew.sh/Installation (`docs/Installation.md`, last reviewed 2026-09-21): "Homebrew can be installed for a non-admin account. The shell installer needs a writable prefix; arrange initial provisioning if needed, or use `--path` to select a compatible custom prefix. The macOS `.pkg` installer can provision the default prefix for the account selected by `HOMEBREW_PKG_USER`."
- docs.brew.sh/Support-Tiers, "Custom prefixes": a custom prefix "must be equal to or shorter in bytes than the platform's default" (`/opt/homebrew` is 13 bytes; `/opt/brew` fits). Other prefixes are Tier 3.
- `Homebrew/install` `install.sh` at HEAD (1243 lines): usage `-p, --path=PATH  Set the installation prefix (no longer than the default)`; it aborts with `The Homebrew prefix "..." is longer than the default prefix "/opt/homebrew".`; and `Insufficient permissions to install Homebrew to ...` when the parent is not writable and there is no sudo.
- docs.brew.sh/Homebrew-for-Mac-Admins: non-admin installs use the account's primary group; for `staff` group and other write permissions are removed.

**Test on this Mac.** I used the pkg's own payload as a prefix at `~/Library/Caches/cp-spike-s004/hb` (43 bytes; a real `/Users/<name>/homebrew` is 20 to 30 bytes, also over 13), with `env -i`, `HOMEBREW_NO_ANALYTICS=1`, `HOMEBREW_NO_AUTO_UPDATE=1`. (A first attempt under `/private/tmp` was refused: "Your HOMEBREW_PREFIX is in the Homebrew temporary directory".)
- `brew config` works (`HOMEBREW_PREFIX: ~/Library/Caches/cp-spike-s004/hb`). The first run downloaded portable Ruby from `ghcr.io`.
- `brew install --dry-run git awscli` prints: "Warning: Your Homebrew's prefix is not /opt/homebrew. Some of Homebrew's bottles (binary packages) cannot be used with this prefix." and "This is a Tier 3 configuration".
- `brew install git awscli`: build logs appeared for `gettext`, `git`, `openssl@4`, `pkgconf`, `ca-certificates` and `python@3.15` (`logs/<formula>/01.configure.log`, `02.make.log`, with clang calls). Other dependencies (aws-c-*, pcre2, ...) were poured as bottles. After about 15 minutes git 2.56.1 had been compiled; the awscli step (python from source) had not finished when I stopped it. Disk used: 851 MB.
- On a clean Mac the compile step needs Command Line Tools (Installation docs footnote 3: "Xcode or the CLT is required to build formulae from source"), so the first compiler call raises the developer-tools dialog, and installing CLT needs an administrator password. **Not run on a clean machine** (this Mac has Xcode and CLT); the conclusion rests on the build logs plus the documentation.
- Control: with the default prefix all 27 formulae of `git` + `awscli` have `arm64_sequoia`, `arm64_tahoe` and `arm64_golden_gate` bottles in the formulae API ("missing bottles: none", script `tools/closure.py` run in the session). `uses_from_macos` entries (curl, expat, bzip2, libxml2, ncurses, libedit, libffi, mandoc; perl build-only) are macOS system pieces. So with `/opt/homebrew` nothing is compiled.

**Decision.** An own-folder install leaves a non-admin user with a Tier 3, compile-from-source Homebrew whose first build triggers the CLT installer, which itself needs an admin password. It does not do what the owner wanted. Remove "if the spike proves it" from the plan and state that it is not offered.

**What works for a non-admin account:** the pkg run with an administrator's credentials. `do shell script ... with administrator privileges` asks a standard user for an administrator's name and password (needs verification on a VM with a standard user; I only have an admin account). Postinstall then makes the console user the owner and, for a `staff`-group standard user, removes group/other write. Afterwards `brew install git awscli` needs no sudo. The app should detect "not an administrator" before the prompt with `id -Gn` or `dseditgroup -o checkmember -m "$USER" admin` (prints "yes <user> is a member of admin" here), and tell the user an administrator must type their name and password.

## 6. install.sh behaviour (macOS < 15, Intel) and curl-bash

From `install.sh` HEAD:
- **Intel:** `abort "Homebrew on macOS is only supported on Apple Silicon processors!"`.
- **macOS < 15.0:** `MACOS_OLDEST_SUPPORTED="15.0"`, `MACOS_NEWEST_UNSUPPORTED="28.0"`. For an older version it does not abort: it prints "You are using macOS 14.x. We (and Apple) do not provide support for this old version. This installation may not succeed. After installation, you will encounter build failures with some formulae." and continues.
- **The pkg:** `Distribution` has `hostArchitectures="arm64"` and `allowed-os-versions min="15.0.0"`, so Installer refuses on Intel and macOS < 15 (exact error text not seen; needs such a Mac). Moot for Codepass: the app is arm64-only with `LSMinimumSystemVersion` 15.0, so the gate never runs there; those users stay on 0.0.1 (owner default 5).
- `install.sh` on a Mac without CLT and with sudo: `should_install_command_line_tools` is true when `/Library/Developer/CommandLineTools/usr/bin/git` is missing and sudo works; it then runs `softwareupdate -i` and falls back to `xcode-select --install` ("expect a GUI popup"). So `curl | bash` **does** raise the developer-tools dialog and needs sudo in a terminal; the pkg route does not. This confirms the plan's choice of the pkg.
- **brew itself never raises the dialog:** `Library/Homebrew/utils/git.sh` `setup_git`: "/usr/bin/git opens an installation prompt without developer tools" and it uses `<xcode-select path>/usr/bin/git` only if the path is set and is not `/`. `cmd/update.sh` calls git through `shims/shared/git` which does the same; with no usable git, `brew update` installs brewed git (`install_tool git`).
- **The app can raise it.** On a Mac without CLT, running `git`, `/usr/bin/git`, `make`, `clang`, `python3` or `xcrun` from the app (for example `git --version` in detection) starts the macOS "install developer tools" dialog. `internal/envcheck` must find git by file path (`/opt/homebrew/bin/git`, `/usr/local/bin/git`) and run `/usr/bin/git` only after `xcode-select -p` prints a path other than `/`. Not reproduced here (CLT present); stated from brew's own comments.

## 7. brew behind a proxy

- Brew's Manpage ("Environment") documents `all_proxy`, `ftp_proxy`, `http_proxy`, `https_proxy`, `no_proxy` ("Use this ... proxy for curl(1), git(1) and svn(1) when downloading"), and `HOMEBREW_CURLRC`, `HOMEBREW_BREW_GIT_REMOTE`, `HOMEBREW_CORE_GIT_REMOTE`, `HOMEBREW_API_DOMAIN`, `HOMEBREW_BOTTLE_DOMAIN` for mirrors.
- **Tested** (`spike/installers/brew-proxy.sh`): with a fresh empty cache, `HTTPS_PROXY=http://127.0.0.1:1` and, separately, `https_proxy=http://127.0.0.1:1` both make brew fail at "Downloading Homebrew API data" with `Error: Failure while executing; ... shims/shared/curl ... exited with 7.`; without the variable the same command downloads. Both spellings are honoured; the app should set both cases.
- A Finder-launched app and its children do not see shell variables. The app must read `scutil --proxy` (this Mac prints only an exception list: no proxy) and set the variables per child. PAC files (`ProxyAutoConfigEnable`) cannot be used by curl; the app must evaluate them itself or show a "set the proxy by hand" message. Authenticated proxies work as `http://user:pass@host:port`, but then the password sits in the child environment: never log it.
- Hosts that must be reachable: `github.com` and `objects.githubusercontent.com` (the pkg), `formulae.brew.sh` (API), `ghcr.io` (portable Ruby and bottles; observed), `pkg-containers.githubusercontent.com` (bottle blob redirect; from Homebrew docs, not observed), and for the AI casks `downloads.claude.ai` and the openai/codex release on github.com.
- Recorded failure signals (`spike/installers/netfail.py`, `gonetfail.go`):

```
curl dead proxy             exit 7   Failed to connect to 127.0.0.1 port 1 ... Couldn't connect to server
curl proxy answers 407      exit 22  The requested URL returned error: 407          (with --fail)
curl proxy hangs, 3 s max   exit 28  Connection timed out after 3006 milliseconds
curl DNS failure            exit 6   Could not resolve host
curl self-signed TLS (MITM) exit 60  SSL certificate problem: self signed certificate
curl HTTP 404 (--fail -L)   exit 56  The requested URL returned error: 404
curl captive-portal HTML    exit 0   content-type text/html size 42     (no error: check content type and size)
Go dead proxy   Get "...": proxyconnect tcp: dial tcp 127.0.0.1:1: connect: connection refused
Go proxy 407    Get "...": Proxy Authentication Required
Go proxy hangs  Get "...": context deadline exceeded
Go DNS          Get "...": dial tcp: lookup ...: no such host
Go MITM TLS     Get "...": tls: failed to verify certificate: x509: certificate signed by unknown authority
brew            Error: Failure while executing; `.../shims/shared/curl ...` exited with 7.
```

## 8. Brew formulae and casks (read-only checks, `brew info --json=v2` on this Mac plus formulae.brew.sh)

| Name | Kind | Version 2026-11-16 | Notes |
|---|---|---|---|
| git | formula | 2.56.1 | bottles arm64_sequoia, arm64_tahoe, arm64_golden_gate; deps pcre2, gettext |
| awscli | formula | 2.40.3 | same bottle tags; deps 9 aws-c libraries + python@3.15; 27 formulae with git |
| opencode | formula | 2.3.0 | same bottle tags |
| gopls | formula | 0.24.0 | **no runtime deps; Go is only a build dependency.** `brew install gopls` does not give a Go toolchain, and gopls needs `go` on PATH |
| typescript-language-server | formula | 6.1.0 | bottle tag `all`; deps node and typescript (31 formulae in total) |
| claude-code | **cask** | 2.1.311 | `downloads.claude.ai/claude-code-releases/2.1.311/darwin-arm64/claude`; artifacts `binary`, `zap` only, so no sudo |
| codex | **cask** | 0.169.0 | openai/codex release tarball; `binary`, completions, `zap`; no sudo |
| go, node | formulae | 1.27.4 / 26.13.1 | toolchains for gopls / node tools; same bottle tags |
| ripgrep | formula | 15.2.0 | not used by the plan; bottle exists |

`brew info` on this Mac lists only the bottle of its own tag (`arm64_golden_gate`); the other tags were read from `https://formulae.brew.sh/api/formula/<name>.json`. The plan's "five brew package names exist" is confirmed (two are casks, as the plan writes).

## 9. winget IDs (microsoft/winget-pkgs master, GitHub API, read-only)

| ID | Exists | Latest in repo | Installer | Scope in manifest |
|---|---|---|---|---|
| `Git.Git` | yes | 2.56.0.4 | inno, `ElevationRequirement: elevatesSelf`, silent `/SP- /VERYSILENT /SUPPRESSMSGBOXES /NORESTART` | x64 and arm64, **user and machine** |
| `Amazon.AWSCLI` | yes | 2.40.2 | wix MSI `https://awscli.amazonaws.com/AWSCLIV2-2.40.2.msi`, x64 only | **machine** (`%ProgramFiles%\Amazon\AWSCLIV2`) **and user** (`%LocalAppData%\Programs\Amazon\AWSCLIV2`) |
| `GoLang.Go` | yes | 1.27.4 | wix MSI | machine |
| `OpenJS.NodeJS.LTS` | yes | 26.13.1 | wix MSI, `elevatesSelf` | machine |
| `OpenJS.NodeJS` | yes | 26.12.1 | wix MSI | machine |
| `Anthropic.ClaudeCode` | yes | 2.1.318 | portable exe from `downloads.claude.ai` | n/a |
| `OpenAI.Codex` | yes | 0.168.0 | zip with exe | n/a |
| `BurntSushi.ripgrep.MSVC` | yes | 15.2.0 | zip | n/a |
| OpenCode | **not found** under `s/SST`, `s/sst`, `a/anomalyco`, `o/opencode-ai` | | | not on winget by those names; the plan routes it elsewhere |

Consequences: (a) `winget install --id Git.Git -e` and `--id Amazon.AWSCLI -e` are correct IDs. (b) Both manifests offer `--scope user`. The plan's claim that the AWS CLI installer "is machine-wide and needs an administrator" and that "Windows has no per-user AWS CLI installer" conflicts with the manifest's user-scope entry. Unverified: whether the MSI really installs per user without UAC and puts `aws` on the user PATH. That is a Windows run (section 11); if it works Windows gets a no-admin path. (c) Only an x64 AWS CLI exists (emulated on Windows on Arm). (d) Non-interactive flags: `--accept-package-agreements --accept-source-agreements` (documented, not run).

Winget return codes (microsoft/winget-cli `doc/windows/package-manager/winget/returnCodes.md`, read, not run): 0x8A150019 needs administrator; 0x8A150105 disk full; 0x8A150107 no network; 0x8A150008 download failed; 0x8A150011 installer hash mismatch; 0x8A15003A blocked by Group Policy; 0x8A15010F organisation policy prevents install; 0x8A15001B / 0x8A15001C Store client / app blocked by policy; 0x8A15010C cancelled by user; 0x8A150109 / 0x8A15010A reboot needed; 0x8A15010D already installed; 0x8A15002B no applicable update; 0x8A150049 MSI install failed. All are in `classify.Winget`.

## 10. Failure causes from design 5.1 and how each is recognised

| Cause | macOS signal | Windows signal | Verified |
|---|---|---|---|
| Not an administrator | Pre-flight (not output): `id -Gn` lacks `admin`. Cancelled prompt: osascript `(-128)`. Wrong or non-admin credentials: `(-60005)` | winget `0x8A150019`; UAC declined `0x8A15010C` / `0x8A150077` | pre-flight command and -128 text run here; the rest from SDK header and docs |
| Network or proxy | curl 6/7/28/35/52/55/56/60; 22 with 407 means proxy auth; brew text `exited with N`; Go errors in section 7; curl exit 0 with `text/html` is a captive portal | `0x8A150008`, `0x8A150107`, `0x8A150086`, `0x8A15002E` | macOS and Go reproduced; winget documented |
| Blocked by device management | Not reproduced (no MDM). Pre-flight hint: `profiles status -type enrollment` (here "MDM enrollment: No"). Installer words "restricted", "blocked", "not allowed" are a guess | `0x8A15003A`, `0x8A15010F`, `0x8A15001B`, `0x8A15001C` | documented only |
| Disk full | script exit 11 (`cat` prints "No space left on device"); `installer` or brew text; pre-flight `df` against 1.2 GB | `0x8A150105` | code path exists, real text not reproduced (would need filling the daily Mac's disk) |
| Package signature wrong | script exit 12 / 13 / 14; with the size known a truncated file gives 15 | `0x8A150011` | reproduced: tampered, unsigned, wrong team, truncated, HTML page |
| winget unavailable | n/a | start failure `exec: "winget": executable file not found in %PATH%` | documented; needs Windows |
| Prefix already exists (new) | script exit 20 | n/a | by construction |
| User cancelled | `(-128)` | `0x8A15010C` | -128 text reproduced |
| brew formula failed (new) | brew exit non-zero with `Error:` lines; classifier falls back to "show the log" | n/a | partly |

`spike/installers/classify` (Go; `go vet`, `go test` and `GOOS=windows go build` pass) holds these rules and a table test built from the recorded strings. It is the starting point for F1-12.

## 11. Needs manual verification (clean VM / Windows / a person)

None of these can run on the owner's daily Mac without admin actions or without a clean machine.

1. **macOS 15 and macOS 26 VMs without Command Line Tools** (Apple silicon host, Tart or UTM, snapshot first). (a) `xcode-select -p` errors and `/Library/Developer/CommandLineTools` is absent. (b) `sudo bash homebrew-pkg-install.sh /path/Homebrew.pkg 927JGANW46 <size> verify` expects exit 0; then `install`, expects exit 0 and `/opt/homebrew/bin/brew --version` printing the release. (c) As the VM user: `HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_AUTO_UPDATE=1 /opt/homebrew/bin/brew install git awscli`, expecting only "Pouring" lines, no compiler and **no "developer tools" dialog**. (d) `/opt/homebrew/bin/git --version`, `/opt/homebrew/bin/aws --version`. (e) `brew update`: expect no dialog. (f) Confirm that `/usr/bin/git --version` does raise the dialog (the reason envcheck must not run it). (g) Run the real `osascript` call once, enter the password, confirm the exit-code transport and that stdout is lost on failure. This is the S0-04 acceptance test; it has not been done.
2. **Standard (non-admin) macOS account:** run the same osascript call; expect a name-and-password dialog; type an admin's credentials; expect `/opt/homebrew` owned by the standard user and `brew install git awscli` to work with no sudo. Cancel once and give a wrong password once to see the real texts (expected -128 and -60005).
3. **`open -W` flow:** `open -W -n -a Installer /path/Homebrew.pkg`; note the wizard pages, the exit status after cancel versus success, and whether Installer shows the certificate.
4. **Notarization blocked or offline:** cut the VM's network after the download and run `spctl -a -vv -t install Homebrew.pkg` and `pkgutil --check-signature` on the unstapled pkg; decide whether `spctl` may be optional.
5. **Managed Mac:** a Mac with an MDM profile that restricts installers, for the real wording of the "blocked" row.
6. **Disk full:** a VM with a small disk, for the real `cat` and `installer` text.
7. **Windows 10 1809 (without App Installer) and Windows 11**, standard user and administrator: `winget --version` on 1809 (expect "not recognized"), then install App Installer from the Store and record whether the Store can be blocked; `winget install --id Git.Git -e --accept-package-agreements --accept-source-agreements --silent` and the `--scope user` variant, noting UAC (manifest says `elevatesSelf`); `winget install --id Amazon.AWSCLI -e --scope user --silent` as a **standard user** (does it install without UAC into `%LocalAppData%\Programs\Amazon\AWSCLIV2`, and does a new process find `aws`?), then machine scope as a standard user to record the real exit code; record exit codes and last lines for no network, a bad proxy, disk full, Group Policy block.
8. **Proxy:** a network with a real authenticated proxy and one with a PAC file, to confirm brew works with the variables the app sets and to see what a PAC-only setup looks like.

## 12. What the plan must change

**product-plan.md**
1. Section 4.1, line 193 (the pkg flow): replace "Alternative kept for the spike: hand the package to Installer.app with `open -W`, which verifies it itself" with: "The privileged script is embedded in the app and passed to `osascript` as an argument (never a file). The app first checks size, Team ID and signature unprivileged; the script then copies the file from one open descriptor into a `mktemp -d` folder under `/var/root`, verifies the copy, runs `installer` on it, and refuses when `/opt/homebrew` exists. `open -W` is a fallback with a swap window that cannot be excluded (S0-04)." Add Team ID `927JGANW46`, "pin the Team ID, not the certificate" (the certificate changed between 5.1.13 and 7.0.13), and that size and digest come from the GitHub API at download time.
2. Line 196 and owner decision 1 text: delete "the spike tries it, and if it works the app offers it" and the optional second button. Write: "Own-folder Homebrew is not offered: Homebrew's installer refuses a prefix longer than 13 bytes and a longer prefix compiles from source (Tier 3), which needs Command Line Tools. A standard account uses the pkg with an administrator's name and password." Update the risk row (line 428) to the verified design.
3. Line 194: `brew install git awscli` is correct; add `HOMEBREW_NO_AUTO_UPDATE=1` for the setup run and the proxy variables in both upper and lower case.
4. Line 195 and every "no per-user AWS CLI installer" sentence (plan 4.1, tasks, design 5.1, decision 1 facts): change to "the `Amazon.AWSCLI` winget manifest lists a user-scope installer; test `--scope user` on Windows (S0-04) and use it for non-admin accounts if it works." Keep the "machine-wide" wording only after the Windows run confirms it.
5. Line 199 and line 319: gopls does not pull in Go; add `brew install go` (asked, with size) before `gopls`; typescript-language-server pulls in node by itself (31 formulae). `claude-code` and `codex` are casks and need no sudo.
6. Section 4.1 detection: on macOS never run `git`, `/usr/bin/git`, `make`, `clang`, `python3` or `xcrun` unless `xcode-select -p` prints a path other than `/`; find git by path.
7. Add a pre-flight list for the setup screen: admin group (`id -Gn`), enrolment (`profiles status -type enrollment`), free space (about 1.2 GB), `scutil --proxy`, and the host list from section 7.
8. Setup flow wording: "verify signature" is shown before the password prompt (unprivileged check) and again inside the script; after the prompt the row shows one indeterminate state, because `do shell script` cannot stream.

**tasks.md**
1. S0-04 Accept: the "five brew package names and the winget IDs are confirmed" clause is met (sections 8 and 9). The VM and Windows clauses stay open; attach the section 11 checklist as the clean-machine follow-up for the owner or a VM-capable machine. F1-12 need not wait for it except for the exit-code table, which is now fixed.
2. S0-04 Build: delete "Homebrew into a folder the user owns" (disproven); keep proxy and winget on Windows 10 1809 and 11; add "AWS CLI `--scope user` as a standard user".
3. Owner decisions, decision 1: add "S0-04 result: the own-folder Homebrew path is not available; a standard account installs through the pkg with an administrator's credentials; on Windows a per-user AWS CLI install may exist (to be tested)."
4. F1-12 (`internal/toolinstall`): start from `spike/installers/homebrew-pkg-install.sh` and `spike/installers/classify`; exit codes 0/10/11/12/13/14/15/20/21/30/31; the privileged step cannot stream; size and digest from the GitHub API; Team ID from a config.
5. F1-12 Tests: add the cases of `test-verify.sh` (good, size, team, flipped byte, unsigned, truncated, HTML page) as unit cases that run without root through the test work-dir switch; keep "reviewed and tested on the VM".
6. F1-12 Review: add "the script text is never read from disk; every argument goes through `quoted form of`; an existing `/opt/homebrew` stops the script".
7. F1-15 and design 5.1: keep the "Not an administrator" row text; remove the conditional "Install into my own folder" button. Add rows for "Prefix exists" ("Homebrew is partly installed in /opt/homebrew: remove it or finish it yourself") and "Download damaged" (size mismatch, retry). The privileged step shows the final error line only, not a live tail.
8. Standing review checklist: add item 9, "the app never runs `/usr/bin/git` or another developer-tool stub before `xcode-select -p` succeeds".
