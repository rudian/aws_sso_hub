# Security policy

## Supported versions

| Version | Supported |
| --- | --- |
| 0.0.x | Yes |

Only the latest 0.0.x release receives fixes.

## Reporting a vulnerability

Please do not report security problems in a public issue or discussion.

Use GitHub's private vulnerability reporting instead: open the **Security** tab of
https://github.com/rudian/aws_sso_hub, choose **Report a vulnerability**, and describe the problem. Helpful details:

- the AWS SSO Hub version and your operating system,
- what you expected and what happened, with the smallest steps that reproduce it,
- the impact you see (for example: a secret ends up somewhere it should not, a command can be injected, a file outside
  the expected folders can be read or written).

Please do not include real account IDs, access keys, tokens or session credentials in the report; replace them with
placeholders.

You will get an answer as soon as a maintainer can look at it. Please allow time for a fix and a release before
disclosing details publicly.

## Scope

AWS SSO Hub is a desktop app that edits the AWS CLI and Git configuration of the signed-in user and talks only to
AWS endpoints and the user's own Git remotes. Reports about behaviour of the AWS CLI, Git, AWS services or the
operating system themselves belong to those projects.
