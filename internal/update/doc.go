// Package update verifies and installs Codepass updates on top of the Wails v3 updater.
//
// Wiring notes for the shared-file task (F1-W):
//
//   - main() starts with updater.HandleHelperMode(); TestHandleHelperModeIsFirst then checks it.
//   - ControllerConfig.Quit must end the app directly, without the quit dialog or the "stay in the background"
//     choice (not plain app.Quit through F1-03's quit flow): the user has already chosen Reopen, and if the app stays
//     up the Wails helper gives up after 30 s and relaunches the old copy. The controller shows PhaseError with
//     CodeRestartTimeout after 45 s in that case.
//   - ControllerConfig.Proxy takes the system-proxy resolver of internal/netx (F1-06). An app opened from Finder has
//     no proxy variables. Leave HTTP nil (the controller builds a client with no overall timeout) or, if a client is
//     injected, give it none: the Wails default cuts a large download off after 30 s.
//   - ControllerConfig.HelperLog is a file in the data folder (for example <data>/update/restarts). Highest.Path, Channel,
//     Gate.Requirement, HelperLog and StagedRecord (for example <data>/update/staged) are all required; Check
//     refuses with CodeConfig when one is missing.
//   - Call Highest.Record(version) on every start. Start Controller.Run in a goroutine (it removes the helper logs
//     of the restarts it recorded) and call CleanHelperLogs nowhere else.
//   - State carries Code (stable, see the Code* constants) and Step (check, download, verify, restart); the UI maps
//     Code to text in both languages and logs Error.
//   - Call Controller.Close() from the app's shutdown hook: it removes a downloaded update that was never restarted
//     into and cancels a download still running (and does nothing once Restart has started the helper, even while the host is quitting inside Restart). ControllerConfig.StagedRecord (for example
//     <data>/update/staged) lets the next start's Run remove one that a crash or kill left behind. Wire
//     Controller.Retry to the UI's Retry on a refusal: a refused release is not offered again until then.
//   - The test channel is built with TestChannel(repo, token): its public keys are pinned in the binary (testKeyHex),
//     settings choose only the repository. Never read a key from settings, and never export or import one.
//   - NewTokenStore(sandbox) takes Paths.Sandbox: in sandbox mode it never touches the real keychain. Otherwise it is the Keychain on macOS (Security.framework through cgo, so the item trusts Codepass's own
//     signature) and the Credential Manager on Windows. The item's service name is "Codepass update test channel":
//     manual check A16 must read that name, not "Codepass", or it passes only because nothing matches.
package update
