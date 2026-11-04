//go:build production && !sandbox

package paths

import "testing"

func TestResolveIgnoresSandboxEnvInReleaseBuilds(t *testing.T) {
	isolateHome(t)
	t.Setenv(SandboxEnv, t.TempDir())
	p, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if p.Sandbox || p.SandboxRoot != "" {
		t.Errorf("a release build must ignore %s: %+v", SandboxEnv, p)
	}
}
