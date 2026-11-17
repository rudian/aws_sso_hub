package aitools

import (
	"errors"
	"testing"
)

// Every flag the A2-04 entry lists, in every spelling the tools accept.
var forbiddenArgLists = [][]string{
	{"--dangerously-skip-permissions"},
	{"--dangerously-skip-permissions=true"},
	{"--allow-dangerously-skip-permissions"},
	{"--permission-mode", "bypassPermissions"},
	{"--permission-mode=bypassPermissions"},
	{"--auto"},
	{"--yolo"},
	{"--dangerously-bypass-approvals-and-sandbox"},
	{"--approve-for-me"},
	{"--sandbox", "danger-full-access"},
	{"-s", "danger-full-access"},
	{"--sandbox=danger-full-access"},
	{"-sdanger-full-access"},
	{"--ask-for-approval", "never"},
	{"-a", "never"},
	{"--full-auto"},
	{"-c", "sandbox_mode=\"danger-full-access\""},
	{"-c", "approval_policy=\"never\""},
	{"--config", "sandbox_mode=\"workspace-write\""},
	{"--config=profiles.x.sandbox_mode=\"read-only\""},
	{"-c", "default_permissions=\"x\""},
	// the same flags after a harmless one
	{"--standalone", "--session", "ses_abc", "--auto"},
	{"--session-id", "x", "--dangerously-skip-permissions", "--", "hello"},
}

func TestCheckArgsRefusesEveryForbiddenFlag(t *testing.T) {
	for _, args := range forbiddenArgLists {
		err := CheckArgs(args)
		if !errors.Is(err, ErrForbiddenFlag) {
			t.Errorf("CheckArgs(%q) = %v, want ErrForbiddenFlag", args, err)
		}
	}
}

func TestCheckArgsLetsMessageTextThrough(t *testing.T) {
	ok := [][]string{
		{"--session-id", "x", "--", "--dangerously-skip-permissions"},
		{"--", "--auto", "--yolo", "-s", "danger-full-access"},
		{"--standalone", "--prompt=--dangerously-skip-permissions"},
		{"--standalone", "--prompt=--auto"},
		{"-c", "notify=[\"/x/codepass\",\"report\"]", "--", "-a", "never"},
		{"--permission-mode", "plan"},
		{"-s", "read-only"},
		{"-c", "features.hooks=true"},
		{"--plugin-dir", "/tmp/a", "--plugin-dir", "/tmp/b"},
	}
	for _, args := range ok {
		if err := CheckArgs(args); err != nil {
			t.Errorf("CheckArgs(%q) = %v, want nil", args, err)
		}
	}
}
