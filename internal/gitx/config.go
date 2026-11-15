package gitx

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/rudian/codepass/internal/api"
)

// Entry is one `git config --null` record. Key is the canonical key as git prints it (section and variable
// lowercase, subsection case preserved). Subsection and Var are split from Key ("credential.<sub>.<var>").
type Entry struct {
	Scope, Origin, Key, Value, Subsection, Var string
	NoValue                                    bool // the key has no "=" (boolean shorthand)
}

// splitKey splits "section.sub.section.var" into section, subsection (between the first and last dot) and var.
func splitKey(key string) (section, sub, variable string) {
	first := strings.IndexByte(key, '.')
	if first < 0 {
		return key, "", ""
	}
	last := strings.LastIndexByte(key, '.')
	section = key[:first]
	if last == first {
		return section, "", key[first+1:]
	}
	return section, key[first+1 : last], key[last+1:]
}

// ParseNullConfig parses the output of `git config --null [--show-scope --show-origin] --get-regexp ...`.
// withScope=true expects `<scope>\0<origin>\0<key>\n<value>\0` records; false expects `<key>\n<value>\0`.
func ParseNullConfig(out []byte, withScope bool) ([]Entry, error) {
	entries := make([]Entry, 0)
	if len(bytes.TrimSpace(out)) == 0 {
		return entries, nil
	}
	if out[len(out)-1] != 0 {
		return nil, fmt.Errorf("gitx: config output is not NUL terminated")
	}
	toks := bytes.Split(out[:len(out)-1], []byte{0})
	step := 1
	if withScope {
		step = 3
	}
	if len(toks)%step != 0 {
		return nil, fmt.Errorf("gitx: unexpected config record count %d", len(toks))
	}
	for i := 0; i < len(toks); i += step {
		var e Entry
		kv := toks[i]
		if withScope {
			e.Scope = string(toks[i])
			e.Origin = strings.TrimPrefix(string(toks[i+1]), "file:")
			kv = toks[i+2]
		}
		if nl := bytes.IndexByte(kv, '\n'); nl >= 0 {
			e.Key, e.Value = string(kv[:nl]), string(kv[nl+1:])
		} else {
			e.Key, e.NoValue = string(kv), true
		}
		_, e.Subsection, e.Var = splitKey(e.Key)
		entries = append(entries, e)
	}
	return entries, nil
}

func (g *Git) configEntries(ctx context.Context, dir, regexp string) ([]Entry, error) {
	res, err := g.runOK(ctx, dir, nil, []int{1}, "config", "--null", "--show-scope", "--show-origin", "--get-regexp", regexp)
	if err != nil {
		return nil, err
	}
	if res.ExitCode == 1 {
		return []Entry{}, nil
	}
	es, perr := ParseNullConfig(res.Stdout, true)
	if perr != nil {
		return nil, api.NewError(api.ErrGitFailed, "cannot read git config").WithDetail(perr.Error())
	}
	return es, nil
}

// CredentialEntries returns every credential.* and awsssohub.* entry visible from repoDir, in git's read order.
func (g *Git) CredentialEntries(ctx context.Context, repoDir string) ([]Entry, error) {
	return g.configEntries(ctx, repoDir, `^(credential|awsssohub)\.`)
}

// GlobalEntries returns the credential.* entries of the system and global files only (run outside any repository).
func (g *Git) GlobalEntries(ctx context.Context) ([]Entry, error) {
	es, err := g.configEntries(ctx, "", `^credential\.`)
	if err != nil {
		return nil, err
	}
	out := es[:0]
	for _, e := range es {
		if e.Scope == "system" || e.Scope == "global" {
			out = append(out, e)
		}
	}
	return out, nil
}
