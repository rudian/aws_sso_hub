# Spike S0-05: signing and notarization

Branch: `spike/signing` (worktree `.claude/worktrees/wf_39cfdb02-5e6-2`, commit 04677e0). Script: `spike/signing/sign-notarize.sh`. Not pushed.
Scratch output (not in repo): `/private/tmp/claude-501/spk/` (signed app, notary JSON and log, run log `spk-run1.log`).

## What I did

`spike/signing/sign-notarize.sh [entitlements.plist]`:
1. `wails3 task package ARCH=arm64` (current Taskfiles; ad-hoc signs, produces `bin/AWS SSO Hub.app`), then copies the bundle to a scratch dir with `ditto --noextattr` and `xattr -cr`.
2. `codesign --force --options runtime --timestamp --sign "Developer ID Application: <owner company> (XC34P8TNZ6)"` on the executable, then on the bundle, **with no entitlements file**.
3. `ditto -c -k --keepParent` to a zip, `xcrun notarytool submit --keychain-profile codepass-notary --wait --output-format json`, `notarytool log`, `stapler staple`, `stapler validate`, `spctl -a -vvv -t exec`, then a quarantined copy.
The identity is found by name in the login keychain and the notary credentials by profile name only. No secret is in any file in the repo.

## Results (all passed on the first attempt, no rejection)

```
lipo -archs: arm64
codesign --verify --deep --strict: valid on disk / satisfies its Designated Requirement
notarytool: {"id":"e1e89473-5bf2-44bd-a3a1-386848d58e2e","status":"Accepted"}   issues: null
stapler staple/validate: "The staple and validate action worked!" / "The validate action worked!"
spctl -a -vvv -t exec: accepted  source=Notarized Developer ID  origin=Developer ID Application: <owner company> (XC34P8TNZ6)
quarantined copy (xattr -w com.apple.quarantine "0083;<hex time>;Safari;"): spctl accepted, source=Notarized Developer ID
codesign -dvv: Identifier=com.rudian.awsssohub  flags=0x10000(runtime)  TeamIdentifier=XC34P8TNZ6
  Authority=Developer ID Application > Developer ID Certification Authority > Apple Root CA ; secure Timestamp present
codesign -d --entitlements -: none (empty)
```

Timings (this Mac, arm64, warm caches): build 37 s, sign 1 s, zip 1 s, notarization 27 s (submit + wait), staple 2 s. Total 69 s. A team's first-ever submissions can be slower; give the release script a generous `--wait` timeout.

### For plan 4.14

- **Team ID: `XC34P8TNZ6`**
- **Designated requirement (verbatim from `codesign -d -r-`):**
  `identifier "com.rudian.awsssohub" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = XC34P8TNZ6`
- The 4.14 step-4 requirement string was tested and works against the notarized copy:
  `codesign --verify --deep --strict -R='anchor apple generic and identifier "com.rudian.awsssohub" and certificate leaf[subject.OU] = "XC34P8TNZ6"' <app>` returns 0; the same with `"WRONGTEAM1"` fails with "code failed to satisfy specified code requirement(s)" (rc 3). So the plan's string is correct once `<TeamID>` is `XC34P8TNZ6`.
- The bundle identifier is `com.rudian.awsssohub`. Changing it breaks the identifier clause of the updater requirement for existing installs and changes the single-instance ID. Decide in F1-01 and keep the requirement string in sync.

### Entitlements actually needed

Hardened runtime with **no entitlements** was accepted by notarization with no issues. The binary links only Cocoa, WebKit, Security, ServiceManagement, UniformTypeIdentifiers, Carbon, CoreServices; no `dlopen` or `plugin.Open` in `internal/` or `main.go`. So `allow-jit`, `allow-unsigned-executable-memory` and `disable-library-validation` are not expected to be needed (WebKit content runs in Apple's own WebContent process).
One likely addition (untested, see below): `internal/terminal/terminal.go` opens Terminal/iTerm through `osascript`. Sending Apple Events from a hardened-runtime app needs `com.apple.security.automation.apple-events` and `NSAppleEventsUsageDescription` in `Info.plist`; the latter is absent today (grep of `build/darwin/Info.plist` is empty). Recommend adding both in F1-01; harmless to notarization.

## What did not work / is unverified

- **Running the signed, hardened build was not verified.** `main.go` sets `SingleInstance` with the bundle ID and the owner's installed `/Applications/AWS SSO Hub.app` (pid 43529) is running, so launching my signed copy (also with a temp `HOME`) exited at once with rc 0, same as the ad-hoc build: it handed over to the running instance (this may have raised the owner's window briefly). So I cannot show from evidence that the hardened app starts, or that WebView, Keychain, `osascript` Terminal launching and `git`/`aws` child processes work under it.
- "Opens with a plain double-click on a clean Mac" needs a clean machine. `spctl` acceptance of a quarantined copy is the local proxy; the quarantine flag was written with `xattr -w`, not set by a real browser download.
- The `.p12` backup is an owner action (Keychain Access: export the "Developer ID Application" certificate with its private key, keep offline). Not done by me.
- The keychain also holds Apple Development and Apple Distribution identities for the same team. Only "Developer ID Application" is right for direct distribution; the script pins the full identity name.

### Needs manual verification

1. Quit the installed AWS SSO Hub (single instance), then `open "/private/tmp/claude-501/spk/quarantine/AWS SSO Hub.app"` (or re-run the script). Confirm the window renders, sign-in opens the browser, credential reads work, and "Open in terminal" works (macOS should ask for Automation permission; if nothing opens and no prompt appears, add the apple-events entitlement and usage string, re-sign).
2. On a clean macOS VM or second Mac: download the zip in a browser (real quarantine), double-click, expect no Gatekeeper warning, also offline (stapled ticket).
3. Owner: export the `.p12` backup.

## Decisions this spike was asked to make

- The signing path works end to end on the owner's Mac in about 70 s: Taskfile package output plus a re-sign. `codesign:adhoc` in `build/darwin/Taskfile.yml` is replaced by `--force` re-signing in the release script.
- Use `codesign --options runtime --timestamp`, executable first and bundle second; `--deep` only for verification. One Mach-O is enough today; if helper binaries are bundled later, sign each individually before the bundle.
- Notarize a `ditto` zip; staple the `.app`; build the release archive from the stapled app (the zip sent to Apple has no ticket).
- Team ID and DR as recorded above.

## What the plan must change

product-plan.md
- 4.14 step 4: fill `<TeamID>` with `XC34P8TNZ6` (string verified). Add that the release archive is built from the stapled app.
- 4.14 step 7: add "sign with `--options runtime --timestamp`, no entitlements by default; add `com.apple.security.automation.apple-events` plus `NSAppleEventsUsageDescription` if the terminal-launch check fails". Add the timing (about 70 s end to end).
- Add a risk row: the bundle ID is part of the updater requirement and single-instance ID; renaming to Codepass must either keep `com.rudian.awsssohub` or change the requirement and accept a manual reinstall for 0.0.1 users. State the choice.

tasks.md
- S0-05: mark "Team ID and DR recorded" done; leave "clean Mac double-click" and ".p12 backup" as owner checks (steps above).
- F1-01 (rename to Codepass): add `NSAppleEventsUsageDescription` to `build/darwin/Info.plist`; replace `codesign:adhoc` for release builds; decide the bundle ID.
- F1-10 (release scripts): base on `spike/signing/sign-notarize.sh`; fail on any status other than `Accepted` and save `notarytool log`; refuse on a dirty tree; identity and `codepass-notary` profile by name only.
- F1-09/F1-10 acceptance: add a launch smoke test of the signed build with the installed app quit, because single-instance behaviour hides launch failures.
