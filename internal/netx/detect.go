package netx

import (
	"context"
	"time"
)

// Detect returns the effective proxy: environment variables win (they are an
// explicit choice, and set when launched from a terminal), then the system
// settings. A failing system query yields an empty Config, never an error.
func Detect(ctx context.Context) Config {
	if c := FromEnv(nil); !c.Empty() {
		return c
	}
	return System(ctx)
}

// System reads the operating system's proxy settings.
func System(ctx context.Context) Config {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if c, ok := systemConfig(ctx); ok {
		return c
	}
	return Config{Source: "none"}
}
