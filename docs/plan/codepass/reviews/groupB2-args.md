# groupB2: notes passed to the workflow (wf_cd24a0fd-ced)

Extracted from each implementer's first prompt; examples for the next group's `groupNote` and `notes` args.

## groupNote

Milestone 1, group B2 (one task, serial). Group B is merged into codepass: internal/store (versioned atomic JSON files; store.File.Update zeroes v and needs a non-nil pointer; Quarantine takes the target pointer), internal/projects (projects.json; RepoRecord already has MissingSince *time.Time, FirstCommit and OriginURL), internal/gitx (read-only git layer: use it for any git read), internal/watch, internal/netx, internal/update, internal/appguard. Follow-up FU-02a in tasks.md is yours. The owner's installed AWS SSO Hub app may be running on this Mac: never run, quit, kill, move or trash it, and never point any binary at the owner's real data folder. Disk space is limited (about 8 GB free): delete large temporary files you create. Part of the gate: CODEPASS_SCRUB_LIST=~/.codepass-dev/scrub-list.txt bash scripts/scrub-check.sh --tracked must exit 0 (never open, print or quote that list).

## notes["F1-18"]

Acceptance item 'the 0.0.1 binary run against the data folder still starts': do not run any real binary (no 0.0.1 build, no installed app). Prove it at unit level instead: settings.json stays byte-identical through the registry, the import and every write path, and the v0.0.1 settings struct (git show v0.0.1:internal/settings/settings.go) still decodes the resulting settings.json. The real run is the gate's manual check with a copy of the owner's data folder. Case-insensitive paths: macOS (APFS default) and Windows resolve differently-cased paths to one record; Linux stays case-sensitive. Keep legacyId exactly the 0.0.1 hash (pinned per OS in F1-02's test).
