//go:build !darwin && !windows

package netx

import "context"

func systemConfig(context.Context) (Config, bool) { return Config{}, false }
