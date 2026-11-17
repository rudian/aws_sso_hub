package aitools

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
)

// Approval is what a task's header shows (plan 4.8, "Approval and sandbox vocabulary"). The app only reads it.
type Approval struct {
	Tool     ID
	Kind     string // "permission-mode", "approval-sandbox" or "permission-rules"
	Mode     string // Claude Code: permission mode; OpenCode: allow, ask, deny or mixed
	Approval string // Codex approval policy
	Sandbox  string // Codex sandbox mode
	Known    bool
	Source   string // "hook", "config" or ""
}

// ClaudeModes is Claude Code's permission-mode vocabulary (from `claude --help`).
var ClaudeModes = []string{"manual", "acceptEdits", "auto", "plan", "dontAsk", "bypassPermissions"}

// ClaudeApproval turns the permission_mode of a hook payload into the header's value. Claude Code's payloads
// carry it on every event except SessionStart, and it can change during a session (Shift+Tab), so the header
// says "as of the last event". "default" is Claude Code's own name for manual. An empty or unknown value is
// "unknown", never a guess.
func ClaudeApproval(permissionMode string) Approval {
	a := Approval{Tool: Claude, Kind: "permission-mode"}
	m := permissionMode
	if m == "default" {
		m = "manual"
	}
	for _, v := range ClaudeModes {
		if m == v {
			a.Mode, a.Known, a.Source = v, true, "hook"
		}
	}
	return a
}

var (
	codexApprovals = map[string]bool{"on-request": true, "never": true, "untrusted": true, "on-failure": true}
	codexSandboxes = map[string]bool{"read-only": true, "workspace-write": true, "danger-full-access": true}
)

// topLevelTOML reads the top-level (before the first [table]) key = "string" pairs of a TOML file, and the keys
// whose value is anything else. It is deliberately small: the app reads two strings and one key's presence,
// and never writes this file.
func topLevelTOML(path string) (strs map[string]string, keys map[string]bool, err error) {
	return tomlTable(path, "")
}

// tomlTable is topLevelTOML for one table: table "" is the top level, "profiles.fast" is [profiles.fast].
func tomlTable(path, table string) (strs map[string]string, keys map[string]bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	strs, keys = map[string]string{}, map[string]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	cur := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			if strings.HasPrefix(line, "[[") {
				cur = "\x00" + line // an array of tables: never the one asked for
			} else if end := strings.IndexByte(line, ']'); end > 0 {
				cur = strings.ReplaceAll(strings.TrimSpace(line[1:end]), `"`, "")
			}
			continue
		}
		if cur != table {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.Trim(strings.TrimSpace(k), `"`)
		v = strings.TrimSpace(v)
		keys[k] = true
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
			if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
				strs[k] = v[1 : 1+end]
			}
		}
	}
	return strs, keys, sc.Err()
}

// CodexApproval reads the approval policy and the sandbox mode from Codex's config.toml (read only). The
// top-level values apply unless a top-level `profile` key selects a profile: then the values come from
// [profiles.<name>] (a value the profile does not set falls back to the top level; a profile table that cannot
// be read leaves the answer unknown). A missing file or key leaves the field empty. Known is true when at least
// one value was read.
func CodexApproval(configPath string) Approval {
	a := Approval{Tool: Codex, Kind: "approval-sandbox"}
	strs, keys, err := topLevelTOML(configPath)
	if err != nil {
		return a
	}
	if keys["profile"] {
		name := strs["profile"]
		ps, _, perr := tomlTable(configPath, "profiles."+name)
		if name == "" || perr != nil || len(ps) == 0 && !profileTableExists(configPath, name) {
			return a // a profile is selected and its values cannot be read: unknown, not the top level's
		}
		merged := map[string]string{}
		for _, k := range []string{"approval_policy", "sandbox_mode"} {
			if v, ok := ps[k]; ok {
				merged[k] = v
			} else if v, ok := strs[k]; ok {
				merged[k] = v
			}
		}
		strs = merged
	}
	if v := strs["approval_policy"]; codexApprovals[v] {
		a.Approval = v
	}
	if v := strs["sandbox_mode"]; codexSandboxes[v] {
		a.Sandbox = v
	}
	a.Known = a.Approval != "" || a.Sandbox != ""
	if a.Known {
		a.Source = "config"
	}
	return a
}

// profileTableExists reports whether config.toml has a [profiles.<name>] header.
func profileTableExists(path, name string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.ReplaceAll(strings.TrimSpace(l), `"`, "")
		if l == "[profiles."+name+"]" {
			return true
		}
	}
	return false
}

// CodexHasNotify reports whether the person's own config.toml sets "notify" at the top level. When it does, the
// app does not pass -c notify=[...], which would replace it.
func CodexHasNotify(configPath string) bool {
	_, keys, err := topLevelTOML(configPath)
	return err == nil && keys["notify"]
}

// stripJSONC removes // and /* */ comments and trailing commas outside strings, so a .jsonc-style config parses.
func stripJSONC(b []byte) []byte { return dropTrailingCommas(stripComments(b)) }

func stripComments(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inStr, esc := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inStr:
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(b) && b[i+1] == '/':
			for i < len(b) && b[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			i += 2
			for i+1 < len(b) && !(b[i] == '*' && b[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}
	return out
}

// dropTrailingCommas removes a comma whose next non-space character closes an object or an array.
func dropTrailingCommas(b []byte) []byte {
	out := make([]byte, 0, len(b))
	inStr, esc := false, false
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case inStr:
			out = append(out, c)
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == ',':
			// a trailing comma: the next non-space character closes an object or an array
			j := i + 1
			for j < len(b) && (b[j] == ' ' || b[j] == '\t' || b[j] == '\n' || b[j] == '\r') {
				j++
			}
			if j < len(b) && (b[j] == '}' || b[j] == ']') {
				continue
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}

func readJSONC(path string) (map[string]json.RawMessage, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(stripJSONC(b), &m) != nil {
		return nil, false
	}
	return m, true
}

// OpenCodeApproval summarises the permission rules of OpenCode's config files, later files overriding earlier
// ones (the global file first, then the project's). The v1 key is "permission" (a word, or an object of action
// to word); the v2 docs show "permissions", a list of {action, resource, effect}. The summary is "allow", "ask"
// or "deny" when every rule agrees, "mixed" otherwise. No event carries the effective mode, so an absent or
// unreadable setting is "unknown" (Known false).
func OpenCodeApproval(configPaths ...string) Approval {
	a := Approval{Tool: OpenCode, Kind: "permission-rules"}
	var effects []string
	found := false
	for _, p := range configPaths {
		m, ok := readJSONC(p)
		if !ok {
			continue
		}
		if raw, has := m["permission"]; has {
			var word string
			var obj map[string]json.RawMessage
			switch {
			case json.Unmarshal(raw, &word) == nil:
				effects, found = []string{word}, true
			case json.Unmarshal(raw, &obj) == nil:
				effects, found = nil, true
				for _, v := range obj {
					var w string
					if json.Unmarshal(v, &w) == nil {
						effects = append(effects, w)
						continue
					}
					var nested map[string]string // "bash": {"git *": "allow"}
					if json.Unmarshal(v, &nested) == nil {
						for _, w := range nested {
							effects = append(effects, w)
						}
					}
				}
			}
		}
		if raw, has := m["permissions"]; has {
			var list []struct {
				Effect string `json:"effect"`
			}
			if json.Unmarshal(raw, &list) == nil {
				effects, found = nil, true
				for _, r := range list {
					effects = append(effects, r.Effect)
				}
			}
		}
	}
	if !found {
		return a
	}
	uniq := ""
	for _, e := range effects {
		if e != "allow" && e != "ask" && e != "deny" {
			continue
		}
		switch {
		case uniq == "":
			uniq = e
		case uniq != e:
			uniq = "mixed"
		}
	}
	if uniq == "" {
		return a // nothing recognisable
	}
	a.Mode, a.Known, a.Source = uniq, true, "config"
	return a
}

// OpenCodePlugins returns the "plugins" list of OpenCode's config files, so the app's plugin is added to the
// person's own and never replaces them.
func OpenCodePlugins(configPaths ...string) []string {
	var out []string
	for _, p := range configPaths {
		m, ok := readJSONC(p)
		if !ok {
			continue
		}
		var list []string
		if json.Unmarshal(m["plugins"], &list) == nil {
			out = append(out, list...)
		}
	}
	return out
}
