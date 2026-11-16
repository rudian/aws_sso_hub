# NEW USER REQUIREMENTS (added by the orchestrator after the workflow started) — BINDING

Source: the user, 2026-11-04 (translated): "On macOS, after SSO login, the login must stay valid even after the app quits.
After SSO login, the app must show how long the current SSO account's token is still valid."

## R8 — Login survives app exit (macOS and Windows)
- The login lives ONLY in the AWS-CLI-compatible token cache (~/.aws/sso/cache/<sha1(session-name)>.json, 0600; Windows: %USERPROFILE%\.aws\sso\cache).
  Nothing about the login may depend on the app process running.
- The app must NOT log out, delete, or invalidate tokens on quit, window close, crash, or restart.
- After quitting, `git` (via `aws codecommit credential-helper`) and `aws ... --profile X` must keep working until the IAM Identity Center session ends.
- On next launch the app re-reads the cache and shows the correct state immediately (no re-login needed).

## R9 — Show remaining validity per SSO login
Facts to respect (verify against docs/research/aws-sso.md; if research found an authoritative source for the session end, prefer it):
- The cached `accessToken` lives ~1 hour (`expiresAt`); it is auto-renewed with `refreshToken` by the aws CLI / SDK / this app.
  So `expiresAt` alone is NOT "how long my login lasts" — showing only that would wrongly tell users to log in every hour.
- The real limit is the IAM Identity Center session duration (refresh-token lifetime; default 8 h; admin-configurable).
  It is not in the cache file and (as far as we know) not returned by any API available to the end user.

Spec (the plan must define exact data fields, Go/TS contract, UI states, and tests for this):
1. Per sso-session, the app stores in its own data file (never secrets): `lastLoginAt` (set when the app completes an interactive login)
   and `sessionDurationHours` (per portal, default 8, editable in the portal settings with a hint explaining where admins change it).
2. Main display on every account card / portal header: "session remaining" = lastLoginAt + sessionDuration − now,
   e.g. zh "还剩 6 小时 12 分" / en "6h 12m left", with a progress ring/bar and color steps (normal → warning < 1 h → danger < 15 min).
   Live countdown (updates every minute; every second in the last 5 minutes is fine).
3. Details (tooltip or expandable): login time, estimated session end (local time), access-token expiry (with "auto-renews"),
   and — when the app has fetched them — the role-credential expiry for that profile.
4. Login done outside the app (e.g. `aws sso login` in a terminal → the cache's clientId/token changed without the app doing it):
   show "Logged in (session length unknown) · token auto-renews" plus a hint "log in from this app to get a full countdown".
   Do not invent a precise countdown in that case.
5. Truth check: the state is "logged in" only if the access token is unexpired OR a refresh succeeds. When the access token is
   within the CLI's refresh window (or expired), the app refreshes it exactly like the CLI does and writes it back atomically in the
   CLI format (temp file + rename, 0600). If refresh fails (invalid_grant / expired), the state becomes "expired — log in again"
   regardless of the estimate. Re-check on app launch, on window focus, and every few minutes while running.
6. When the estimate reaches zero but refresh still works, keep showing "logged in" and treat sessionDurationHours as too short
   (offer to update it). When refresh fails earlier than the estimate, mark expired (optionally suggest a shorter duration).
7. P1: if a tray / menu-bar menu exists, show each session's remaining time there too; optional reminder 15 min before the end.
