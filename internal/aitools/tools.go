// Package aitools knows the three AI coding tools the app runs in a task (Claude Code, Codex, OpenCode):
// how to find them, the command that installs a missing one, the arguments of a launch, the naming call, and
// the approval mode a task header shows (plan 4.7, 4.8).
//
// Nothing here starts a tool by itself except Detect (only `--version`) and Namer (one headless call), and
// nothing here can produce a flag that skips a permission prompt (CheckArgs, forbidden.go).
package aitools

import "strings"

// ID names a tool. The values are also the program names.
type ID string

const (
	Claude   ID = "claude"
	Codex    ID = "codex"
	OpenCode ID = "opencode"
)

// All lists the tools in the order the screen shows them.
func All() []ID { return []ID{Claude, Codex, OpenCode} }

// Valid reports whether id is one of the three.
func (id ID) Valid() bool { return id == Claude || id == Codex || id == OpenCode }

// Name is the display name.
func (id ID) Name() string {
	switch id {
	case Claude:
		return "Claude Code"
	case Codex:
		return "Codex"
	case OpenCode:
		return "OpenCode"
	}
	return string(id)
}

// Install is one installer call: a program and its complete argument list.
type Install struct {
	Exe  string   // brew, winget or npm
	Args []string // complete; nothing else is appended
}

// Display is the command as the screen shows it before it runs.
func (i Install) Display() string { return strings.Join(append([]string{i.Exe}, i.Args...), " ") }

// InstallFor returns the installer call for a tool on an OS ("darwin" or "windows"; ok is false elsewhere).
// macOS: Homebrew casks (formula for OpenCode). Windows: winget for Claude Code and Codex, npm for OpenCode
// because winget has no package for it (check B10 confirms the package names).
func InstallFor(id ID, goos string) (Install, bool) {
	switch goos {
	case "darwin":
		switch id {
		case Claude:
			return Install{"brew", []string{"install", "--cask", "claude-code"}}, true
		case Codex:
			return Install{"brew", []string{"install", "--cask", "codex"}}, true
		case OpenCode:
			return Install{"brew", []string{"install", "opencode"}}, true
		}
	case "windows":
		switch id {
		case Claude:
			return Install{"winget", []string{"install", "--id", "Anthropic.ClaudeCode", "-e"}}, true
		case Codex:
			return Install{"winget", []string{"install", "--id", "OpenAI.Codex", "-e"}}, true
		case OpenCode:
			return Install{"npm", []string{"install", "-g", "opencode-ai"}}, true
		}
	}
	return Install{}, false
}
