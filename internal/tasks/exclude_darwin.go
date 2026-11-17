package tasks

import (
	"context"
	"os/exec"
	"time"
)

// ExcludeFromBackup keeps a folder out of Time Machine (a sticky exclusion, no administrator needed): what
// <data>/sessions holds is conversation and terminal text (plan 3.1). The program has an absolute path and the
// folder is one argument.
func ExcludeFromBackup(dir string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "/usr/bin/tmutil", "addexclusion", dir).Run()
}
