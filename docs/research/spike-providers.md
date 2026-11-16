# S0-07 Provider tokens: findings

Branch: `spike/providers` (probe scripts only: `spikes/providers/probe-github.sh`, `probe-bitbucket.sh`). No token value was printed or written: `gh auth token` was never run; only response headers were grepped; the only fake tokens used were the literal strings `ghp_000...x` and `notatoken`. No tokens were created, so "revoke afterwards" does not apply. Nothing was created, merged, deleted or pushed on either provider.

## 1. What I did

- GitHub: read-only `gh api -i` calls with the already-signed-in `gh` (a classic token with scopes `admin:org, repo, workflow`) against a public repo (`cli/cli`), the owner's public repo and private org repos. Create / merge / delete-branch were NOT called; their scopes come from docs.
- Bitbucket Cloud: no account. Used Atlassian docs, the public OpenAPI file `https://api.bitbucket.org/swagger.json` (per-operation OAuth scope names), and anonymous calls to a public repo for headers and error shapes.

## 2. Decision (owner decision 2 confirmed workable)

One classic GitHub PAT with `repo` + `read:org` covers every endpoint the app needs, including checks. The fine-grained question in the S0-07 Accept line is moot after decision 2 and stays unverified (no token to test; creating one is an outward action).

## 3. GitHub: endpoints and minimum scope

`X-Accepted-OAuth-Scopes` is shown where GitHub sent it. GitHub leaves it empty on many repo endpoints, so "repo" there comes from docs, not from the header.

| Action | Endpoint | Observed | Min classic scope | Fine-grained (docs, not tested) |
|---|---|---|---|---|
| Verify token | `GET /user` | 200; `X-OAuth-Scopes` lists granted scopes | none | none |
| List user repos | `GET /user/repos` | 200 | `repo` (private) | Metadata: read |
| List orgs | `GET /user/orgs` | 200; accepted: `admin:org, read:org, repo, user, write:org` | `read:org` | not found in docs page fetched |
| List PRs in a repo | `GET /repos/{r}/pulls` | 200, ETag, Link rel=next | `repo` (private) | Pull requests: read |
| PR detail (mergeable, mergeable_state) | `GET /repos/{r}/pulls/{n}` | 200; accepted: `repo` | `repo` | Pull requests: read |
| Reviews | `GET .../pulls/{n}/reviews` | 200 | `repo` | Pull requests: read |
| Requested reviewers | `GET .../pulls/{n}/requested_reviewers` | 200 | `repo` | Pull requests: read |
| PRs across repos | `GET /search/issues?q=is:pr+is:open+review-requested:@me` | 200; own bucket 30/min; no ETag | `repo` for private results | Pull requests: read |
| Same via GraphQL | `POST /graphql` (`search`, `reviewDecision`, `mergeStateStatus`, `statusCheckRollup`) | 200; 5000 points/h | `repo` | n/a |
| Check runs | `GET /repos/{r}/commits/{sha}/check-runs` | 200 on public `cli/cli` (858 runs); 200 `total_count:0` on a private repo | `repo` for private (docs) | Checks: read (unverified) |
| Combined status | `GET /repos/{r}/commits/{sha}/status` | 200; accepted: `repo, repo:status` | `repo` or `repo:status` | Commit statuses: read |
| Branch protection (explain merge blocks) | `GET /repos/{r}/branches/{b}/protection` | 404 `Branch not protected` | `repo` + admin on repo | Administration: read |
| Allowed merge methods | `GET /repos/{r}` | 200: `allow_merge_commit/squash_merge/rebase_merge`, `permissions.push` | `repo` | Metadata: read |
| Create PR | `POST /repos/{r}/pulls` | not called | `repo` | Pull requests: write |
| Request reviewers, create review | `POST .../requested_reviewers`, `.../reviews` | not called | `repo` | Pull requests: write |
| Merge PR | `PUT /repos/{r}/pulls/{n}/merge` | not called | `repo` | Contents: write (docs, S24) |
| Close PR | `PATCH .../pulls/{n}` state=closed | not called | `repo` | Pull requests: write |
| Delete source branch | `DELETE /repos/{r}/git/refs/heads/{b}` | not called | `repo` | Contents: write (docs) |

Docs: scopes and headers https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/scopes-for-oauth-apps ; fine-grained permission per endpoint https://docs.github.com/en/rest/authentication/permissions-required-for-fine-grained-personal-access-tokens .

Findings worth acting on:
- `workflow` is not needed. `read:org` is only for listing orgs and team requests; `GET /user/orgs` also accepts `repo`, so `repo` alone may suffice. Untested (only a broader token was available). Keep `read:org` as decided.
- Check runs and combined status are separate systems. Actions and Apps write check runs; legacy CI writes statuses. Show both, or use one GraphQL `statusCheckRollup` (returned `null` for commits with neither; map to "no checks").
- `mergeable` can be `null` right after a push (lazy computation). Seen: `mergeable:false, mergeable_state:"dirty"` on a conflicting dependabot PR; GraphQL `mergeStateStatus` gave `DIRTY` and `CLEAN`. Re-poll after a few seconds when null (docs-based, null case not seen).

## 4. GitHub: rate limits and ETag (measured)

- `GET /rate_limit` (free): core 5000/h, search 30/min, graphql 5000 points/h. Every response carries `X-RateLimit-Limit/-Remaining/-Reset/-Used/-Resource`.
- ETag on PR list and detail: weak `W/"..."` (private org repo) or strong (public repo). `Cache-Control: private, max-age=60`.
- `If-None-Match: <etag>` returned `HTTP/2.0 304 Not Modified` for the PR list and the PR detail. `X-RateLimit-Remaining` was 4974 before and 4974 on the 304, so a 304 did not cost quota in this measurement. The 304 echoed the strong form of a tag that the 200 sent as `W/`; replaying the stored weak string still matched.
- `/search/issues` returned no ETag, so cross-repo "to review" cannot use 304 and uses 1 of 30 per minute per poll. GraphQL has no ETag either. Poll "to review" at most every 60 s while the PR page is open, else every 15 min (P6-07).
- Primary exhaustion: 403 or 429 with `x-ratelimit-remaining: 0` and `x-ratelimit-reset`. Secondary: 403/429, honour `retry-after` if present, else wait 1 min then back off exponentially; 100 concurrent requests max, 900 points/min REST (GET 1, write 5). Docs https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api . Not triggered, so shapes are docs-only.

## 5. GitHub: error shapes (observed unless marked)

| Situation | Status | Body / header |
|---|---|---|
| Invalid token | 401 | `{"message":"Bad credentials","documentation_url":"https://docs.github.com/rest","status":"401"}` |
| Valid token, missing scope | 404 (not 403) | `{"message":"Not Found","documentation_url":"https://docs.github.com/rest/users/emails#list-email-addresses-for-the-authenticated-user","status":"404"}`; only clue: `X-OAuth-Scopes: admin:org, repo, workflow` vs `X-Accepted-OAuth-Scopes: user, user:email` (sample: `GET /user/emails`) |
| No auth, private repo | 404 | same `Not Found` body; rate limit 60/h |
| Branch not protected | 404 | `{"message":"Branch not protected",...}` |
| Fine-grained / SAML (docs, not reproduced) | 403 | `Resource not accessible by personal access token`; SAML adds header `X-GitHub-SSO: required; url=...` |

Client rule: on 404 with a token, if `X-Accepted-OAuth-Scopes` is non-empty and shares nothing with `X-OAuth-Scopes`, say "token lacks scope X"; otherwise "not found or no access". Show `message` as plain text. 401 = token expired: degrade to "open in browser".

## 6. Bitbucket Cloud: auth

Source: https://support.atlassian.com/bitbucket-cloud/docs/using-api-tokens/
- REST: `Authorization: Bearer <api_token>` (no email needed), or Basic `email:api_token`. Use Bearer.
- Git over HTTPS username: the Bitbucket username (case-sensitive) with the token as password, or the static names `x-bitbucket-api-token-auth` / `x-token-auth`. Tokens in the remote URL are discouraged. The app uses SSH for git, so this matters only for an HTTPS fallback.
- API tokens always carry scopes (names like `read:repository:bitbucket`); clone needs `read:repository`, push also `write:repository`.
- The public swagger uses the older OAuth names (`pullrequest:write`); the mapping below is mine and must be confirmed on a real token-creation screen.

## 7. Bitbucket: minimum scopes per action

OAuth names come from `swagger.json` `security` entries; API-token names from https://support.atlassian.com/bitbucket-cloud/docs/api-token-permissions/ .

| Action | Endpoint | OAuth scope (swagger) | API token scope (`:bitbucket`) |
|---|---|---|---|
| Verify user | `GET /user` | `account` | `read:user` |
| List workspaces | `GET /user/workspaces` | `account` | `read:workspace` (and `read:user`, to be safe) |
| List repos in workspace | `GET /repositories/{ws}` | `repository` | `read:repository` |
| List PRs | `GET .../pullrequests?state=OPEN` | `pullrequest` | `read:pullrequest` (docs: does not imply repository read) |
| PR detail, diffstat | `GET .../pullrequests/{id}` | `pullrequest` | `read:pullrequest` |
| Create PR (sets `close_source_branch`) | `POST .../pullrequests` | `pullrequest:write` | `write:pullrequest` |
| Approve / request changes | `POST .../approve`, `.../request-changes` | `pullrequest:write` | `write:pullrequest` |
| Merge | `POST .../merge` | `pullrequest:write` | `write:pullrequest` |
| Decline | `POST .../decline` | `pullrequest:write` | `write:pullrequest` |
| Close source branch | flag `close_source_branch:true` in the create or merge body, no separate call | covered by merge | `write:pullrequest` |
| Delete a branch by hand | `DELETE .../refs/branches/{name}` | `repository:write` | `write:repository` (not needed) |
| PR build statuses | `GET .../pullrequests/{id}/statuses` | `pullrequest` | `read:pullrequest` |
| Commit statuses | `GET .../commit/{sha}/statuses` | `repository` | `read:repository` |
| Pipelines (optional) | `GET .../pipelines` | `pipeline` | `read:pipeline` |
| Default reviewers | `GET .../effective-default-reviewers` | `pullrequest` | `read:pullrequest` |

Minimum token: `read:user`, `read:workspace`, `read:repository`, `read:pullrequest`, `write:pullrequest` (each `:bitbucket`). Add `read:pipeline` only if pipeline detail is shown.

Merge (swagger): `merge_strategy` in `merge_commit, squash, fast_forward, squash_fast_forward, rebase_fast_forward, rebase_merge` (default `merge_commit`); `?async=true` returns 202 with `Location` to `.../merge/task-status/{task_id}`; a sync call can also return 202 after a timeout. Errors: 400 cannot merge, 409 a ref changed, 503 merge queue unavailable, 555 timed out (retry). Create PR: 400 invalid, 401 unauthenticated.

Cross-workspace: `GET /workspaces` is gone (observed 404 `There is no API hosted at this URL`) and the swagger has no `/pullrequests/{user}`. "My PRs" and "to review" therefore need one list call per repo per workspace slug the user entered, filtered client-side. Server-side `q=` filtering by reviewer with token auth is unverified.

## 8. Bitbucket: rate limits, ETag, error shapes

- Limits (https://support.atlassian.com/bitbucket-cloud/docs/api-request-limits/): anonymous 60/h per IP; `/2.0/repositories/*` 1000/h by default, scaled to at most 10000 only for Standard/Premium with 100+ paid users and workspace/project/repository access tokens (a personal API token stays at 1000/h); git over HTTPS/SSH 60000/h; rolling hour. `X-RateLimit-Limit/-Resource/-NearLimit` appear only on scaled endpoints. The docs do not describe 429 or `Retry-After`. The binding constraint is about 1000 requests per hour across all repos, so ETag is essential and the client must count its own calls (no remaining header).
- ETag (observed anonymously): a repo GET sent `etag: "35bd..."`; `If-None-Match` returned `HTTP/2 304`; `cache-control: max-age=900`; PR list also sends an etag. Whether a 304 counts toward the 1000/h is undocumented and untestable here.
- Errors (observed), shape `{"type":"error","error":{"message":"...","detail":"..."}}`:
  - Bad Bearer: 401, `www-authenticate: OAuth realm="Bitbucket.org HTTP"`, message `Token is invalid, expired, or not supported for this endpoint.`
  - No auth on `/user`: 401, empty body, `www-authenticate: BitbucketCustom realm=Bitbucket.org HTTP`.
  - No access or missing repo: 404 `You may not have access to this repository or it no longer exists in this workspace...`.
  - Missing scope with a valid token: not reproduced (no account). Responses carry `x-accepted-oauth-scopes` (e.g. `pullrequest` on the PR list), so the client can name the needed scope from it.

## 9. What did not work / unverified

- Fine-grained token behaviour (checks readable, merge with Contents: write, SAML approval): untested; moot under decision 2.
- A token with exactly `repo` + `read:org`: only a broader token was available; minimums are from docs and the accepted-scopes header.
- Write actions on both providers were not exercised.
- No private repo with real check runs: proven on public `cli/cli`; empty list on a private repo.
- Bitbucket: nothing authenticated. Unverified: API-token scope names vs swagger names, whether `read:pullrequest` alone lists PRs, the 403 missing-scope body, 429 shape, 304 cost, reviewer `q=` filter.
- SAML org with classic token: needs "Authorize SSO"; `X-GitHub-SSO` not reproduced.

### Needs manual verification (owner, throwaway repo and workspace)
1. GitHub: create a classic token with only `repo` + `read:org` on a throwaway repo; paste it into Codepass or a keychain, not a file. Check `GET /user` shows `X-OAuth-Scopes: read:org, repo`; `GET /user/orgs` 200; create a PR, merge it, delete its branch; `GET /user/emails` should 404 with accepted scopes `user, user:email`. Revoke the token.
2. GitHub with a SAML org: authorize the token under Configure SSO; before that, confirm the 403 and `X-GitHub-SSO` header.
3. Bitbucket: create an API token with the five scopes in section 7. Check list repos, list PRs, create PR with `close_source_branch:true`, merge, decline. Remove `write:pullrequest`, confirm the 403 body and `x-accepted-oauth-scopes`. Repeat a conditional GET many times and watch whether the hour budget moves. Delete the token.

## 10. What the plan must change

product-plan.md
- 5.11 GitHub token line: replace the fine-grained discussion with "Classic token, scopes `repo` + `read:org` only (no `workflow`). Fine-grained tokens are not supported in 1.0." Drop "whether check runs can be read with a fine-grained token is a spike item".
- Same section: show checks from `check-runs` and the combined `status` (or GraphQL `statusCheckRollup`); `null` means "no checks".
- Bitbucket token line: replace "repository and pull request read and write" with `read:user`, `read:workspace`, `read:repository`, `read:pullrequest`, `write:pullrequest` (suffix `:bitbucket`), optional `read:pipeline`. Closing the source branch is the `close_source_branch` flag on create/merge under `write:pullrequest`. Add the HTTPS username note (`x-bitbucket-api-token-auth` or account username) for any HTTPS fallback.
- Errors: GitHub reports a missing classic scope as 404; compare `X-OAuth-Scopes` and `X-Accepted-OAuth-Scopes`. Bitbucket: 401 message and `x-accepted-oauth-scopes`. 401 on either means expired: degrade to "open in browser".
- Polling: GitHub uses ETag (304 free, measured) for per-repo list and detail; cross-repo "to review" uses search (30/min, no ETag) or GraphQL. Bitbucket personal tokens have about 1000 requests/hour, no remaining header and no cross-workspace endpoint: poll per repo, count calls locally, 15 min for repos off screen, back off on 429.
- Risk table: add "Bitbucket personal token limit 1000/h".

tasks.md
- S0-07: mark done with the manual items above open; the fine-grained Accept item is dropped by decision 2. Add manual steps 1 and 3 before P6-03 / P6-04 sign-off.
- P6-03: accept "a missing-scope 404 is reported as a scope problem using the two scope headers"; scopes `repo` + `read:org`; tests with recorded 401, 404-with-headers, secondary-limit 403, and 304.
- P6-04: accept "client counts its own requests, stays under 1000/h, passes `close_source_branch` on merge"; tests for 400/409/555, async 202 merge, and the 401 message.
- P6-07: "to review" must work without ETag; Bitbucket lists per entered workspace and repo.
- P6-08: token help text lists the exact scopes per provider; verify step calls `GET /user` and compares granted scopes.
