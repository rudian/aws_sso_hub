package taskstate

import "strings"

// CodexNotifyArgs is `-c notify=["<exe>","report","codex-notify"]` (turn complete only). It replaces a
// user's own `notify`, so the caller adds it only when the user has none (plan 4.8). Whether Codex's own hooks
// can be supplied per launch without a trust prompt is manual check A8; until it says so this is the route.
func CodexNotifyArgs(exe string) []string {
	return []string{"-c", "notify=[" + tomlString(exe) + `,"report","codex-notify"]`}
}

func tomlString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(s) + `"`
}
