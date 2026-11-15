package gitx

import "testing"

const ccHost = "git-codecommit.us-east-1.amazonaws.com"

func TestMatchesTable(t *testing.T) {
	base := CredURL{Protocol: "https", Host: ccHost, Path: "v1/repos/acme_cs_portal"}
	withUser := base
	withUser.User = "user"
	cases := []struct {
		id, pattern string
		u           CredURL
		want        bool
	}{
		{"P1", "https://" + ccHost, base, true},
		{"P2", "https://" + ccHost + "/", base, true},
		{"P3", "https://" + ccHost + "/v1/repos", base, true},
		{"P4", "https://" + ccHost + "/v1/repos/", base, true},
		{"P5", "https://" + ccHost + "/v1/repos/acme_cs_portal", base, true},
		{"P5b", "https://" + ccHost + "/v1/repos/acme_cs_portal", CredURL{Protocol: "https", Host: ccHost, Path: "v1/repos/acme_cs_portal-v4"}, false},
		{"P6", "https://" + ccHost + "/v1/repos/acme_cs_portal/", base, true},
		{"P7", "https://" + ccHost + "/v1/repos/acme_cs_portal.git", base, false},
		{"P8", "https://" + ccHost + "/v1/repos/web-ap", base, false},
		{"P9", "https://*.us-east-1.amazonaws.com", base, true},
		{"P10", "https://git-codecommit.*.amazonaws.com", base, true},
		{"P11", "https://*.amazonaws.com", base, false},
		{"P12", ccHost, base, true},
		{"P13 user given", "https://user@" + ccHost, withUser, true},
		{"P13 no user in credential", "https://user@" + ccHost, base, false},
		{"P14", "https://other@" + ccHost, withUser, false},
		{"P15", "https://" + ccHost + ":443", base, true},
		{"P16", "https://" + ccHost + ":8443", base, false},
		{"P17", "http://" + ccHost, base, false},
		{"P18", "https://GIT-CODECOMMIT.us-east-1.amazonaws.com", base, true},
		{"P19", "https://" + ccHost + "/V1/repos", base, false},
		{"P20", "https://", base, true},
		{"P21", "https://" + ccHost + "/v1/repos/acme_cs_portal?x=1", base, false},
		{"P22", "https://" + ccHost + "/v1/%72epos", base, true},
		// partial (scheme-less) patterns: exact equality of every field present
		{"partial path prefix no", ccHost + "/v1/repos", base, false},
		{"partial path exact", ccHost + "/v1/repos/acme_cs_portal", base, true},
		{"partial wildcard no", "*.us-east-1.amazonaws.com", base, false},
		{"partial host:port no", ccHost + ":443", base, false},
		{"partial user matches", "user@" + ccHost, withUser, true},
		{"partial user absent", "user@" + ccHost, base, false},
		{"explicit port 443 in url", "https://" + ccHost, CredURL{Protocol: "https", Host: ccHost, Port: "443", Path: "v1/repos/x"}, true},
	}
	for _, c := range cases {
		if got := Matches(c.pattern, c.u); got != c.want {
			t.Errorf("%s: Matches(%q) = %v, want %v", c.id, c.pattern, got, c.want)
		}
	}
}

func TestParseCredURL(t *testing.T) {
	u, err := ParseCredURL("HTTPS://user:pw@" + ccHost + ":443/v1/%72epos/x?q=1#f")
	if err != nil {
		t.Fatal(err)
	}
	want := CredURL{Protocol: "https", Host: ccHost, Port: "443", Path: "v1/repos/x", User: "user"}
	if u != want {
		t.Errorf("got %+v want %+v", u, want)
	}
	if _, err := ParseCredURL("/tmp/x"); err == nil {
		t.Error("expected error for a path")
	}
}

func TestParseNullConfig(t *testing.T) {
	out := []byte("global\x00file:/h/.gitconfig\x00credential.helper\n\x00" +
		"global\x00file:/h/.gitconfig\x00credential.https://git-codecommit.us-east-1.amazonaws.com/v1/repos/x.helper\n!aws --profile a\x00" +
		"local\x00file:.git/config\x00credential.bare\x00" +
		"local\x00file:.git/config\x00awsssohub.profile\nacme\x00")
	es, err := ParseNullConfig(out, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 4 {
		t.Fatalf("len = %d", len(es))
	}
	if e := es[0]; e.Scope != "global" || e.Origin != "/h/.gitconfig" || e.Key != "credential.helper" || e.Value != "" || e.NoValue || e.Subsection != "" || e.Var != "helper" {
		t.Errorf("entry0 %+v", e)
	}
	if e := es[1]; e.Subsection != "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/x" || e.Var != "helper" || e.Value != "!aws --profile a" {
		t.Errorf("entry1 %+v", e)
	}
	if e := es[2]; !e.NoValue || e.Var != "bare" {
		t.Errorf("entry2 %+v", e)
	}
	if e := es[3]; e.Key != "awsssohub.profile" || e.Value != "acme" || e.Subsection != "" {
		t.Errorf("entry3 %+v", e)
	}
	// without scope
	es, err = ParseNullConfig([]byte("credential.helper\n\x00credential.usehttppath\ntrue\x00credential.flag\x00"), false)
	if err != nil || len(es) != 3 || !es[2].NoValue || es[1].Value != "true" {
		t.Errorf("without scope: %+v %v", es, err)
	}
	if es, err := ParseNullConfig(nil, true); err != nil || es == nil || len(es) != 0 {
		t.Errorf("empty: %v %v", es, err)
	}
	if _, err := ParseNullConfig([]byte("garbage"), true); err == nil {
		t.Error("expected error for non-NUL-terminated output")
	}
}

func TestResolveResetSemantics(t *testing.T) {
	u := CredURL{Protocol: "https", Host: ccHost, Path: "v1/repos/initech_infra"}
	mk := func(scope, sub, v string, val string) Entry {
		key := "credential." + v
		if sub != "" {
			key = "credential." + sub + "." + v
		}
		return Entry{Scope: scope, Origin: "o", Key: key, Value: val, Subsection: sub, Var: v}
	}
	entries := []Entry{
		mk("system", "", "helper", "osxkeychain"),
		mk("global", "", "helper", ""),
		mk("global", "", "helper", "!aws codecommit credential-helper $@ --profile acme"),
		mk("global", "", "usehttppath", "true"),
		mk("global", "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/initech_infra", "helper", ""),
		mk("global", "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/initech_infra", "helper", "!aws --profile initech-admin codecommit credential-helper $@"),
		mk("global", "https://github.com", "helper", "gh"),
	}
	eff := Resolve(entries, u)
	if len(eff.Helpers) != 1 || eff.Helpers[0].Value != "!aws --profile initech-admin codecommit credential-helper $@" {
		t.Fatalf("URL rule must win for its repo: %+v", eff.Helpers)
	}
	if eff.ResetScope != "global" || eff.ResetSubsection == "" || !eff.UseHTTPPath {
		t.Errorf("reset/usehttppath: %+v", eff)
	}
	other := CredURL{Protocol: "https", Host: ccHost, Path: "v1/repos/other"}
	eff = Resolve(entries, other)
	if len(eff.Helpers) != 1 || eff.Helpers[0].Value != "!aws codecommit credential-helper $@ --profile acme" || eff.ResetSubsection != "" {
		t.Errorf("global default for others: %+v", eff)
	}
	// local binding without a reset appends after the global URL rule (c3)
	entries = append(entries, mk("local", "", "helper", "LOCAL"))
	eff = Resolve(entries, u)
	if len(eff.Helpers) != 2 || eff.Helpers[1].Value != "LOCAL" {
		t.Errorf("c3: %+v", eff.Helpers)
	}
	// github is untouched by codecommit sections
	gh := Resolve(entries, CredURL{Protocol: "https", Host: "github.com", Path: "o/r"})
	if len(gh.Helpers) != 2 || gh.Helpers[0].Value != "gh" {
		// system osxkeychain reset by global plain reset; plain local LOCAL applies to github too (g1)
		t.Logf("github helpers: %+v", gh.Helpers)
	}
}
