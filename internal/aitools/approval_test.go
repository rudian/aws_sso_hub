package aitools

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClaudeApproval(t *testing.T) {
	for _, m := range ClaudeModes {
		if a := ClaudeApproval(m); !a.Known || a.Mode != m || a.Source != "hook" {
			t.Errorf("%s: %+v", m, a)
		}
	}
	if a := ClaudeApproval("default"); a.Mode != "manual" {
		t.Fatalf("%+v", a)
	}
	for _, m := range []string{"", "yolo", "BypassPermissions"} {
		if a := ClaudeApproval(m); a.Known || a.Mode != "" {
			t.Errorf("%q: %+v", m, a)
		}
	}
}

func TestCodexApprovalReadsTopLevelOnly(t *testing.T) {
	d := t.TempDir()
	p := write(t, d, "config.toml", `# my config
model = "gpt-x"
approval_policy = "on-request"   # comment
sandbox_mode = 'workspace-write'
notify = ["/usr/bin/say", "done"]

[profiles.fast]
approval_policy = "never"
sandbox_mode = "danger-full-access"
`)
	a := CodexApproval(p)
	if !a.Known || a.Approval != "on-request" || a.Sandbox != "workspace-write" || a.Source != "config" || a.Kind != "approval-sandbox" {
		t.Fatalf("%+v", a)
	}
	if !CodexHasNotify(p) {
		t.Fatal("the user's own notify must be seen")
	}
	p2 := write(t, d, "c2.toml", "model = \"x\"\n[mcp]\nnotify = [\"a\"]\n")
	if a := CodexApproval(p2); a.Known || CodexHasNotify(p2) {
		t.Fatalf("%+v", a)
	}
	if a := CodexApproval(filepath.Join(d, "missing.toml")); a.Known {
		t.Fatal("a missing file is unknown")
	}
	p3 := write(t, d, "c3.toml", "approval_policy = \"bogus\"\nsandbox_mode = \"read-only\"\n")
	if a := CodexApproval(p3); a.Approval != "" || a.Sandbox != "read-only" || !a.Known {
		t.Fatalf("%+v", a)
	}
}

func TestOpenCodeApproval(t *testing.T) {
	d := t.TempDir()
	cases := []struct {
		body, want string
		known      bool
	}{
		{`{"permission":"allow"}`, "allow", true},
		{`{"permission":{"edit":"ask","bash":"ask"}}`, "ask", true},
		{`{"permission":{"edit":"allow","bash":"ask"}}`, "mixed", true},
		{`{"permission":{"bash":{"git *":"allow","*":"deny"}}}`, "mixed", true},
		{`{"permissions":[{"action":"edit","resource":"*","effect":"deny"}]}`, "deny", true},
		{"{ // comment\n \"permission\": \"allow\", /* x */ }", "allow", true},
		{`{"model":"x"}`, "", false},
		{`not json`, "", false},
		{`{"permission":"frobnicate"}`, "", false},
	}
	for i, c := range cases {
		p := write(t, d, "o.json", c.body)
		a := OpenCodeApproval(p)
		if a.Known != c.known || a.Mode != c.want {
			t.Errorf("case %d: %+v", i, a)
		}
	}
	// the project's file overrides the global one
	g := write(t, d, "g.json", `{"permission":"allow"}`)
	pr := write(t, d, "p.json", `{"permission":"ask"}`)
	if a := OpenCodeApproval(g, pr, filepath.Join(d, "none.json")); a.Mode != "ask" {
		t.Fatalf("%+v", a)
	}
	if a := OpenCodeApproval(filepath.Join(d, "none.json")); a.Known {
		t.Fatal()
	}
}

func TestOpenCodePlugins(t *testing.T) {
	d := t.TempDir()
	g := write(t, d, "g.json", `{"plugins":["a","/b"]}`)
	p := write(t, d, "p.json", `{"plugins":["c"]}`)
	if got := OpenCodePlugins(g, p, filepath.Join(d, "none")); !reflect.DeepEqual(got, []string{"a", "/b", "c"}) {
		t.Fatalf("%q", got)
	}
}

// A top-level `profile` key selects a profile whose values replace the top level's.
func TestCodexApprovalFollowsTheSelectedProfile(t *testing.T) {
	d := t.TempDir()
	p := write(t, d, "c.toml", "profile = \"fast\"\napproval_policy = \"on-request\"\nsandbox_mode = \"workspace-write\"\n\n[profiles.fast]\napproval_policy = \"never\"\n\n[profiles.other]\nsandbox_mode = \"read-only\"\n")
	a := CodexApproval(p)
	// the profile overrides the approval policy; the sandbox falls back to the top level
	if !a.Known || a.Approval != "never" || a.Sandbox != "workspace-write" {
		t.Fatalf("%+v", a)
	}
	missing := write(t, d, "m.toml", "profile = \"gone\"\napproval_policy = \"on-request\"\n[profiles.fast]\napproval_policy = \"never\"\n")
	if a := CodexApproval(missing); a.Known {
		t.Fatalf("a selected profile that cannot be read is unknown: %+v", a)
	}
	empty := write(t, d, "e.toml", "profile = \"p\"\napproval_policy = \"on-request\"\n[profiles.p]\nmodel = \"x\"\n")
	if a := CodexApproval(empty); !a.Known || a.Approval != "on-request" {
		t.Fatalf("%+v", a)
	}
}
