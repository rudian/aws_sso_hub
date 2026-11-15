package watch

import (
	"path/filepath"
	"strings"
)

// defaultIgnoredNames are folder (or file) names that are never reported, at any depth.
var defaultIgnoredNames = []string{
	"node_modules", "__pycache__", ".venv", ".next", ".nuxt", ".gradle",
	".terraform", ".DS_Store", ".mypy_cache", ".pytest_cache",
}

// gitNoisy are the entries of a git directory that change on every git
// operation without telling a status view anything new. They match by exact
// name, so a branch called "logs-cleanup" is not noise.
var gitNoisy = map[string]bool{"objects": true, "lfs": true, "logs": true}

// gitNoisyPrefix are matched as name prefixes (the daemon's socket has a suffix).
const gitNoisyPrefix = "fsmonitor--daemon"

type ignorer struct {
	names map[string]bool
}

func newIgnorer(extra []string) *ignorer {
	ig := &ignorer{names: map[string]bool{}}
	for _, n := range defaultIgnoredNames {
		ig.names[strings.ToLower(n)] = true
	}
	for _, n := range extra {
		if n = strings.TrimSpace(n); n != "" {
			ig.names[strings.ToLower(n)] = true
		}
	}
	return ig
}

// ignored reports whether a path relative to the watched folder is noise.
// Names compare case-insensitively: both macOS and Windows volumes usually are.
func (ig *ignorer) ignored(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	segs := strings.Split(filepath.ToSlash(rel), "/")
	for i, s := range segs {
		ls := strings.ToLower(s)
		if ig.names[ls] {
			return true
		}
		if ls == ".git" {
			if gitDirNoise(segs[i+1:]) {
				return true
			}
			// index.lock, HEAD.lock, packed-refs.lock ... come and go with every git command.
			if strings.HasSuffix(strings.ToLower(segs[len(segs)-1]), ".lock") {
				return true
			}
		}
	}
	return false
}

// gitDirNoise judges the segments after a ".git" folder.
func gitDirNoise(rest []string) bool {
	if len(rest) == 0 {
		return false
	}
	first := strings.ToLower(rest[0])
	if gitNoisy[first] || strings.HasPrefix(first, gitNoisyPrefix) {
		return true
	}
	switch first {
	case "worktrees":
		// .git/worktrees/<name>/logs/... : the name is never noise.
		if len(rest) > 2 {
			return gitNoisy[strings.ToLower(rest[2])]
		}
	case "modules":
		// .git/modules/<name>/objects/... : each module directory is a git
		// directory of its own; <name> may span segments and may itself be
		// called "logs", so only segments after the first count.
		for _, seg := range rest[min(2, len(rest)):] {
			if gitNoisy[strings.ToLower(seg)] || strings.HasPrefix(strings.ToLower(seg), gitNoisyPrefix) {
				return true
			}
		}
	}
	return false
}
