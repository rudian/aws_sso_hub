# Group B2 wiring notes (F1-18)
For F1-W, F1-11 and F1-14. Follow-ups FU-18a..h are in tasks.md.

## F1-18

### notesForWiring
- internal/app/container.go: pass the opened projects.Store as repos.Deps.Registry. It satisfies repos.Registry, and Open is projects.Open(<data>/projects.json). Quarantine and ErrNewerVersion handling are already in internal/projects. Without it the engine falls back to a memory-only registry, so IDs and test results would not survive a restart.
- api.Repo has no field for the moved offer, and internal/api is not mine. F1-11 or the wiring task should add one (for example MovedFrom {oldId, name}) and fill it from Engine.MovedOffers() in buildRepo. It should also add a service method calling Engine.ReattachMoved(ctx, newID) for the answer 'Is this acme_api, moved here?'. The strings 这是移动过来的 acme_api 吗？ / Is this acme_api, moved here? belong to that UI task.
- Relocate, Reattach and Reconcile are the calls F1-18's follow-on C3-10/C3-11 (rename or move a folder) should use. They are safe to call from outside the engine; call Engine.Scan afterwards to rebuild its list.
- Records for repositories removed from every scan root stay for 30 days and are then dropped by Reconcile on the next scan. A task that stores a repository ID must tolerate a dropped record.
- Engine.Store.RepoTests() (settings.json) is only a one-time import source. After the first release that has run once on every machine it can be removed from repos.Store.
- internal/app/container.go was edited in this fix round (outside F1-18 Owns, forced by the 'no silent fallback' fix because internal/app tests scan through the container): it now opens projects.Open(filepath.Join(filepath.Dir(p.SettingsFile), "projects.json")) and passes it as repos.Deps.Registry; an open error (store.ErrNewerVersion / store.ErrCorrupt) is returned from NewContainer unchanged. F1-W should keep this and add the user-facing handling (newer-version message, Quarantine for a corrupt file) in main/startup.
- api.ReposState / frontend services have no binding for the moved-folder question yet: the Go side exposes Engine.MovedOffers(), ReattachMoved(ctx,newID) and the new DeclineMoved(ctx,newID) for F1-14 and the wiring task to expose through a service.
- types.go gained RepoRecord.FirstSeen (*time.Time) and DeclinedMoves ([]string); projects.Registry interface in internal/repos changed (IDsForPaths and Reconcile take the scan time, new IDsForExisting, DeclineMove). Regenerate bindings only if a service later exposes these.

### deferred
- Non-fatal warning when new folders are left out of a scan because projects.json could not be written: api.ReposState has no warnings field and api types are outside this task. The folders are silently retried on the next scan; F1-W/F1-14 can add a warning field if wanted.
- The SameFile branch for two different folders whose names differ only in case is only exercised on case-sensitive filesystems (test skips on default APFS); on macOS the same-file respelling branch is exercised instead.
- 0.0.1 binary run against the data folder is not executed (per orchestrator). Unit level proof: settings.json byte-identical through the registry, import, reattach/decline and Scan, and the existing test decodes it with a v0.0.1-shaped struct; the real run stays the gate's manual check.

### manualChecks (now C8-C11)
- With a copy of the owner's real data folder (never the live one): run the new build against it, confirm projects.json appears, the 13 known repositories keep their saved connection results, and settings.json is byte-identical (cmp with the copy). Then start the 0.0.1 app against that same copy and confirm it still starts and shows its repositories.
- Rename a repository's folder inside a scan root in Finder, rescan, and confirm the moved offer appears (no UI yet, so check Engine.MovedOffers through the F1-11 row once it exists); then re-attach and confirm project, hidden flag and identity override are kept.
- On macOS (case-insensitive APFS) add the same repository once with a differently cased path and confirm it resolves to one record. This is only unit-tested with the case-fold switch, not on a real volume.
- Move a repository to an external drive, unmount it, scan, and confirm the record stays (marked missing) rather than being deleted before 30 days.
