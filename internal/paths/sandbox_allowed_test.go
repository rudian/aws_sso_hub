//go:build !production || sandbox

package paths

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolveSandboxEnv(t *testing.T) {
	isolateHome(t)
	root := t.TempDir()
	t.Setenv(SandboxEnv, root)
	// A real AWS_CONFIG_FILE must not leak into sandbox mode.
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "other"))
	p, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	want := ForRoot(root)
	if !reflect.DeepEqual(p, want) {
		t.Errorf("Resolve in sandbox = %+v, want %+v", p, want)
	}
}
