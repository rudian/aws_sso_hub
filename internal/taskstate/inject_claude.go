package taskstate

import (
	"encoding/json"
	"path/filepath"
	"runtime"
)

// hookTimeoutSeconds keeps a stuck hook from holding the tool: report mode needs well under a second.
const hookTimeoutSeconds = 5

type hookCmd struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type hookGroup struct {
	Matcher string    `json:"matcher,omitempty"`
	Hooks   []hookCmd `json:"hooks"`
}

// ClaudeHooks is the hooks object of plan 4.8: six events plus one PreToolUse whose matcher is exactly
// AskUserQuestion (it runs for that tool only, so no other tool call starts a process). No PostToolUse.
func ClaudeHooks(goos, exe string) map[string][]hookGroup {
	cmd := []hookCmd{{Type: "command", Command: ReportCommand(goos, exe, "report", string(ToolClaude)), Timeout: hookTimeoutSeconds}}
	h := map[string][]hookGroup{}
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "Notification", "PermissionRequest", "Stop", "StopFailure", "SessionEnd"} {
		h[ev] = []hookGroup{{Hooks: cmd}}
	}
	h["PreToolUse"] = []hookGroup{{Matcher: QuestionTool, Hooks: cmd}}
	return h
}

// WriteClaudePlugin writes the session-only plugin (.claude-plugin/plugin.json and hooks/hooks.json) under
// taskDir (<data>/sessions/<task-id>/) and returns its folder, to be passed as `--plugin-dir <folder>`. Plugin
// hooks add to whatever settings win, so the owner's own `claude` function and its --settings keep working and
// flag order stops mattering (S0-03). Nothing is written to ~/.claude.
func WriteClaudePlugin(taskDir, exe string) (string, error) {
	return writeClaudePlugin(taskDir, exe, runtime.GOOS)
}

func writeClaudePlugin(taskDir, exe, goos string) (string, error) {
	dir := filepath.Join(taskDir, "claude-plugin")
	manifest, _ := json.MarshalIndent(map[string]string{
		"name":        "codepass-signals",
		"version":     "1.0.0",
		"description": "Reports task state to Codepass. Session-only; written by the app for one launch.",
	}, "", "  ")
	hooks, err := json.MarshalIndent(map[string]any{"hooks": ClaudeHooks(goos, exe)}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writePrivate(filepath.Join(dir, ".claude-plugin", "plugin.json"), manifest); err != nil {
		return "", err
	}
	if err := writePrivate(filepath.Join(dir, "hooks", "hooks.json"), hooks); err != nil {
		return "", err
	}
	return dir, nil
}

// ClaudePluginArgs is the launch argument for the plugin folder.
func ClaudePluginArgs(pluginDir string) []string { return []string{"--plugin-dir", pluginDir} }

// WriteClaudeSettingsFallback writes the same hooks as a settings file for `--settings <file>`. It is only the
// fallback: two --settings flags do not merge (the last wins), so it is used only when the plugin route
// cannot be.
func WriteClaudeSettingsFallback(taskDir, exe string) (string, error) {
	b, err := json.MarshalIndent(map[string]any{"hooks": ClaudeHooks(runtime.GOOS, exe)}, "", "  ")
	if err != nil {
		return "", err
	}
	p := filepath.Join(taskDir, "claude-settings.json")
	return p, writePrivate(p, b)
}
