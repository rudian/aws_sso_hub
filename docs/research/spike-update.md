# S0-02 · Signed-manifest update: findings

Branch: `spike/update` (local only, commit 521bd9e, worktree `.claude/worktrees/wf_e2601f60-0f7-2`). Nothing pushed.
Run on 2026-11-16, macOS arm64 (the owner's Mac), Go 1.27.4, Wails v3.1.0 `pkg/updater` from the module cache.

## What was built

| Path | What |
|---|---|
| `internal/update/manifest.go` | Manifest struct, ed25519 verify against two pinned keys (current, next), domain-separated signature, unknown fields refused, well-formedness (64-hex sha256, plain file name, size, semver), the version rules of plan 4.14 item 3 |
| `internal/update/provider.go` | `updater.Provider` that wraps the Wails GitHub provider. The inner provider is used only to find the newest release and to stream an asset; version, digest, file, size and notes come only from the verified manifest |
| `internal/update/gate_darwin.go`, `gate_other.go` | Bundle gate run on the staged copy before the helper starts: bundle id, `CFBundleShortVersionString` equals manifest version, `codesign --verify --deep --strict -R=<requirement>`, optional `spctl -a -t exec`. Non-macOS refuses everything (Windows is not implemented) |
| `internal/update/controller.go` | `Prepare` (check, download, gate; on any error removes the staging dir) and `Staged.Restart` (re-runs the gate, then Wails `Restart`). Raw Wails `Restart` is never exposed |
| `internal/update/highest.go` | stored "highest version ever run" file |
| `internal/update/update_test.go` | unit tests: signature, rotation, tamper, digest/file-name/semver hardening, every version rule, highest never lowers, AST test that `updater.HandleHelperMode()` is the first statement of `main()` in both `main.go` and the test app |
| `spike/update/tool` | keygen, manifest, sign, publish, and a fake GitHub releases API (`/repos/o/r/releases`, `/latest`, asset URL that 302-redirects to a "CDN" path, hostile release body, a `DOWN` switch for a 500) on 127.0.0.1, random port |
| `spike/update/testapp` | stand-in app in a real `.app`; `main()` starts with `updater.HandleHelperMode()`; `update` mode runs the gated flow; `ungated` mode is the control that skips the gate |
| `spike/update/mkapp.sh`, `run.sh` | build, sign and zip test bundles; 17 scenarios, 118 assertions |
| `main.go` | one-line change: `updater.HandleHelperMode()` is now the first statement (plus the import) |

Environment: throwaway ed25519 keys in a temp dir; everything under the session scratchpad (`$SP`); the installed test app is `$SP/work/install/Codepass.app`; never `/Applications`. Test bundles were signed with the owner's Developer ID Application identity from the login keychain (`--timestamp=none`, so nothing contacted Apple; no keychain prompt appeared). They were never notarized and never left the machine. `go.mod` gained `golang.org/x/mod` as a direct dependency (already in the graph through Wails).

## Results

`DEVID_HASH=... SP=... spike/update/run.sh` prints `RESULT: 118 passed, 0 failed`. `go test ./internal/update/` passes. `go build .` passes for darwin and `GOOS=windows GOARCH=amd64` (with a placeholder `frontend/dist`, removed afterwards). `GOOS=windows go build ./internal/update/` passes.

Every refusal row also asserts: exit code 3, the reason text, the installed `.app` byte-identical (sha256 of every file, size, mtime, inode, `codesign --verify` result), no `wails-update-*` or `.bak` beside it, no relaunch, version and stored highest unchanged.

| # | Case | Result (reason printed) |
|---|---|---|
| G1 | rc.1 to rc.2, manifest signed by current key | swapped, relaunched as rc.2 (marker file), installed bundle satisfies the requirement, no leftovers, highest = rc.2, notes shown are the manifest's, not the API body. Helper log: `backing up ... swap succeeded on attempt 1 ... helper done` |
| G2 | same, signed by the **next** key | swapped (rotation works) |
| **R1** | **old signed archive + its genuine old manifest re-published under new tag `v1.0.0-rc.3`** (installed rc.2) | **refused**: `manifest version 1.0.0-rc.1 does not equal release tag v1.0.0-rc.3 (re-published archive?)` |
| R1b | same old release under its own old tag | Wails' inner semver filter says up to date (exit 4); untouched |
| R1c | correct rc.2 but this machine already ran rc.5 | refused: `below the highest installed 1.0.0-rc.5` |
| **R2** | **valid manifest over an ad-hoc signed bundle** | **refused**: `codesign requirement failed: ... code failed to satisfy specified code requirement(s)` |
| R2b | valid manifest over a bundle with only the Go linker signature | refused: `code has no resources but signature indicates they must be present` |
| **R3a** | **manifest edited after signing** | **refused**: `signature does not match any pinned key` |
| R3b | signed by an unpinned key | refused, same reason |
| R3c / R3d | `.sig` removed / no manifest at all (what the stock GitHub provider accepts) | refused: `no signed manifest for darwin/arm64` |
| R3e | garbage signature | refused: `malformed signature` |
| R4a | archive byte flipped after signing (same size) | refused: `updater: digest mismatch` (Wails streaming digest) |
| R4b | archive padded | refused: `download exceeds signed size` |
| R5 | Developer ID signed, honest manifest, bundle id `com.evil.app` | refused: `bundle identifier ... expected ...` |
| R6 | manifest says rc.2, bundle inside says rc.1 | refused: `bundle version "1.0.0-rc.1" does not equal manifest version` |
| R7 | sealed resource changed after signing, honest manifest | refused: `a sealed resource is missing or invalid` |
| R8 | API answers 500 | refused: `github: api 500` |
| R9 | install folder not writable | refused before any download: `staging dir: mkdir ...: permission denied` |
| G3 | `spctl -a -t exec` switched on, un-notarized Developer ID bundle | refused: `gatekeeper assessment failed` (shows the check bites; a notarized bundle passing is **unverified**) |
| C1 | CONTROL: ad-hoc archive, Wails updater with no bundle gate, ad-hoc original | **swapped and relaunched**: Wails' own macOS check lets it through |
| C2 | CONTROL: Developer ID original, ad-hoc update, no bundle gate | **swapped and relaunched**: Wails alone downgrades Developer ID to ad-hoc |

Accept criteria: re-published old archive refused (R1), ad-hoc refused (R2), tampered manifest refused (R3), good update swaps and relaunches (G1, G2), `HandleHelperMode` first (AST test plus the `main.go` edit). All met on macOS arm64 locally. Review focus ("every refusal path leaves the installed app untouched"): R1 to R9 and G3 assert it by hash and inode.

## What did not work or surprised me

1. **A plain `ditto -c -k --keepParent` zip breaks the signature after extraction.** Every file here carries the `com.apple.provenance` xattr, so ditto writes AppleDouble `._*` entries (`rc2.app/Contents/._MacOS`, `.__CodeSignature`, ...). Wails' Go extractor writes them as real files and the bundle then fails `codesign --verify` ("a sealed resource is missing or invalid"). The gate refused it, which is how I found it. Fix: `ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl App.app out.zip` (verified: clean 9-entry zip, G1 passes). The first real release script will hit this.
2. **The plan's requirement string is too loose.** `anchor apple generic and identifier "..." and certificate leaf[subject.OU] = "<TeamID>"` is satisfied by an **Apple Development** certificate of the same team (tested: rc=0). Adding the two Developer ID OIDs closes that; tested both ways (real Developer ID bundle passes, development-signed bundle fails):
   `anchor apple generic and identifier "com.rudian.awsssohub" and certificate leaf[subject.OU] = "XC34P8TNZ6" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists`
   The codesign argument form is `-R=<text>` (what plan 4.14 writes). `-R="=<text>"` is a syntax error.
3. **Wails' own signature check is a no-op for our threat model** (C1, C2). `verifySignature` enforces a verdict only if the *original* also passes, and a valid ad-hoc seal passes, so ad-hoc replaces Developer ID. It also runs **after** the swap (rollback via `.bak`), so even its refusals touch the installed app first. All our checks therefore run on the staged copy before `Restart`; Wails' check stays as a second line only.
4. **`Config.PublicKey` cannot carry two keys** (it is one key and switches on "signature mandatory"). I left it empty and put the signed manifest's sha256 in `Verification.Digest`; Wails compares it with the streaming hash of the download and fails closed (R4a). `Prepare` refuses to run if `PublicKey` is set or the release lacks a 32-byte digest. An empty digest would make Wails skip the comparison (`len(Digest) > 0`), so the manifest validator insists on 64 hex.
5. **After a gate refusal the Wails state machine stays in `StateReady`**, and its periodic loop skips `StateReady` forever. There is no public discard. `Prepare` builds a fresh `updater.Updater` per attempt and the app schedules its own checks (`CheckInterval` unused).
6. Wails' inner semver filter hides some refusals as "up to date" (R1b). Harmless; log it.
7. `x/mod/semver` orders `rc.10` above `rc.2` but `rc10` below `rc2` (unit test logs the hazard). Tags must be `v1.0.0-rc.N` with a dot.

## Read from the Wails source, not run

- **The swap is not atomic for a `.app`.** `replaceTarget` does `os.RemoveAll(target)` then `rename(new, target)`, after copying the old app to `Codepass.app.bak`. If the helper dies between those two steps there is no app, and nothing recovers `.bak` (no process is left to run). The window is milliseconds on one volume. The staging dir is created **beside** the target, so the parent folder must be writable (R9: the failure is clean). The `.bak` copy briefly doubles disk use.
- Helper exit codes 10 to 20 are documented in `runHelperSwap`; only 14, 16, 20 leave a broken install. Helper logs go to `$TMPDIR/wails-update-<pid>.log` and are never deleted (15 had accumulated from this run; I removed them).
- **Download timeout**: the inner GitHub provider's default client has `Timeout: 30s`, which includes reading the body, so a roughly 100 MB archive on a slow line would be cut off. Production must pass `HTTPClient` with no overall timeout (context plus dial/TLS/header timeouts). Not exercised (the fake server is instant).
- Asset `url` fields come from the unsigned API response. The provider only follows URLs under `<api base>/repos/<repo>/`; the digest covers the bytes.
- A same-user attacker can edit the stored-highest file or the staged copy between gate and helper. `Staged.Restart` re-runs the gate to narrow that. Anyone who can write there can already replace the app, so the highest-version rule is aimed at a compromised release account, not at the local user.

## Unverified, and why

| Item | Why |
|---|---|
| Notarized and stapled bundle passes `spctl -a -t exec` and the requirement after Go extraction | no notarization here (S0-05); G3 only shows the check rejects un-notarized |
| `1.0.0-rc.1` as `CFBundleShortVersionString` accepted by notarization | not submitted; codesign and plutil accept it |
| Clean macOS 15 VM and Windows 11 VM runs, Windows helper swap, Windows exe version check | no VM; the Windows gate deliberately fails closed |
| Real GitHub API (redirect to the CDN, asset URL form, rate limits, token stripping), `Prerelease` list ordering on real data | no repository may be created |
| `/Applications` install by an admin and by a standard user | the spike never touches `/Applications` |
| Crash in the middle of the swap | cannot be timed; read from source only |
| Download on a slow link | see above |

## Needs manual verification

1. **Clean macOS 15 VM** (no dev tools). With real builds from F1-10 and the test channel repo: download the rc1 zip in Safari (adds quarantine), drag to `/Applications`, open once. Publish rc2. In the app: update. Expect: banner, progress, relaunch as rc2, no Gatekeeper dialog; `codesign --verify --deep --strict -R='<requirement from finding 2>' /Applications/Codepass.app` exits 0; `spctl -a -t exec -vv` says accepted, Notarized Developer ID. Repeat as a standard user with the app in `~/Applications`, and with the app in `/Applications` owned by an admin (expect the "cannot replace itself" message and an untouched app).
2. Same VM: publish an rc3 made from the rc1 zip with rc1's manifest (re-published old archive), and an ad-hoc build. The app must refuse both and keep running rc2.
3. Crash window: after the app calls Restart, `kill -9` the helper (same binary, env `WAILS_UPDATER_HELPER=1`) at different moments and look at what is left (`Codepass.app`, `Codepass.app.bak`).
4. **Windows 11 VM**: rc1 to rc2 with the cross-built exe: swap, relaunch, manifest version versus exe file version, SmartScreen on the downloaded exe, antivirus reaction to the helper re-exec, a locked exe (parent still running). The Windows gate must be written first.
5. Download of the real archive throttled to about 1 MB/s with the production HTTP client.

## Decisions this spike was asked to make

1. **Is Wails `pkg/updater` usable for swap and relaunch?** Yes for download, extract, swap, relaunch and rollback (G1). Not as a trust boundary: every trust decision moves to our provider and gate.
2. **Wrap the GitHub provider or write our own?** Wrap it, with a custom `AssetMatcher` that captures the release's asset list from the one API response the inner provider already fetched (no second request, no race). Use it for discovery and streaming only.
3. **How are two pinned keys carried?** In our provider, not in `Config.PublicKey`; the manifest's sha256 becomes Wails' digest.
4. **Where does the bundle check run?** On the staged copy, between `DownloadAndInstall` and `Restart`, and again inside `Restart`. Never rely on the helper's check.
5. **Is `HandleHelperMode` first in `main()` feasible?** Yes: one line plus import in `main.go`; builds for darwin and windows; guarded by the AST test.

## What the plan must change

**product-plan.md, section 4.14**

- Item 1: the app's provider uses the Wails GitHub provider only for discovery and streaming; version, file name, size, sha256 and notes come only from the verified manifest. The signature is over `"codepass-update-manifest-v1\n" + manifest bytes`, base64, in `manifest-<os>-<arch>.json.sig`. Unknown manifest fields and a sha256 that is not 64 hex are refused.
- Item 2: add that `updater.Config.PublicKey` is not used (one key only, and it forces Wails' own signature mode). The manifest signature is checked by our provider; Wails verifies the manifest's sha256 against the stream.
- Item 3: add "tags look like `v1.0.0-rc.N` (dot before the number)", "manifest os/arch must match", "tag is plain semver, no build metadata".
- Item 4: replace the requirement string with the one in finding 2 (adds the two Developer ID OIDs; an Apple Development certificate of the same team no longer passes). State that the check runs on the staged copy before the helper starts and again inside `Restart`, because Wails' own check runs after the swap and accepts ad-hoc over Developer ID (C1, C2). Keep `spctl -a -t exec`; it is only provable on a notarized build (manual item 1).
- Item 7: the release zip is made with `ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl`, and the script extracts it and runs the same requirement check before upload (finding 1).
- New item "Failure behaviour": every refusal removes the staging folder and leaves the installed app untouched (R1 to R9 assert it). The Wails swap is remove-then-rename with a `.bak` copy and no crash recovery; accept the millisecond window and add it to the risk table. A fresh `Updater` per check, own timer instead of `CheckInterval`, production HTTP client without an overall timeout.
- UI paragraph: the install folder must be writable; the check happens before the download and the message names the folder.
- Risk table row "An update installs something malicious or bricks the app": cite R1 to R9 as the evidence.

**tasks.md**

- F1-09: Accept add "a zip with AppleDouble `._*` entries is refused". Tests add: ad-hoc over Developer ID, Apple Development certificate, wrong bundle id, bundle version below manifest, install folder not writable, API 500, unsigned release body never shown. Build: port `internal/update` from `spike/update`; `spike/update/run.sh` becomes the macOS integration test (needs a Developer ID identity, so not in CI). Review add: "the gate runs before `Restart`, never after".
- F1-10: Build add the zip command and an extract-and-verify step; Accept add "rc tags use `-rc.N`".
- F1-W (owner of `main.go`): `updater.HandleHelperMode()` is the first statement of `main()` (done on `spike/update`; keep the AST test).
- S0-05: add "check the requirement string from finding 2 against the real notarized build, and that `spctl -a -t exec` accepts the extracted copy".
- Dependencies: `golang.org/x/mod` becomes a direct dependency (the wiring task adds it).
- F1-09 or a new task: Windows gate (exe file version equals manifest version), written and tested on the Windows VM.
