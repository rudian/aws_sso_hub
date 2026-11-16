package notify

import (
	"encoding/json"
	"strings"
	"testing"
)

func accts(names ...string) []Account {
	var out []Account
	for _, n := range names {
		out = append(out, Account{Name: n})
	}
	return out
}

// The texts are the ones of design 6.
func TestSignInTexts(t *testing.T) {
	cases := []struct {
		name        string
		lang        string
		in          SignInInfo
		ended       bool
		title, body string
	}{
		{"zh heads-up, three accounts, user", "zh-CN", SignInInfo{Portal: "acme", User: "alex", Accounts: accts("acme-prod", "globex", "initech"), LeadMinutes: 15},
			false, "登录快过期了：acme", "alex · acme-prod 等 3 个账户。大约 15 分钟后过期。"},
		{"en heads-up, three accounts, user", "en", SignInInfo{Portal: "acme", User: "alex", Accounts: accts("acme-prod", "globex", "initech"), LeadMinutes: 15},
			false, "Sign-in expires soon: acme", "alex · acme-prod and 2 more. Expires in about 15 minutes."},
		{"zh ended, three accounts, user", "zh-CN", SignInInfo{Portal: "acme", User: "alex", Accounts: accts("acme-prod", "globex", "initech")},
			true, "登录已过期：acme", "alex · acme-prod 等 3 个账户。重新登录后才能继续用。"},
		{"en ended, three accounts, user", "en", SignInInfo{Portal: "acme", User: "alex", Accounts: accts("acme-prod", "globex", "initech")},
			true, "Sign-in expired: acme", "alex · acme-prod and 2 more. Sign in again to keep using them."},
		{"one account, unknown user (zh)", "zh-CN", SignInInfo{Portal: "acme", Accounts: accts("acme-prod")}, true, "登录已过期：acme", "acme-prod。重新登录后才能继续用。"},
		{"one account, unknown user (en)", "en", SignInInfo{Portal: "acme", Accounts: accts("acme-prod")}, true, "Sign-in expired: acme", "acme-prod. Sign in again to keep using them."},
		{"two accounts (zh)", "zh-CN", SignInInfo{Portal: "acme", User: "alex", Accounts: accts("acme-prod", "globex"), LeadMinutes: 30}, false, "登录快过期了：acme", "alex · acme-prod、globex。大约 30 分钟后过期。"},
		{"two accounts (en)", "en", SignInInfo{Portal: "acme", User: "alex", Accounts: accts("acme-prod", "globex"), LeadMinutes: 30}, false, "Sign-in expires soon: acme", "alex · acme-prod and globex. Expires in about 30 minutes."},
		{"one hour (zh)", "zh-CN", SignInInfo{Portal: "acme", Accounts: accts("acme-prod"), LeadMinutes: 60}, false, "登录快过期了：acme", "acme-prod。大约 1 小时后过期。"},
		{"one hour (en)", "en", SignInInfo{Portal: "acme", Accounts: accts("acme-prod"), LeadMinutes: 60}, false, "Sign-in expires soon: acme", "acme-prod. Expires in about 1 hour."},
		{"no account yet: only the second sentence (en)", "en", SignInInfo{Portal: "acme", User: "alex"}, true, "Sign-in expired: acme", "Sign in again to keep using them."},
		{"no account yet: only the second sentence (zh)", "zh-CN", SignInInfo{Portal: "acme", User: "alex", LeadMinutes: 15}, false, "登录快过期了：acme", "大约 15 分钟后过期。"},
		{"the same account name twice counts once", "en", SignInInfo{Portal: "acme", Accounts: accts("acme-prod", "acme-prod", "globex")}, true, "Sign-in expired: acme", "acme-prod and globex. Sign in again to keep using them."},
	}
	for _, c := range cases {
		build := SignInHeadsUp
		if c.ended {
			build = SignInEnded
		}
		title, body := build(c.lang, c.in)
		if title != c.title || body != c.body {
			t.Errorf("%s:\n got  %q / %q\n want %q / %q", c.name, title, body, c.title, c.body)
		}
	}
}

func TestSignInNoticeShape(t *testing.T) {
	in := SignInInfo{Portal: "acme", Accounts: accts("acme-prod"), LeadMinutes: 15, SignInTime: "2026-11-16T01:00:00Z"}
	h := SignInHeadsUpNotice("en", in)
	e := SignInEndedNotice("en", in)
	if h.ID != e.ID || h.ID != "signin:acme" {
		t.Fatal("heads-up and ended share one id per portal, so on macOS ended replaces the heads-up")
	}
	if !h.Silent || e.Silent {
		t.Fatal("no sound for the heads-up, the default sound for ended")
	}
	if h.Kind != KindSignInHeadsUp || e.Kind != KindSignInEnded || e.Data[DataKind] != "signin.ended" {
		t.Fatalf("%+v %+v", h, e)
	}
	if SignInButton("zh-CN") != "重新登录" || SignInButton("en") != "Sign in again" {
		t.Fatal("button text")
	}
}

// Plan 4.15 "Text that never appears": a token, a URL or start-URL host, an account ID, a role name (and a file
// path, an e-mail address other than the user) never reach a title, a body or the data map.
func TestSecretsNeverReachATitleABodyOrTheDataMap(t *testing.T) {
	const (
		token    = "eyJraWQiOiJhYmNkZWYiLCJhbGciOiJSUzI1NiJ9.AAAABBBBCCCCDDDD"
		startURL = "https://d-1234567890.awsapps.com/start"
		host     = "d-1234567890.awsapps.com"
		acctID   = "111122223333"
		role     = "AdministratorAccess"
	)
	forbidden := []string{token, startURL, host, "awsapps", acctID, role, "AWSReservedSSO", "/Users/", "@globex.example.com", "https://"}
	in := SignInInfo{
		Portal:      "acme",
		StartURL:    startURL,
		User:        "alex@example.com",
		LeadMinutes: 15,
		SignInTime:  "2026-11-16T01:00:00Z",
		Accounts: []Account{
			{Name: "acme-prod", AccountID: acctID, Role: role},
			{Name: acctID, AccountID: "444455556666", Role: role},                      // a name that is the account ID
			{Name: "acme-prod-" + role, AccountID: "777788889999", Role: role},         // a name that carries the role
			{Name: "AWSReservedSSO_" + role + "_0123abcd", AccountID: "1", Role: role}, // a role's generated name
			{Name: startURL, AccountID: "2"},                                           // a name that is the sign-in URL
			{Name: host, AccountID: "3"},                                               // ... or its host
			{Name: token, AccountID: "4"},                                              // ... or a token
			{Name: "~/work/acme", AccountID: "5"},                                      // ... or a path
			{Name: "ops@globex.example.com", AccountID: "6"},                           // ... or somebody's address
			{Name: "globex", AccountID: "7", Role: role},
		},
	}
	check := func(what string, texts ...string) {
		for _, tx := range texts {
			for _, f := range forbidden {
				if strings.Contains(tx, f) {
					t.Errorf("%s contains %q: %q", what, f, tx)
				}
			}
		}
	}
	for _, lang := range []string{"zh-CN", "en"} {
		th, bh := SignInHeadsUp(lang, in)
		te, be := SignInEnded(lang, in)
		check("heads-up "+lang, th, bh)
		check("ended "+lang, te, be)
		// only the two clean names survive
		if !strings.Contains(be, "acme-prod") || !strings.Contains(be, "globex") || strings.Contains(be, "3") {
			t.Errorf("the safe names remain and nothing else is counted: %q", be)
		}
		for _, n := range []Notice{SignInHeadsUpNotice(lang, in), SignInEndedNotice(lang, in)} {
			raw, _ := json.Marshal(n.Data)
			check("data map", string(raw))
			check("notice", n.Title, n.Body, n.ID)
			if len(n.Data) != 3 {
				t.Errorf("the data map holds three keys: %v", n.Data)
			}
			for k := range n.Data {
				if k != DataKind && k != DataPortal && k != DataTime {
					t.Errorf("unexpected key %q", k)
				}
			}
		}
	}
	// the user name itself may be an e-mail address (it is the Identity Center user), but never a URL or a token
	_, body := SignInEnded("en", SignInInfo{Portal: "acme", User: startURL, Accounts: accts("globex")})
	check("a user that is a URL", body)
	_, body = SignInEnded("en", SignInInfo{Portal: "acme", User: token, Accounts: accts("globex")})
	check("a user that is a token", body)
	// a portal name that is a secret does not appear in the title
	title, _ := SignInEnded("en", SignInInfo{Portal: startURL, StartURL: startURL})
	check("a portal that is a URL", title)
	if title != "Sign-in expired" {
		t.Errorf("title %q", title)
	}
}

// A name that merely contains a short role-like word stays: only whole names and long secrets are dropped.
func TestAnOrdinaryNameIsKept(t *testing.T) {
	in := SignInInfo{Portal: "acme", Accounts: []Account{{Name: "Admin Console tools", AccountID: "111122223333", Role: "Admin"}}}
	if _, body := SignInEnded("en", in); !strings.HasPrefix(body, "Admin Console tools.") {
		t.Fatal(body)
	}
}

// A portal named after its directory ID, and a long but legal account name, are names and are kept.
func TestDirectoryIDPortalAndLongAccountNamesAreKept(t *testing.T) {
	long := "acme-production-data-platform-eu1" // 33 characters
	in := SignInInfo{Portal: "d-1234567890", StartURL: "https://d-1234567890.awsapps.com/start", Accounts: []Account{{Name: long, AccountID: "111122223333", Role: "Admin"}, {Name: "globex"}}}
	for _, lang := range []string{"en", "zh-CN"} {
		title, body := SignInEnded(lang, in)
		if !strings.Contains(title, "d-1234567890") || !strings.Contains(body, long) || !strings.Contains(body, "globex") {
			t.Errorf("%s: %q / %q", lang, title, body)
		}
	}
	// the start URL's host still never appears, nor a token
	if title, _ := SignInEnded("en", SignInInfo{Portal: "d-1234567890.awsapps.com", StartURL: in.StartURL}); strings.Contains(title, "awsapps") {
		t.Errorf("%q", title)
	}
	if _, body := SignInEnded("en", SignInInfo{Portal: "acme", Accounts: []Account{{Name: strings.Repeat("a", 65)}}}); strings.Contains(body, "aaaa") {
		t.Errorf("%q", body)
	}
}
