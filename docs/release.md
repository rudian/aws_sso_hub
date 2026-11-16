# Releasing Codepass

Releases are built, signed, notarized and uploaded **on the owner's Mac**. Nothing here runs in CI, and no
secret is read from the repository or from CI: the Developer ID certificate and the notary credentials live in
the login keychain, the ed25519 update key is a file you keep outside the repository. The scripts are in
`build/release/`; the update checks they feed are described in plan 4.14.

## What the release does

Two commands, with the smoke test between them:

```
build/release/release.sh [--repo OWNER/NAME] 1.0.0-rc.1            # build, sign, notarize, upload a DRAFT
build/release/smoke.sh <output folder>/verify/Codepass.app          # you, on the extracted app
build/release/release.sh --publish [--repo OWNER/NAME] 1.0.0-rc.1  # verify the draft's assets, publish
```

1. Refuses a dirty working tree (an untracked source file counts, whatever your `status.showUntrackedFiles`; a
   failing `git status` counts as dirty), a version that is not `1.0.0` or `1.0.0-rc.N` (a dot before the number,
   no leading zeros, N at least 1: `rc10` would sort below `rc2`, `rc.01` is not semver, and `rc.0` would share a
   Windows file version with the final), a source tree whose version differs
   from the one released (`internal/version`, `build/darwin/Info.plist`, `build/config.yml`, `build/windows/info.json`), a missing
   `CODEPASS_EXPECT_PUBKEY`, and a signing key that lives inside the repository or is readable by others.
2. Exports `HEAD` into `src/` in the output folder and builds the arm64 bundle there (`wails3 task package`), so
   the checkout is never touched and the bundle is exactly the committed tree. Makes a clean copy (no extended
   attributes, no `._*`, no `.DS_Store`), checks bundle ID, version and architecture.
3. Re-signs with `--force --options runtime --timestamp`, the executable first and then the bundle (`--deep` only
   to verify), and checks the bundle against the updater's full designated requirement (plan 4.14 item 4).
4. Zips with `ditto`, notarizes with `notarytool submit --wait --timeout 30m`, **stops on any status other than
   `Accepted`**, and always keeps the `notarytool log` (`notary-log.json`) in the output folder.
5. Staples and validates the ticket, and checks Gatekeeper reports `Notarized Developer ID`.
6. Builds the release archive **from the stapled app** with
   `ditto -c -k --keepParent --norsrc --noextattr --noqtn --noacl` (a plain `ditto` zip carries `._*` entries
   that break the signature after extraction).
7. Lists the archive with `zipinfo -1` (stops on any `._*`, `__MACOSX` or `.DS_Store` entry, or on anything but
   exactly one top-level `Codepass.app`), extracts it with `unzip` (which, like the app's own extractor, writes
   any `._*` entry as a file instead of merging it into extended attributes) and runs the same checks again:
   `codesign --verify --deep --strict`, the requirement, `stapler validate`, `spctl`. This is what the user gets.
8. Writes `manifest-darwin-arm64.json`, signs it with the ed25519 key (`relkit`, `build/release/relkit`), and
   verifies the signature and the archive's size and sha256 with the public key before going on.
9. Checks that no step was skipped (a skipped step is a failure; this happens before the upload), that `HEAD` did
   not move and the tree is still clean, then creates a **draft** release with `gh`: the six files (the macOS archive,
   manifest and signature, and the same three for Windows) and a body that repeats the crash-recovery steps below. Release candidates are marked
   prerelease. The body names the commit that was built (`Built from commit ...`); a local tag of the same name must point at
   it, and when the target repository is this source repository the tag is created with `--target` at that commit.
   It reads the asset list back and stops. **Nothing is published yet.**
10. After the smoke test passes, `release.sh --publish VERSION` downloads the draft's assets, verifies the
    manifest signature and the archive digest against `CODEPASS_EXPECT_PUBKEY`, and only then publishes.

Everything lands in a fresh `codepass-release.XXXXXX` folder under `--out` (default `$TMPDIR`), which must be
outside the repository (a `..` in `--out` is refused); the script prints its path. Delete it when you are done; the app inside is a few hundred
megabytes.

Before publishing, quit every installed copy and run the launch smoke test on the extracted app:

```
build/release/smoke.sh <output folder>/verify/Codepass.app
```

It refuses to start while Codepass or the old AWS SSO Hub is running (both share one bundle ID and one
single-instance ID, so a second copy hands over and exits, which hides a launch failure). It starts the app the
way a double click does (`open -n`, so macOS treats Codepass, not your terminal, as the responsible process and
the app gets launchd's environment, not your shell's PATH), finds its process, requires it to stay up for ten
seconds and to open a window, then asks you to confirm browser sign-in, a keychain read and "Open in terminal".
The Automation prompt for "Open in terminal" must name Codepass; if it names your terminal, the test was not
run the right way. Any "no" fails the test. If it fails, leave the draft unpublished and delete it.

## Settings (environment or `build/release/local.env`)

Copy `build/release/local.env.example` to `build/release/local.env` (git-ignored) and fill it in. The same names
work as environment variables. Nothing in the repository holds an identity name, a profile name or a home path.

| Name | Meaning |
|---|---|
| `CODEPASS_SIGN_IDENTITY` | Full name of the `Developer ID Application` identity (`security find-identity -v -p codesigning`). Apple Development and Apple Distribution identities are refused. |
| `CODEPASS_TEAM_ID` | The Team ID the updater pins (the certificate's OU). |
| `CODEPASS_NOTARY_PROFILE` | Name of the `notarytool` keychain profile (`xcrun notarytool store-credentials <name>`). |
| `CODEPASS_SIGNING_KEY_FILE` | The current ed25519 private key (hex, mode 600, outside the repository). |
| `CODEPASS_EXPECT_PUBKEY` | Required for a real release and for `--publish`. The public key (hex) the key file must match; set it to the key pinned in the app. |
| `CODEPASS_ENTITLEMENTS` | `--dry-run` only: another entitlements plist to sign with (a test hook). A real release refuses it and signs only the committed `build/darwin/entitlements.plist`. |
| `CODEPASS_RELEASE_REPO` | `owner/name` to upload to (or `--repo`). |
| `WAILS3`, `GH`, `CODEPASS_OUT_DIR`, `NOTARY_TIMEOUT` | Optional tool paths, output base folder (same as `--out`), notary wait (default `30m`). |
| `CODEPASS_NO_LOCAL_ENV`, `CODEPASS_LOCAL_ENV` | `1` skips `local.env`; the second names another file to read instead. `test.sh` sets the first. |

## Bumping the version

The version is in four places and the script checks, before it builds, that they agree with the one you release:
`internal/version/version.go` (`Version`), `build/darwin/Info.plist` (`CFBundleVersion` and
`CFBundleShortVersionString`), `build/config.yml` (`info.version`) and `build/windows/info.json` (`file_version`,
`product_version`, `FileVersion`, `ProductVersion`). Change all four, commit, then release.

The Windows file version is the release version with four parts, one-to-one (rc numbers start at 1):
`1.0.0` -> `1.0.0.0`, `1.0.0-rc.N` -> `1.0.0.N`. This is the contract F1-20's "exe file version equals manifest
version" gate builds on.
Optional release notes go in `docs/release-notes/<version>.txt` (plain text, at most 16 KB); the app shows them
only from the signed manifest.

## Key ceremony (once, on the owner's Mac)

The update key signs every manifest. Whoever holds it can ship code to every install, so it never goes to
GitHub, CI, a chat, or a cloud drive in clear text.

1. Make a folder outside every repository and keep it private (`keygen` refuses a folder inside a git work tree):
   `go run ./build/release/relkit keygen -dir "<private folder>/current"`.
   It writes `current.priv` (mode 600) and `current.pub` (hex), and refuses to overwrite an existing key.
2. Do it a second time for the **next** key (`.../next`). The app pins both public keys (plan 4.14 item 2) so the
   key can be rotated: ship a release signed with the current key that pins the next one, then switch signing
   to the next key and generate a new "next".
3. Put the public keys into the key constants compiled into the app (`internal/update`: `pinnedKeyHex` for production,
   `testKeyHex` for the test channel), then commit and build. The app never reads a key from settings or from an
   imported file: the trust anchor is in the binary (plan 3.3). Keep the private keys in two places
   you control: the Mac, and an encrypted offline copy (for example a password-protected disk image on a USB
   stick in a drawer). Losing the only copy means no further updates can be signed; every user would have to
   reinstall by hand.
4. Point `CODEPASS_SIGNING_KEY_FILE` at the current private key and set `CODEPASS_EXPECT_PUBKEY` to its public key.
5. Back up the Developer ID certificate with its private key as a password-protected `.p12`
   (Keychain Access, "Export"), also in two places.

Test the whole chain with a throwaway key before using the real one: `--dry-run` always generates one on the fly
and never signs with `CODEPASS_SIGNING_KEY_FILE`, even when it is set. If `CODEPASS_EXPECT_PUBKEY` is also set, a
dry run only compares the key file's public half with it.

## The test channel

The test channel is a **private** GitHub repository that serves release candidates to the owner's own installs.
It is separate from the production key and, by default, from the production repository: the updater accepts asset
URLs only under the channel's own configured repository, so the test repository can have any name. Use a
throwaway name (for example `rudian/codepass-testchannel`) so the source repository's rename to `rudian/codepass`
at release (R7-08) is not blocked. If you decide to use the future production repository as the test channel,
record that decision first. The app reads the channel with a read-only fine-grained token for that one
repository, entered once in the app and kept in the Keychain (F1-09); the production channel needs no token.

Setup by the owner (none of it is done by the scripts or by this task):

1. Create the private repository **with an initial commit** (a release has to tag a commit; with an empty
   repository the first upload fails after a full notarization).
2. Make a **test signing key**, separate from the production key, in its own folder outside every repository:
   `go run ./build/release/relkit keygen -dir "<private folder>/testchannel"`. Put its public half (`current.pub`)
   into `testKeyHex` in `internal/update`, commit and build: the app pins it in the binary and never reads a key
   from settings. Test-channel releases are
   signed with this key and never with the production key: run them with
   `CODEPASS_SIGNING_KEY_FILE="<private folder>/testchannel/current.priv"`,
   `CODEPASS_EXPECT_PUBKEY=<its public key>` and `--repo <owner>/<test repository>`.
3. Create a fine-grained token with **Contents: read** on that repository only, for the app.
4. `gh auth login` on the Mac with an account that can create releases there.
5. Release candidates: tags `v1.0.0-rc.1`, `v1.0.0-rc.2`, ... Final: `v1.0.0`.

### System notifications in a development build

A release build (`-tags production`, `build/darwin/Taskfile.yml` `BUILD_FLAGS`) uses the system's notification
service. Sandbox mode (`AWSSSOHUB_SANDBOX`) and every test use a fake, which also shows each notice as a toast in the
window. A development build (no `production` tag) uses the fake too. `AWSSSOHUB_REAL_NOTIFICATIONS=1` switches it to the
real service, but only without `AWSSSOHUB_SANDBOX` (never inside `go test`), so such a build uses the real data
folder, the real `~/.aws` and the installed app's bundle ID: its permission answer, its notifications and a click on
them are the installed app's. That variable is for a deliberate check on the owner's own data only. The owner's checks
C22 to C24 and B19 use a test-channel build, which is the installed Codepass with the same bundle ID and data folder.

## Dry run (what CI can run)

```
build/release/release.sh --dry-run [--app <a bundle>] --out <folder> <version>
build/release/test.sh
go test ./build/release/...
```

`--dry-run` calls no Apple service and no upload (`--app` is allowed only here): the bundle is signed ad hoc, notarize and staple are reported as
skipped, a throwaway ed25519 key signs the manifest (never the configured key), and the `gh` commands are printed, not run. It also asserts
that the requirement check **refuses** the ad-hoc bundle, which proves the check is live. `test.sh` builds a tiny
fake bundle for this, runs `release.sh` isolated from `local.env` and from every `CODEPASS_*`, `GH`, `WAILS3`
variable (so it passes on the release Mac), and checks the refusals (bad versions including leading zeros, bundle
version mismatch, output inside the repository also through a symlinked checkout, a real run without an identity,
a real run with `--app`, an AppleDouble file in the bundle), the dirty-tree check, and `--publish` against a stub
`gh`. `relkit`'s Go tests cover the manifest and the
signature: a golden manifest and signature, round trip, wrong key, second pinned key, tampered manifest, changed archive, unknown fields, domain
separation, key files that are never overwritten.

## Windows

Windows ships unsigned in 1.0 (owner decision 3). `release.sh` cross-builds the exe in the same HEAD export
(`wails3 task windows:build ARCH=amd64`, production flags), packages it as `Codepass-<v>-windows-amd64.zip`
(holding only `codepass.exe`), and signs `manifest-windows-amd64.json` with the same key and `relkit`. All six
assets go into the one draft, and `--publish` verifies both manifests. `build/windows/info.json` must carry the
file version for the release or the script stops: `1.0.0` is `1.0.0.0`, `1.0.0-rc.2` is `1.0.0.2`
(`file_version`, `product_version`, `FileVersion`, `ProductVersion`). `--app` (dry run) skips the Windows build
unless `--windows-exe PATH` is given.

## Entitlements

If `build/darwin/entitlements.plist` exists, `release.sh` signs both the executable and the bundle with it, and
writes what the signature holds to `entitlements.txt` in the output folder. It does not exist by default; add it
only if the smoke test shows 'Open in terminal' needs `com.apple.security.automation.apple-events`.

## Recovery after a crash in the middle of a swap

The updater swaps the app by copying the running bundle to `Codepass.app.bak`, removing `Codepass.app`, and
renaming the new bundle into place. If the helper dies in that short window there is no `Codepass.app`, only
`Codepass.app.bak` next to where it was (usually `/Applications`). Recover by hand:

```
cd /Applications
ls -d Codepass.app Codepass.app.bak
# only if Codepass.app is missing:
mv Codepass.app.bak Codepass.app
```

If both exist, the swap finished and `Codepass.app.bak` is the old version; delete it when you are sure the new one
runs. If `Codepass.app` exists but does not start and `.bak` is the old version, quit the app, then
`mv Codepass.app Codepass.app.broken && mv Codepass.app.bak Codepass.app`. Then download the release again from
the channel. The GitHub release body of every release repeats these steps.

## Not done by the scripts

Creating the repository, any release or token, generating the production keys, the `.p12` backup, and the clean
Mac double-click test are the owner's steps (`docs/plan/codepass/manual-checks.md`).
