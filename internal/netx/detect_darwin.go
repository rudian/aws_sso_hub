//go:build darwin

package netx

import (
	"context"
	"os/exec"
)

func systemConfig(ctx context.Context) (Config, bool) {
	out, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return Config{}, false
	}
	return ParseScutil(string(out)), true
}
