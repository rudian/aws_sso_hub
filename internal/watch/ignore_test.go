package watch

import "testing"

func TestIgnorer(t *testing.T) {
	ig := newIgnorer([]string{"vendor"})
	cases := map[string]bool{
		"":                                     false,
		"main.go":                              false,
		"src/app.ts":                           false,
		"node_modules/x/index.js":              true,
		"web/node_modules":                     true,
		"Node_Modules/a":                       true,
		"a/__pycache__/m.pyc":                  true,
		".DS_Store":                            true,
		"src/.DS_Store":                        true,
		"vendor/x.go":                          true,
		".git/HEAD":                            false,
		".git/index":                           false,
		".git/refs/heads/main":                 false,
		".git/objects/ab/cdef":                 true,
		".git/logs/HEAD":                       true,
		".git/index.lock":                      true,
		".git/refs/heads/main.lock":            true,
		".git/modules/sub/objects/12":          true,
		"docs/index.lock":                      false, // a .lock outside .git is a real file
		".gitignore":                           false,
		"src/gitobjects/objects":               false,
		"sub/.git/lfs/objects/aa":              true,
		"sub/.git/fsmonitor--daemon.ipc":       true,
		".git/refs/heads/logs-cleanup":         false,
		".git/refs/heads/lfs-migration":        false,
		".git/refs/remotes/origin/objects-api": false,
		".git/refs/tags/logstash-1.0":          false,
		".git/worktrees/logs-fix/HEAD":         false,
		".git/modules/lfs-tools/HEAD":          false,
		".git/modules/logs/HEAD":               false,
		".git/worktrees/x/logs/HEAD":           true,
		".git/modules/a/logs/HEAD":             true,
		".git/logs":                            true,
		".git/lfs/tmp/x":                       true,
	}
	for rel, want := range cases {
		if got := ig.ignored(rel); got != want {
			t.Errorf("ignored(%q) = %v, want %v", rel, got, want)
		}
	}
}
