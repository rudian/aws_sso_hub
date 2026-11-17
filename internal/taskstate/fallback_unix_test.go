//go:build !windows

package taskstate

import "testing"

func TestParsePSAndRealTableSmoke(t *testing.T) {
	ps := parsePS([]byte("  1     1 /sbin/launchd\n 42  42 /Applications/My App/bin/x y\nbad line\n"))
	if len(ps) != 2 || ps[1].Name != "/Applications/My App/bin/x y" || ps[1].PGID != 42 {
		t.Fatalf("%+v", ps)
	}
	got, err := PSTable{}.List()
	if err != nil || len(got) == 0 {
		t.Fatalf("%v %d", err, len(got))
	}
}
