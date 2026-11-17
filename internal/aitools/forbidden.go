package aitools

import (
	"errors"
	"fmt"
	"strings"
)

// ErrForbiddenFlag is returned when an argument list would skip a tool's permission prompts or override its
// sandbox. The builders never produce one; CheckArgs is the second lock on the door, and the tests drive it with
// every flag the plan lists (plan 4.7, 4.8; tasks A2-04).
var ErrForbiddenFlag = errors.New("aitools: this flag skips permission prompts or overrides the sandbox")

// forbiddenPrefixes match a flag with or without "=value".
var forbiddenPrefixes = []string{
	"--dangerously",       // --dangerously-skip-permissions, --dangerously-bypass-approvals-and-sandbox
	"--allow-dangerously", // --allow-dangerously-skip-permissions
	"--yolo",
	"--approve-for-me",
	"--full-auto", // Codex: lowers the approval policy and picks a sandbox
}

// forbiddenConfigKeys are Codex config keys a "-c key=value" must not set: the sandbox and the approval policy
// follow the owner's own settings. Prefixes.
var forbiddenConfigKeys = []string{"sandbox", "approval", "default_permissions"}

// CheckArgs returns ErrForbiddenFlag (wrapped, naming the flag) when args holds a flag that skips permission
// prompts or overrides the sandbox. It reads the flags only: everything after a "--" is the first message and
// is never a flag, and so is a "--prompt=<text>" token (OpenCode's message form).
func CheckArgs(args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return nil
		}
		if strings.HasPrefix(a, "--prompt=") {
			continue
		}
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		bad := func() error { return fmt.Errorf("%w: %s", ErrForbiddenFlag, a) }
		for _, p := range forbiddenPrefixes {
			if a == p || strings.HasPrefix(a, p+"=") || strings.HasPrefix(a, p+"-") {
				return bad()
			}
		}
		switch {
		case a == "--auto" || strings.HasPrefix(a, "--auto="): // OpenCode: auto-approve permissions
			return bad()
		case a == "--permission-mode":
			if next == "bypassPermissions" {
				return bad()
			}
		case a == "--permission-mode=bypassPermissions":
			return bad()
		case a == "--sandbox" || a == "-s":
			if next == "danger-full-access" {
				return bad()
			}
		case a == "--sandbox=danger-full-access", a == "-s=danger-full-access", a == "-sdanger-full-access":
			return bad()
		case a == "--ask-for-approval" || a == "-a":
			if next == "never" {
				return bad()
			}
		case a == "--ask-for-approval=never", a == "-a=never", a == "-anever":
			return bad()
		case a == "-c" || a == "--config":
			if configKeyForbidden(next) {
				return fmt.Errorf("%w: %s %s", ErrForbiddenFlag, a, keyOf(next))
			}
		case strings.HasPrefix(a, "--config="):
			if configKeyForbidden(strings.TrimPrefix(a, "--config=")) {
				return bad()
			}
		}
	}
	return nil
}

func keyOf(kv string) string {
	k, _, _ := strings.Cut(kv, "=")
	return strings.TrimSpace(k)
}

func configKeyForbidden(kv string) bool {
	k := strings.ToLower(keyOf(kv))
	// A dotted key under a profile ("profiles.x.sandbox_mode") ends in the same name.
	last := k
	if i := strings.LastIndexByte(k, '.'); i >= 0 {
		last = k[i+1:]
	}
	for _, p := range forbiddenConfigKeys {
		if strings.HasPrefix(k, p) || strings.HasPrefix(last, p) {
			return true
		}
	}
	return false
}
