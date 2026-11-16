package notify

import "testing"

func TestChooseReal(t *testing.T) {
	on := func(string) string { return "1" }
	off := func(string) string { return "" }
	cases := []struct {
		name                        string
		production, sandbox, inTest bool
		env                         func(string) string
		want                        bool
	}{
		{"production build", true, false, false, off, true},
		{"production build, sandbox variable set", true, true, false, on, false},
		{"development build", false, false, false, off, false},
		{"development build with the opt-in", false, false, false, on, true},
		{"development build with the opt-in in sandbox mode", false, true, false, on, false},
		{"a test binary never uses the system, whatever the opt-in", false, false, true, on, false},
		{"a test binary of a production build", true, false, true, off, false},
	}
	for _, c := range cases {
		if got := ChooseReal(c.production, c.sandbox, c.env, c.inTest); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	// only the exact value "1" opts in
	for _, v := range []string{"", "0", "true", "yes", " 1"} {
		v := v
		if ChooseReal(false, false, func(string) string { return v }, false) {
			t.Errorf("%q must not opt in", v)
		}
	}
}

// go test is a development build run by a test binary: UseReal is false even with the opt-in set.
func TestUseRealIsFalseUnderGoTest(t *testing.T) {
	t.Setenv(EnvRealNotifications, "1")
	if UseReal(false) || UseReal(true) {
		t.Fatal("a test must never reach the system's notification service")
	}
}
