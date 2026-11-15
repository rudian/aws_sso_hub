package credstore

import (
	"context"
	"strings"
	"testing"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/gitx"
)

type call struct {
	name  string
	args  []string
	stdin string
	cap   bool
}

// fakeRun records every command and answers from a script; it never executes anything.
func fakeRun(t *testing.T, answer func(c call, n int) (string, int)) *[]call {
	t.Helper()
	var calls []call
	old := run
	run = func(ctx context.Context, name string, args []string, stdin []byte, capture bool) ([]byte, int, error) {
		c := call{name: name, args: append([]string(nil), args...), stdin: string(stdin), cap: capture}
		calls = append(calls, c)
		out, exit := answer(c, len(calls))
		return []byte(out), exit, nil
	}
	t.Cleanup(func() { run = old })
	return &calls
}

func joined(c call) string { return c.name + " " + strings.Join(c.args, " ") }

func TestDarwinListArgv(t *testing.T) {
	calls := fakeRun(t, func(c call, n int) (string, int) {
		if strings.Contains(joined(c), "git-codecommit.us-east-1") {
			return "", 0
		}
		return "", 44
	})
	s := New("darwin", nil, false)
	if !s.Supported() {
		t.Fatal("darwin must be supported")
	}
	got, err := s.List(context.Background(), []string{"git-codecommit.us-west-2.amazonaws.com", "https://git-codecommit.us-east-1.amazonaws.com", "git-codecommit.us-east-1.amazonaws.com", "bad host; rm -rf"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "keychain:git-codecommit.us-east-1.amazonaws.com" || got[0].Store != api.StoreKeychain || got[0].Host != "git-codecommit.us-east-1.amazonaws.com" {
		t.Errorf("list: %+v", got)
	}
	want := []string{
		"security find-internet-password -s git-codecommit.us-east-1.amazonaws.com",
		"security find-internet-password -s git-codecommit.us-west-2.amazonaws.com",
	}
	if len(*calls) != 2 {
		t.Fatalf("calls: %v", *calls)
	}
	for i, c := range *calls {
		if joined(c) != want[i] || c.cap {
			t.Errorf("call %d: %q capture=%v", i, joined(c), c.cap)
		}
	}
}

func TestDarwinClearArgvAndStdin(t *testing.T) {
	deletes := 0
	calls := fakeRun(t, func(c call, n int) (string, int) {
		switch {
		case c.name == "security" && c.args[0] == "delete-internet-password":
			deletes++
			if deletes <= 2 {
				return "", 0
			}
			return "", 44
		case c.name == "security" && c.args[0] == "find-internet-password":
			return "", 44 // gone
		}
		return "", 0
	})
	git := func(ctx context.Context) (*gitx.Git, error) { return &gitx.Git{Bin: "/usr/bin/git-under-test"}, nil }
	n, err := New("darwin", git, false).Clear(context.Background(), []string{"keychain:git-codecommit.us-east-1.amazonaws.com", "cmdkey:ignored", "keychain:evil host"})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var lines []string
	for _, c := range *calls {
		lines = append(lines, joined(c))
	}
	want := []string{
		"/usr/bin/git-under-test -c credential.helper= -c credential.helper=osxkeychain credential reject",
		"security delete-internet-password -s git-codecommit.us-east-1.amazonaws.com",
		"security delete-internet-password -s git-codecommit.us-east-1.amazonaws.com",
		"security delete-internet-password -s git-codecommit.us-east-1.amazonaws.com",
		"security find-internet-password -s git-codecommit.us-east-1.amazonaws.com",
	}
	if strings.Join(lines, "\n") != strings.Join(want, "\n") {
		t.Errorf("commands:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	if (*calls)[0].stdin != "protocol=https\nhost=git-codecommit.us-east-1.amazonaws.com\n\n" {
		t.Errorf("reject stdin: %q", (*calls)[0].stdin)
	}
	for _, c := range *calls {
		if c.cap {
			t.Errorf("output must be discarded: %s", joined(c))
		}
	}
}

func TestDarwinClearCapsLoop(t *testing.T) {
	calls := fakeRun(t, func(c call, n int) (string, int) {
		if c.args[0] == "find-internet-password" {
			return "", 0 // still there
		}
		return "", 0
	})
	n, err := New("darwin", nil, false).Clear(context.Background(), []string{"keychain:git-codecommit.us-east-1.amazonaws.com"})
	if err != nil || n != 0 {
		t.Errorf("n=%d err=%v", n, err)
	}
	deletes := 0
	for _, c := range *calls {
		if len(c.args) > 0 && c.args[0] == "delete-internet-password" {
			deletes++
		}
	}
	if deletes != 20 {
		t.Errorf("delete attempts = %d, want the cap of 20", deletes)
	}
}

const cmdkeySample = `
Currently stored credentials:

    Target: LegacyGeneric:target=git:https://git-codecommit.us-east-1.amazonaws.com
    Type: Generic
    User: acme
    Local machine persistence

    Target: WindowsLive:target=virtualapp/didlogical
    Type: Generic
    User: 02xyz
    Local machine persistence

    Target: LegacyGeneric:target=git:https://someone@git-codecommit.us-east-1.amazonaws.com/v1/repos/x
    Type: Generic
    User: someone
`

func TestParseCmdkeyList(t *testing.T) {
	got := ParseCmdkeyList(strings.ReplaceAll(cmdkeySample, "\n", "\r\n"))
	want := []string{
		"LegacyGeneric:target=git:https://git-codecommit.us-east-1.amazonaws.com",
		"LegacyGeneric:target=git:https://someone@git-codecommit.us-east-1.amazonaws.com/v1/repos/x",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v", got)
	}
	if len(ParseCmdkeyList("")) != 0 {
		t.Error("empty")
	}
}

func TestWindowsListAndClear(t *testing.T) {
	calls := fakeRun(t, func(c call, n int) (string, int) {
		if c.args[0] == "/list" {
			return cmdkeySample, 0
		}
		return "", 0
	})
	s := New("windows", nil, false)
	got, err := s.List(context.Background(), nil)
	if err != nil || len(got) != 2 {
		t.Fatalf("%v %v", got, err)
	}
	if got[0].Host != "git-codecommit.us-east-1.amazonaws.com" || got[0].Store != api.StoreCredentialManager || got[1].Path != "v1/repos/x" || got[1].Host != "git-codecommit.us-east-1.amazonaws.com" {
		t.Errorf("list: %+v", got)
	}
	n, err := s.Clear(context.Background(), []string{got[0].ID, "cmdkey:LegacyGeneric:target=git:https://github.com"})
	if err != nil || n != 1 {
		t.Errorf("clear n=%d err=%v", n, err)
	}
	last := (*calls)[len(*calls)-1]
	if joined(last) != "cmdkey /delete:LegacyGeneric:target=git:https://git-codecommit.us-east-1.amazonaws.com" {
		t.Errorf("delete argv: %s", joined(last))
	}
}

func TestSandboxDisabled(t *testing.T) {
	calls := fakeRun(t, func(c call, n int) (string, int) { return "", 0 })
	s := New("darwin", nil, true)
	if s.Supported() {
		t.Error("sandbox store must be unsupported")
	}
	got, err := s.List(context.Background(), []string{"git-codecommit.us-east-1.amazonaws.com"})
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("list: %v %v", got, err)
	}
	_, err = s.Clear(context.Background(), []string{"keychain:git-codecommit.us-east-1.amazonaws.com"})
	if ae, ok := err.(*api.AppError); !ok || ae.Code != api.ErrSandbox {
		t.Errorf("clear: %v", err)
	}
	if len(*calls) != 0 {
		t.Errorf("sandbox executed commands: %v", *calls)
	}
	if u := New("linux", nil, false); u.Supported() {
		t.Error("linux unsupported")
	} else if _, err := u.Clear(context.Background(), nil); err == nil {
		t.Error("expected E_UNSUPPORTED_OS")
	}
}

func TestDarwinClearRefusesNonCodeCommitHosts(t *testing.T) {
	calls := fakeRun(t, func(c call, n int) (string, int) { return "", 44 })
	git := func(ctx context.Context) (*gitx.Git, error) { return &gitx.Git{Bin: "/usr/bin/git-under-test"}, nil }
	s := New("darwin", git, false)
	n, err := s.Clear(context.Background(), []string{"keychain:github.com", "keychain:accounts.example.com", "keychain:evil.git-codecommit.us-east-1.amazonaws.com.example.com"})
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(*calls) != 0 {
		t.Errorf("no security or git credential call may be made for other hosts: %v", *calls)
	}
	for _, host := range []string{
		"git-codecommit.us-east-1.amazonaws.com", "git-codecommit-fips.us-gov-west-1.amazonaws.com",
		"git-codecommit.cn-north-1.amazonaws.com.cn", "vpce-0abc.git-codecommit.us-east-1.vpce.amazonaws.com",
	} {
		*calls = nil
		if _, err := s.Clear(context.Background(), []string{"keychain:" + host}); err != nil {
			t.Fatal(err)
		}
		if len(*calls) == 0 {
			t.Errorf("%s must be cleared", host)
		}
	}
}

func TestWindowsClearOnlyListedCodeCommitTargets(t *testing.T) {
	calls := fakeRun(t, func(c call, n int) (string, int) {
		if c.args[0] == "/list" {
			return cmdkeySample + "\n    Target: LegacyGeneric:target=github.com/git-codecommit.x\n    Type: Generic\n", 0
		}
		return "", 0
	})
	s := New("windows", nil, false)
	n, err := s.Clear(context.Background(), []string{
		"cmdkey:LegacyGeneric:target=github.com/git-codecommit.x",
		"cmdkey:LegacyGeneric:target=git:https://git-codecommit.us-east-1.amazonaws.com/v1/repos/not-stored",
		"cmdkey:WindowsLive:target=virtualapp/didlogical",
		"cmdkey:LegacyGeneric:target=git:https://git-codecommit.us-east-1.amazonaws.com",
	})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	var deletes []string
	for _, c := range *calls {
		if strings.HasPrefix(c.args[0], "/delete:") {
			deletes = append(deletes, c.args[0])
		}
	}
	if len(deletes) != 1 || deletes[0] != "/delete:LegacyGeneric:target=git:https://git-codecommit.us-east-1.amazonaws.com" {
		t.Errorf("deletes: %v", deletes)
	}
}
