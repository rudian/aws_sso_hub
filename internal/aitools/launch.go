package aitools

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/rudian/codepass/internal/execx"
	"github.com/rudian/codepass/internal/pty"
)

// MaxMessageBytes caps the first message. A Windows command line holds 32767 UTF-16 units in all, and the
// message is one argument of it.
const MaxMessageBytes = 20000

// Errors of the launch builder. Each names a field a caller can show.
var (
	ErrBadTool      = errors.New("aitools: unknown tool")
	ErrBadSessionID = errors.New("aitools: the session ID is not valid for this tool")
	ErrBadMessage   = errors.New("aitools: the first message is not valid")
	ErrBadDir       = errors.New("aitools: the folder is not valid")
	ErrBadAccount   = errors.New("aitools: the account environment is not valid")
	ErrBadPlugin    = errors.New("aitools: a plugin folder is not valid")
	ErrBadConfig    = errors.New("aitools: a Codex config override is not valid")
	ErrBadTask      = errors.New("aitools: the task ID is not valid")
)

var (
	reUUID     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reOCID     = regexp.MustCompile(`^ses_[A-Za-z0-9_]{1,64}$`)
	reTaskID   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
	reCfgKey   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	accountVar = map[ID]string{Claude: "CLAUDE_CONFIG_DIR", Codex: "CODEX_HOME"}
)

// NewSessionID makes the session ID the app assigns when a task is created (plan 4.8 "Resume uses session IDs
// the app creates"): a UUID for Claude Code (--session-id), "ses_..." for OpenCode (--session creates the
// session when it is absent; verified in S0-03). Codex has none to assign: its ID is captured from its hook
// or notify, so the result is "".
func NewSessionID(tool ID) (string, error) {
	switch tool {
	case Claude:
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		b[6] = b[6]&0x0f | 0x40 // version 4
		b[8] = b[8]&0x3f | 0x80 // variant
		h := hex.EncodeToString(b[:])
		return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
	case OpenCode:
		var b [13]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		return "ses_" + hex.EncodeToString(b[:]), nil
	case Codex:
		return "", nil
	}
	return "", ErrBadTool
}

// LaunchRequest is everything a launch depends on. Nothing here is spliced into text: every value becomes one
// argument or one variable.
type LaunchRequest struct {
	Tool ID
	GOOS string // "" = the running OS (via runtimeGOOS)
	// Shell is the person's login shell on macOS ("" = /bin/zsh). Windows starts the program directly.
	Shell string
	// ToolPath is the program's absolute path (Windows; "" = the bare name, found through PATH).
	ToolPath string
	// Exists tells whether a file exists (the Windows shim lookup, WindowsCommand); nil = the file system.
	Exists func(string) bool
	// Lookup resolves a bare program name (Windows; nil = execx.LookPath).
	Lookup func(string) (string, error)
	Dir    string // the task's folder

	TaskID    string // becomes CODEPASS_TASK in the child's environment (report mode reads it)
	SessionID string // Claude Code UUID or OpenCode ses_... ; Codex: the captured ID, for Resume only
	Resume    bool

	// FirstMessage is the task description. It reaches the tool only as message text (plan 4.7 point 3).
	FirstMessage string

	// PluginDirs are Claude Code's session-only plugin folders (--plugin-dir, one flag each; hooks travel this
	// way because two --settings flags do not merge and a user's own function passes one).
	PluginDirs []string
	// OpenCodeConfig is the JSON for OPENCODE_CONFIG_CONTENT (OpenCodeConfigContent builds it).
	OpenCodeConfig string
	// CodexConfig are "-c key=value" overrides (key and value already in TOML form). Sandbox and approval keys
	// are refused: those follow the owner's own settings.
	CodexConfig []string
	// CodexNotify is the argv of the notify program. It becomes -c notify=[...] only when the person has none
	// of their own (CodexHasNotify false): the override would replace theirs.
	CodexNotify    []string
	CodexHasNotify bool

	// AccountEnv is the account's variable for milestone 8 ("CLAUDE_CONFIG_DIR=/abs/path" or "CODEX_HOME=...").
	AccountEnv []string
}

// Launch is what A2-02's terminal session needs: the argument list, the folder, and the variables the app adds
// to the session's explicit environment (never the whole environment).
type Launch struct {
	Argv []string
	Dir  string
	Env  []string // app-added variables only: CODEPASS_TASK, the account variable, OPENCODE_CONFIG_CONTENT
	// NotifySkipped is true when a Codex notify program was asked for but the person already has one.
	NotifySkipped bool
}

// BuildLaunch builds one launch. The arguments are checked with CheckArgs before they are returned.
func BuildLaunch(r LaunchRequest) (Launch, error) {
	if !r.Tool.Valid() {
		return Launch{}, ErrBadTool
	}
	goos := r.GOOS
	if goos == "" {
		goos = runtimeGOOS
	}
	if r.Dir != "" && (!execx.IsAbsPath(r.Dir, goos) || execx.HasControlChars(r.Dir)) {
		return Launch{}, ErrBadDir
	}
	if r.TaskID != "" && !reTaskID.MatchString(r.TaskID) {
		return Launch{}, ErrBadTask
	}
	if r.FirstMessage != "" {
		if len(r.FirstMessage) > MaxMessageBytes || strings.ContainsRune(r.FirstMessage, 0) {
			return Launch{}, ErrBadMessage
		}
		if r.Resume {
			return Launch{}, fmt.Errorf("%w: a resumed session takes no new first message", ErrBadMessage)
		}
	}
	var args []string
	var err error
	switch r.Tool {
	case Claude:
		args, err = claudeArgs(r)
	case Codex:
		args, err = codexArgs(r)
	case OpenCode:
		args, err = openCodeArgs(r)
	}
	if err != nil {
		return Launch{}, err
	}
	if err := CheckArgs(args); err != nil {
		return Launch{}, err
	}
	out := Launch{Dir: r.Dir}
	if r.Tool == Codex && len(r.CodexNotify) > 0 && r.CodexHasNotify {
		out.NotifySkipped = true
	}
	if goos == "windows" {
		prog := r.ToolPath
		if prog == "" {
			prog = string(r.Tool)
		}
		if out.Argv, err = WindowsCommand(r.Tool, prog, args, r.FirstMessage, ShimEnv{Exists: r.Exists, Lookup: r.Lookup}); err != nil {
			return Launch{}, err
		}
	} else {
		sh := r.Shell
		if sh == "" {
			sh = "/bin/zsh"
		}
		out.Argv = pty.LaunchArgv(sh, string(r.Tool), args...)
	}
	if r.TaskID != "" {
		out.Env = append(out.Env, "CODEPASS_TASK="+r.TaskID)
	}
	acc, err := accountEnv(r.Tool, r.AccountEnv, goos)
	if err != nil {
		return Launch{}, err
	}
	out.Env = append(out.Env, acc...)
	if r.Tool == OpenCode && r.OpenCodeConfig != "" {
		// Only what the app injects: a "permission", "permissions" or "agent" key is the config form of
		// OpenCode's --auto, so anything but "plugins" is refused (the second lock, like CheckArgs).
		var cfg map[string]json.RawMessage
		if json.Unmarshal([]byte(r.OpenCodeConfig), &cfg) != nil {
			return Launch{}, fmt.Errorf("%w: OpenCode config is not a JSON object", ErrBadPlugin)
		}
		for k := range cfg {
			if k != "plugins" {
				return Launch{}, fmt.Errorf("%w: OpenCode config key %q is not injected by the app", ErrBadPlugin, k)
			}
		}
		out.Env = append(out.Env, "OPENCODE_CONFIG_CONTENT="+r.OpenCodeConfig)
	}
	return out, nil
}

// accountEnv validates the milestone 8 account variables: only the tool's own variable, with an absolute path.
func accountEnv(tool ID, env []string, goos string) ([]string, error) {
	var out []string
	for _, kv := range env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || accountVar[tool] == "" || k != accountVar[tool] || !execx.IsAbsPath(v, goos) || execx.HasControlChars(v) {
			return nil, ErrBadAccount
		}
		out = append(out, kv)
	}
	return out, nil
}

func checkDirs(dirs []string, goos string) error {
	for _, d := range dirs {
		if !execx.IsAbsPath(d, goos) || execx.HasControlChars(d) {
			return ErrBadPlugin
		}
	}
	return nil
}

// message returns the argument tail for a first message after "--" (Claude Code and Codex).
func messageTail(msg string) []string {
	if msg == "" {
		return nil
	}
	return []string{"--", msg}
}

func claudeArgs(r LaunchRequest) ([]string, error) {
	var a []string
	if r.SessionID != "" {
		if !reUUID.MatchString(r.SessionID) {
			return nil, ErrBadSessionID
		}
		if r.Resume {
			a = append(a, "--resume", r.SessionID)
		} else {
			a = append(a, "--session-id", r.SessionID)
		}
	} else if r.Resume {
		return nil, ErrBadSessionID
	}
	goos := r.GOOS
	if goos == "" {
		goos = runtimeGOOS
	}
	if err := checkDirs(r.PluginDirs, goos); err != nil {
		return nil, err
	}
	for _, d := range r.PluginDirs {
		a = append(a, "--plugin-dir", d)
	}
	return append(a, messageTail(claudeMessage(r.FirstMessage))...), nil
}

// claudeMessage keeps a one-word message from being read as a command. Claude Code's parser runs a subcommand
// when the first operand is a command name, even after "--" ("update" would update Claude Code, "install" would
// switch it to the native build). A trailing space means the operand is never a command name; the tool trims it
// from the prompt (check A18).
func claudeMessage(msg string) string {
	if msg != "" && !strings.ContainsFunc(msg, unicode.IsSpace) {
		return msg + " "
	}
	return msg
}

func openCodeArgs(r LaunchRequest) ([]string, error) {
	// --standalone always: without it OpenCode hands the work to a shared background service that outlives the
	// task, ignores the task's environment and cannot be stopped with it (S0-03).
	a := []string{"--standalone"}
	if r.SessionID != "" {
		if !reOCID.MatchString(r.SessionID) {
			return nil, ErrBadSessionID
		}
		a = append(a, "--session", r.SessionID)
	} else if r.Resume {
		return nil, ErrBadSessionID
	}
	if r.FirstMessage != "" {
		a = append(a, "--prompt="+r.FirstMessage) // one token: a description starting with a dash is text
	}
	return a, nil
}

func codexArgs(r LaunchRequest) ([]string, error) {
	var a []string
	for _, kv := range r.CodexConfig {
		if !validCodexOverride(kv) {
			return nil, ErrBadConfig
		}
		a = append(a, "-c", kv)
	}
	if len(r.CodexNotify) > 0 && !r.CodexHasNotify {
		n, err := CodexNotifyOverride(r.CodexNotify)
		if err != nil {
			return nil, err
		}
		a = append(a, "-c", n)
	}
	if r.Resume {
		if !reUUID.MatchString(r.SessionID) {
			return nil, ErrBadSessionID
		}
		return append(a, "resume", r.SessionID), nil
	}
	if r.SessionID != "" {
		return nil, ErrBadSessionID // Codex has no way to assign an ID at the start
	}
	return append(a, messageTail(r.FirstMessage)...), nil
}

func validCodexOverride(kv string) bool {
	k, _, ok := strings.Cut(kv, "=")
	return ok && reCfgKey.MatchString(k) && !configKeyForbidden(kv) && !strings.ContainsRune(kv, 0) &&
		!strings.Contains(strings.ToLower(kv), "danger-full-access")
}

// CodexNotifyOverride returns the "notify=[...]" override for a notify program's argv, as a TOML array of
// strings (Codex parses the value of -c as TOML).
func CodexNotifyOverride(argv []string) (string, error) {
	if len(argv) == 0 {
		return "", ErrBadConfig
	}
	parts := make([]string, len(argv))
	for i, s := range argv {
		q, err := tomlString(s)
		if err != nil {
			return "", err
		}
		parts[i] = q
	}
	return "notify=[" + strings.Join(parts, ",") + "]", nil
}

// tomlString quotes s as a TOML basic string.
func tomlString(s string) (string, error) {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n' || r == '\r' || r == 0:
			return "", ErrBadConfig
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String(), nil
}

// OpenCodeConfigContent builds the OPENCODE_CONFIG_CONTENT value: a "plugins" list that holds the person's own
// plugins first and the app's plugin folder last, so that however OpenCode merges the layers (concatenate or
// replace) the person's plugins still load (v2 config key; S0-03).
func OpenCodeConfigContent(plugin string, userPlugins []string) (string, error) {
	if plugin == "" || strings.ContainsRune(plugin, 0) {
		return "", ErrBadPlugin
	}
	list := make([]string, 0, len(userPlugins)+1)
	seen := map[string]bool{}
	for _, p := range append(append([]string{}, userPlugins...), plugin) {
		if p != "" && !seen[p] {
			seen[p] = true
			list = append(list, p)
		}
	}
	b, err := json.Marshal(map[string]any{"plugins": list})
	return string(b), err
}
