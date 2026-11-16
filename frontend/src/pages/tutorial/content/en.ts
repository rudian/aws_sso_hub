import type { TutorialSection } from './types'

// English tutorial. The running example: two companies, "acme" (account 111122223333) and "globex" (account 444455556666),
// each with its own AWS organization and its own IAM Identity Center portal. A third portal, "initech", stands for an
// organization that already uses Identity Center.
//
// Console labels are AWS's own wording and AWS changes them from time to time. Steps therefore also describe the place
// in words, and the breadcrumbs are a hint rather than an exact path.
export const en: TutorialSection[] = [
  {
    id: 'why',
    group: 'admin',
    minutes: 3,
    title: 'What SSO is, and why switch',
    lead: 'Access keys are long-lived: whoever copies one can use it until someone revokes it. SSO signs you in through the browser and hands out short-lived credentials that stop working on their own.',
    steps: [
      {
        title: 'What an access key is',
        text: 'A pair of values stored in `~/.aws/credentials`. It works from any computer, at any time, until it is deactivated. A copy in a backup, a chat message or a repository works just as well as the original.',
      },
      {
        title: 'What SSO changes',
        text: 'You sign in with your browser about once per session (the AWS default is **8 hours**; an administrator can set anything from 15 minutes to 90 days). The AWS CLI and Git then receive short-lived credentials from that sign-in. No long-term secret stays on disk.',
      },
      {
        title: 'What stays the same',
        text: 'Profile names, `aws --profile acme ...` commands and repositories that are bound to a profile keep working. When AWS SSO Hub migrates a profile it **keeps the profile name**.',
      },
      {
        title: 'What the app does to your files',
        text: 'Before it changes `~/.aws/config`, `~/.aws/credentials` or a repository\'s `.git/config`, AWS SSO Hub saves a backup (Settings, Backups). Backups never contain key values. Every change is shown as a diff first.',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: 'Nothing changes until you confirm each step',
        body: 'Opening the app only reads your files. Each change has a preview, a backup and a confirm button.',
      },
    ],
  },
  {
    id: 'enable',
    group: 'admin',
    minutes: 10,
    title: 'Turn on IAM Identity Center',
    lead: 'IAM Identity Center is the AWS service behind the sign-in page. It is turned on once per AWS organization, by someone who can sign in to that organization\'s management account. In this guide each company (acme, globex) has its own organization and its own portal. Do these steps once per company.',
    steps: [
      {
        title: 'Sign in to the company\'s AWS account',
        text: 'Use the account that will own the setup, for example acme (account `111122223333`). You need an administrator-level user or the root user.',
      },
      {
        title: 'Check where the account stands',
        crumbs: ['AWS Organizations'],
        text: 'Open **AWS Organizations** from the console search. If it offers to create an organization, the account is standalone. If it shows this account as the management account, the organization already exists. If it shows the account as a member of another organization, you cannot enable Identity Center here: ask that organization\'s administrator to give you access (see the FAQ).',
        code: { label: 'Or check from a terminal (read-only)', text: 'aws organizations describe-organization --profile acme' },
      },
      {
        title: 'Create the organization (standalone accounts only)',
        text: 'Choose **Create an organization**. A standalone account becomes the management account of a new organization, and nothing else about the account changes. Creating an organization on a free-tier account can upgrade it to a paid plan.',
      },
      {
        title: 'Enable IAM Identity Center',
        crumbs: ['IAM Identity Center', 'Enable'],
        text: 'Search for **IAM Identity Center**. **Pick the region first** in the region selector at the top right: people will sign in through that region, it does not have to be where your workloads run, and changing it later means deleting and re-creating the instance. Then choose **Enable** and, when asked, create an **organization instance**.',
      },
      {
        title: 'Keep the default identity source',
        crumbs: ['Settings', 'Identity source'],
        text: 'The built-in **Identity Center directory** is the simplest choice: users and groups live inside Identity Center. You can connect an external identity provider later.',
      },
      {
        title: 'Add yourself and your teammates',
        crumbs: ['Users', 'Add user'],
        text: 'In the left menu open the list of users and add one for each person: a user name, an email address, and a name. Choose to send an email invitation. Optionally create a group such as `Developers` and add the users to it, so that access is assigned to the group.',
      },
      {
        title: 'Require MFA',
        crumbs: ['Settings', 'Authentication', 'Multi-factor authentication'],
        text: 'In the authentication settings, configure multi-factor authentication to prompt for a code **every time users sign in**, allow authenticator apps and security keys, and let users without a device register one at sign-in.',
      },
      {
        title: 'Decide how long a sign-in lasts',
        crumbs: ['Settings', 'Authentication', 'Session duration'],
        text: 'The setting for **user interactive sessions** is how often people have to sign in again. The default is 8 hours; longer means fewer sign-ins, shorter is safer. AWS SSO Hub cannot read this value, so it works out when each session ends from what AWS returns. The first sign-in may show 8 hours until AWS reveals the end; a session of an hour or less is detected within about two minutes.',
      },
    ],
    callouts: [
      {
        kind: 'warn',
        title: 'Use an organization instance, not an account instance',
        body: 'An account instance only supports applications. AWS states that account instances do not support permission sets and therefore do not support access to AWS accounts. It cannot be converted into an organization instance.',
      },
      {
        kind: 'info',
        title: 'Two companies, two portals',
        body: 'acme and globex each get their own organization and portal, so AWS SSO Hub shows two portals with their own sign-in button and countdown. If the companies merge later, you can move both accounts into one organization and use a single portal.',
      },
    ],
  },
  {
    id: 'assign',
    group: 'admin',
    minutes: 10,
    title: 'Give people access to the account',
    lead: 'A permission set is a role that people can pick after they sign in. You create one, assign it to users or groups for the account, and then pass the portal address on to the developers.',
    steps: [
      {
        title: 'Create a permission set',
        crumbs: ['Multi-account permissions', 'Permission sets', 'Create permission set'],
        text: 'In the left menu, in the part that manages access to AWS accounts, open the permission sets and create one. Pick a predefined set such as **AdministratorAccess** (administrators) or **PowerUserAccess** (developers), or, when people only need Git, a custom set with the AWS managed policy `AWSCodeCommitPowerUser`. Set the **session duration** (default 1 hour, up to 12 hours); 8 to 12 hours suits Git work.',
      },
      {
        title: 'Optional: let the app deactivate the old key',
        text: 'Migrating a profile ends with deactivating its old access key in IAM. **AdministratorAccess** can do that in one click from the app. **PowerUserAccess** and `AWSCodeCommitPowerUser` cannot, because they do not manage IAM. To allow it, add this inline policy to the permission set, with the old IAM user\'s name. Without it, the app shows console steps instead.',
        code: {
          label: 'Inline policy (replace the account ID and user name)',
          text: '{\n  "Version": "2012-10-17",\n  "Statement": [\n    {\n      "Effect": "Allow",\n      "Action": [\n        "iam:ListAccessKeys",\n        "iam:GetAccessKeyLastUsed",\n        "iam:UpdateAccessKey"\n      ],\n      "Resource": "arn:aws:iam::111122223333:user/acme-dev"\n    }\n  ]\n}',
        },
      },
      {
        title: 'Assign the account to people',
        crumbs: ['Multi-account permissions', 'AWS accounts', 'Assign users or groups'],
        text: 'Open the list of AWS accounts in your organization and tick the account (acme). Choose **Assign users or groups**, pick the users or the group, then pick the permission set and submit. Wait until provisioning finishes before testing.',
      },
      {
        title: 'Each person accepts the invitation',
        text: 'Everyone follows the email invitation, sets a password and registers an MFA device. Then they open the portal once in a browser and check that the account and the role appear.',
      },
      {
        title: 'Copy the portal address and region',
        crumbs: ['Dashboard', 'Settings summary'],
        text: 'On the Identity Center dashboard, in the settings summary, copy the **AWS access portal URL**. Note the region you chose in the enable step: you will enter both in AWS SSO Hub. You can customise the subdomain once; the default looks like this.',
        code: { label: 'What the portal address looks like', text: 'https://d-xxxxxxxxxx.awsapps.com/start' },
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: 'Which organization owns the account?',
        body: '**Standalone account:** enable an organization instance in that account (previous section). It creates a new organization.\n**Management account of its own organization:** enable Identity Center there.\n**Member of an organization you control:** use that organization\'s Identity Center and assign the account there.\n**Member of an organization you do not control:** ask its administrator for a permission set, or have the account leave that organization first.',
      },
      {
        kind: 'warn',
        title: 'Account instances cannot grant access to AWS accounts',
        body: 'If Identity Center was enabled in a single account as an account instance, the portal will list no AWS accounts. Enable an organization instance instead.',
      },
    ],
  },
  {
    id: 'portal',
    group: 'dev',
    minutes: 3,
    title: 'Add the portal and sign in',
    lead: 'A portal is one Identity Center sign-in page. Add it once, sign in, and AWS SSO Hub creates a profile for each account and role you pick. You need the portal URL and region from the previous section.',
    steps: [
      {
        title: 'Start the Add SSO portal dialog',
        crumbs: ['Accounts', 'Add SSO portal'],
        text: 'Open **Accounts** and choose **Add SSO portal**.',
      },
      {
        title: 'Enter the portal details',
        text: 'Paste the **portal URL**, choose the **SSO region** (the region where Identity Center lives), and give the portal a short name that uses letters, digits, `-`, `_` or `.`, for example `acme`.',
      },
      {
        title: 'Sign in',
        text: 'Choose **Sign in and continue**. Your browser opens; sign in and choose **Allow access**. The app continues on its own. If the browser cannot open or cannot reach the app, use the device code option.',
      },
      {
        title: 'Pick accounts and roles',
        text: 'Tick the roles you want as profiles and name them, for example `acme-admin`. Each profile gets a colour that belongs to the AWS account, so profiles of one account look alike.',
      },
      {
        title: 'Review and save',
        text: 'The dialog shows the exact change to `~/.aws/config`. Saving makes a backup first. Afterwards each portal card shows a countdown for how long the sign-in lasts.',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: 'Migrating an access-key profile?',
        body: 'Skip this section. The migration wizard adds the portal for you and keeps the profile name.',
      },
    ],
  },
  {
    id: 'migrate',
    group: 'dev',
    minutes: 5,
    title: 'Migrate access keys to SSO',
    lead: 'A migrated profile keeps its name. Commands with `--profile acme` and every repository bound to `acme` keep working; they simply use SSO. Because acme and globex have their own organizations, each gets its own new portal. Repeat these steps for each profile.',
    steps: [
      {
        title: 'Start the migration',
        crumbs: ['Accounts'],
        text: 'In **Accounts**, profiles that still use access keys are listed in a separate panel. On the `acme` card, choose **Migrate to SSO**.',
      },
      {
        title: 'Check who owns the key',
        text: 'The app asks AWS "who am I" with this key. It shows the account (`111122223333`), the IAM user and the masked key ID. Nothing is changed. If the key is a root user key, you can still migrate but must deactivate it from the console later.',
      },
      {
        title: 'Choose the portal and sign in',
        text: 'When the account is not part of any portal you already added, the wizard pre-selects **Add a new portal**: paste the portal URL and region from the previous sections, then sign in. If an existing portal lists the account, that portal is pre-selected instead.',
      },
      {
        title: 'Pick the same account and a role',
        text: 'The account that matches the key is pre-selected and marked. Choose the role; the profile\'s region stays as it was.',
      },
      {
        title: 'Review, save and verify',
        text: 'Check the diff of `~/.aws/config`: only `[profile acme]` changes and a `[sso-session acme]` block is added. After saving, the app confirms the identity **using SSO role credentials, not the old key**, and checks that the account matches.',
      },
      {
        title: 'Remove the old key from this computer',
        text: 'Once the new setup works, delete the `[acme]` block from `~/.aws/credentials`. The backup that is made first does not contain key values. You can also choose **Keep it for now**; the profile then shows "Old key still on disk".',
      },
      {
        title: 'Deactivate the old key in IAM',
        text: 'The key is gone from this computer but still active in AWS. The app tries, in order: your new SSO role, then the old key itself while it is still in memory, then guides you through the IAM console. If you restart the app between removing and deactivating, only the new role and the console remain.',
      },
    ],
    callouts: [
      {
        kind: 'warn',
        title: 'Deactivate first, delete later',
        body: 'Deactivating a key can be undone. Delete it in IAM only after a week without problems, once nothing else (CI jobs, servers) turned out to use it.',
      },
      {
        kind: 'info',
        title: 'Next',
        body: 'Repeat for `globex` (account `444455556666`). It gets its own portal.',
      },
    ],
  },
  {
    id: 'bind',
    group: 'dev',
    minutes: 5,
    title: 'Bind repositories to accounts',
    lead: 'A binding tells one repository which profile to use for CodeCommit. It is stored in that repository\'s own `.git/config`, so two clones can use two accounts and nothing else on the computer changes.',
    steps: [
      {
        title: 'Open the repositories that need attention',
        crumbs: ['Repositories', 'To fix'],
        text: 'Open **Repositories** and look at the **To fix** filter. Each row shows which account would be used if you pushed right now, and the app looks for the account that owns each repository.',
      },
      {
        title: 'Bind what the app suggests',
        text: 'Choose **Bind** on a row, or **Bind all suggestions** to confirm several at once. To add a repository that is outside the scanned folders, use **Add existing repo**: it detects the account and, for an SSO account, can bind it for you.',
      },
      {
        title: 'Resolve repositories with the same name in two accounts',
        text: 'When two accounts have a repository with the same name, the row shows both. **Compare commits** checks which one matches your local latest commit.',
      },
      {
        title: 'Switch SSH remotes to HTTPS',
        text: 'Only HTTPS remotes can use SSO. **Switch to HTTPS** changes `origin` and binds it in one go, after a backup of `.git/config`.',
      },
      {
        title: 'Remove the global default',
        crumbs: ['Git global settings'],
        text: 'If a global Git setting makes one account the default for every CodeCommit repository, remove it once everything is bound: an unbound repository then fails with a clear message instead of quietly using the wrong account. URL rules must be turned into local bindings first.',
        code: {
          label: 'What Git prints for an unbound repository',
          text: 'AWS SSO Hub: this repository is not bound to an AWS account (no CodeCommit credential helper). Open AWS SSO Hub > Repositories and bind it.',
        },
      },
      {
        title: 'Test the connection',
        text: 'Use **Test connection** on a row. It runs a read-only check against the remote and tells you whether the account is signed in and has access.',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: 'What a binding contains',
        body: 'A section for the CodeCommit host with the full path of the AWS CLI, the profile name and a fallback that explains what to do when you are signed out. Other remotes of the same repository, such as GitHub, are left alone.',
      },
    ],
  },
  {
    id: 'daily',
    group: 'dev',
    minutes: 2,
    title: 'Your daily routine',
    lead: 'Sign in once, then work as usual. Git and the AWS CLI use the same sign-in as the app.',
    steps: [
      {
        title: 'Sign in',
        text: 'Open AWS SSO Hub and choose **SSO sign-in** on the portal card, or click the portal in the sidebar. One sign-in covers every profile of that portal. Approve it in the browser.',
      },
      {
        title: 'Work as usual',
        text: '`git pull` and `git push` in bound repositories, and `aws ... --profile acme` in a terminal, just work.',
      },
      {
        title: 'Watch the countdown',
        text: 'The card shows how long the sign-in lasts (the default session is 8 hours). The app renews the short-lived access token by itself until the session ends. After that, choose **SSO sign-in** again. Quitting the app does not sign you out.',
      },
      {
        title: 'Use a terminal',
        text: 'The Terminal page generates these commands for the profile you pick. Signing in from a terminal works too, and the app sees it.',
        code: {
          label: 'In any terminal',
          text: 'export AWS_PROFILE=acme-admin\naws sso login --sso-session acme\naws sts get-caller-identity',
        },
      },
      {
        title: 'Check the colour before you push',
        text: 'Every repository row and every card use the colour of its AWS account. If the chip is not the colour you expect, check the binding.',
      },
    ],
    callouts: [
      {
        kind: 'info',
        title: 'Signing out and Git',
        body: 'Signing out removes the sign-in and the cached role credentials, but credentials that were already handed out stay valid on the AWS side until they expire (up to the permission set\'s session length).',
      },
    ],
  },
  {
    id: 'faq',
    group: 'dev',
    minutes: 5,
    title: 'FAQ and troubleshooting',
    lead: 'Install the AWS CLI, open the app when your system warns about it, and the usual error messages.',
    steps: [
      {
        title: 'Install the AWS CLI v2',
        text: 'Signing in works without it, but binding repositories and Git pushes need AWS CLI v2, version 2.22.0 or later. **macOS:** use Homebrew, or the official installer package from the AWS documentation. **Windows:** use the official MSI installer. Then choose **Check again** on the Health check page.',
        code: { label: 'macOS with Homebrew', text: 'brew install awscli\naws --version' },
      },
      {
        title: 'Open the app when the system warns about it',
        text: 'The release builds are not signed. **macOS:** right-click the app and choose **Open**, or remove the quarantine flag. **Windows:** in the SmartScreen dialog choose **More info**, then **Run anyway**.',
        code: { label: 'macOS, once', text: 'xattr -dr com.apple.quarantine "/Applications/AWS SSO Hub.app"' },
      },
    ],
    faq: [
      {
        q: 'Switch repos from the acme key to SSO',
        a: '**Path 1, migrate in place (recommended).** Migrate the profile with the wizard. The profile keeps its name, so every repository bound to `acme`, and every repository that follows the global default `--profile acme`, keeps working through SSO without any change. Then remove the old key and deactivate it.\n**Path 2, a new profile name.** Create a new SSO profile (for example `acme-sso`), then use **Move repos to…** on the old profile\'s card to move its repositories before you delete `acme`.\n**Repositories that are not listed yet:** use **Add existing repo** on the Repositories page. The app finds the owning account, prefers an SSO profile, and binds the repository when exactly one account matches.',
      },
      {
        q: 'Git asks me for a username and password.',
        a: 'The repository is not bound, or an old password is saved in the keychain or Credential Manager. Bind the repository, then clear saved passwords on the Git global settings page.',
      },
      {
        q: 'I pushed to the wrong account.',
        a: 'Check the account chip of the repository. A global default or a URL rule can catch the wrong clone. Bind the repository locally and use **Test connection**.',
      },
      {
        q: 'The browser did not open, or cannot reach 127.0.0.1.',
        a: 'Use the device code option in the sign-in dialog. This also helps on remote desktops and with locked-down browsers.',
      },
      {
        q: 'The AWS CLI says the token has expired, or the token does not exist.',
        a: 'Sign in again from the portal card. The app and the CLI share the same sign-in. `Token has expired and refresh failed` means the portal session ended or was revoked.',
      },
      {
        q: 'The authorization was not approved or timed out.',
        a: 'The browser page was not confirmed within 10 minutes, or **Deny** was chosen. Try again.',
      },
      {
        q: '`aws --profile acme` still uses the old key.',
        a: 'Environment variables win over profiles. Open the Health check page: it lists shell startup files and environment variables that export `AWS_ACCESS_KEY_ID`. The app never edits those files; remove the lines yourself.',
      },
      {
        q: 'I get a 403 on git push.',
        a: 'The repository is bound to the wrong account, or the role has no CodeCommit write permission. Check the colour of the binding, use **Test connection**, and ask the administrator for a permission set that includes CodeCommit.',
      },
      {
        q: 'The portal shows no accounts.',
        a: 'No permission set is assigned to your user yet, or provisioning is still running. Ask the administrator (see "Give people access to the account"). If Identity Center was enabled as an account instance, it cannot list AWS accounts.',
      },
      {
        q: 'Git still works after I signed out.',
        a: 'Credentials that were already issued stay valid until they expire. AWS SSO Hub deletes the cached role credentials when you sign out, and AWS expires the rest within the permission set\'s session length.',
      },
      {
        q: 'The AWS CLI is too old.',
        a: 'Portal sign-in needs 2.9.0, and the browser-based flow that the AWS CLI uses by default needs 2.22.0 or later. Upgrade it (see "Install the AWS CLI v2").',
      },
      {
        q: 'I see clock skew errors.',
        a: 'Synchronise the system clock, then try again.',
      },
      {
        q: 'Where are the backups?',
        a: 'Settings, Backups. Every change to `~/.aws/config`, `~/.aws/credentials`, the global Git config or a repository\'s `.git/config` makes one first. Credential backups never contain key values.',
      },
      {
        q: 'My AWS account is a member of someone else\'s organization.',
        a: 'Identity Center belongs to the organization. Ask the management account\'s administrator to assign you a permission set on your account, or have the account leave that organization and create its own.',
      },
      {
        q: 'Can I share my setup with a teammate or another computer?',
        a: 'Yes. Settings, Accounts, **Export accounts…** saves the portals and SSO profiles as a JSON file without any key or token. **Import accounts…** reads such a file after a preview. Repository bindings are specific to a computer: scan, then use **Bind all suggestions**.',
      },
    ],
  },
]
