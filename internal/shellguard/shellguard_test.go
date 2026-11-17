package shellguard

import (
	"errors"
	"testing"
)

func TestLoginOrInteractive(t *testing.T) {
	for _, a := range [][]string{{"-l", "-c", "x"}, {"-i"}, {"-lic", "x"}, {"-ilc", "x"}, {"--login"}, {"-l"},
		{"-o", "pipefail", "-l", "-c", "x"}, {"-O", "extglob", "-l", "-c", "x"}, {"--rcfile", "F", "-i"}, {"-o", "login", "-c", "x"}, {"-o", "interactive"}} {
		if !LoginOrInteractive(a) {
			t.Errorf("%v should be login/interactive", a)
		}
	}
	for _, a := range [][]string{{"-c", "ls -l"}, {"script.sh", "-l"}, {"-o", "pipefail", "-c", "x"}, {"--norc", "-c", "x"}, {}} {
		if LoginOrInteractive(a) {
			t.Errorf("%v should not be login/interactive", a)
		}
	}
}

func TestInertInAProductionBinary(t *testing.T) {
	ResetRefused()
	old := active
	active = func() bool { return false }
	defer func() { active = old }()
	if err := Check("/bin/zsh", []string{"-l", "-i", "-c", "true"}, []string{"HOME=/Users/someone"}); err != nil {
		t.Fatal(err)
	}
	if len(Refused()) != 0 {
		t.Fatalf("recorded %v", Refused())
	}
}

func TestRefusesNonTempHomeAndRecords(t *testing.T) {
	ResetRefused()
	err := Check("/bin/zsh", []string{"-l", "-c", "true"}, []string{"HOME=/Users/someone"})
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	if len(Refused()) != 1 {
		t.Fatalf("refused = %v", Refused())
	}
	if err := Check("/bin/bash", []string{"-lic", "x"}, []string{"HOME=" + t.TempDir(), "ZDOTDIR=/Users/someone"}); !errors.Is(err, ErrRefused) {
		t.Fatalf("ZDOTDIR outside temp: %v", err)
	}
	if err := Check("sh", []string{"-i"}, []string{}); !errors.Is(err, ErrRefused) {
		t.Fatalf("no HOME: %v", err)
	}
}

func TestAllowsTempHomeAndNonLoginShells(t *testing.T) {
	ResetRefused()
	home := t.TempDir()
	if err := Check("/bin/zsh", []string{"-l", "-c", "true"}, []string{"HOME=" + home, "ZDOTDIR=" + home}); err != nil {
		t.Fatal(err)
	}
	if err := Check("/bin/bash", []string{"-c", "true"}, []string{"HOME=/Users/someone"}); err != nil {
		t.Fatal(err) // not a login or interactive shell
	}
	if err := Check("/bin/zsh", []string{"-f", "-c", "true"}, []string{"HOME=/Users/someone"}); err != nil {
		t.Fatal(err) // zsh -f reads no rc files
	}
	if err := Check("/usr/bin/git", []string{"-l"}, []string{"HOME=/Users/someone"}); err != nil {
		t.Fatal(err) // not a shell
	}
	if len(Refused()) != 0 {
		t.Fatalf("refused = %v", Refused())
	}
}

func TestNilEnvUsesProcessEnv(t *testing.T) {
	t.Setenv("HOME", "/Users/someone")
	if err := Check("zsh", []string{"-l"}, nil); !errors.Is(err, ErrRefused) {
		t.Fatalf("err = %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ZDOTDIR", "")
	if err := Check("zsh", []string{"-l"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestZshAndFishReadFilesOnEveryStart(t *testing.T) {
	ResetRefused()
	bad := []string{"HOME=/Users/someone"}
	if err := Check("/bin/zsh", []string{"-c", "echo ran"}, bad); !errors.Is(err, ErrRefused) {
		t.Fatalf("zsh -c: %v", err)
	}
	if err := Check("/bin/zsh", []string{"-c", "x"}, []string{"PATH=/bin"}); !errors.Is(err, ErrRefused) {
		t.Fatalf("zsh -c without HOME: %v", err)
	}
	if err := Check("/usr/bin/fish", []string{"-c", "x"}, bad); !errors.Is(err, ErrRefused) {
		t.Fatalf("fish -c: %v", err)
	}
	home := t.TempDir()
	if err := Check("fish", []string{"-c", "x"}, []string{"HOME=" + home, "XDG_CONFIG_HOME=/Users/someone/.config"}); !errors.Is(err, ErrRefused) {
		t.Fatalf("fish XDG_CONFIG_HOME: %v", err)
	}
	if err := Check("bash", []string{"-l", "-c", "x"}, []string{"HOME=" + home, "BASH_ENV=/Users/someone/.env"}); !errors.Is(err, ErrRefused) {
		t.Fatalf("BASH_ENV: %v", err)
	}
	if err := Check("zsh", []string{"-c", "x"}, []string{"HOME=" + home}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckTerminalTreatsABareShellAsInteractive(t *testing.T) {
	ResetRefused()
	bad := []string{"HOME=/Users/someone"}
	if err := CheckTerminal("/bin/bash", nil, bad); !errors.Is(err, ErrRefused) {
		t.Fatalf("bare bash: %v", err)
	}
	if err := CheckTerminal("/bin/zsh", nil, bad); !errors.Is(err, ErrRefused) {
		t.Fatalf("bare zsh: %v", err)
	}
	if err := CheckTerminal("/bin/bash", []string{"-c", "true"}, bad); err != nil {
		t.Fatal(err)
	}
	if err := CheckTerminal("/bin/bash", []string{"script.sh"}, bad); err != nil {
		t.Fatal(err)
	}
	if err := Check("/bin/bash", nil, bad); err != nil {
		t.Fatal(err) // no terminal: not interactive
	}
}

func TestShellNamedByTheSHELLVariableIsAShell(t *testing.T) {
	ResetRefused()
	env := []string{"HOME=/Users/someone", "SHELL=/opt/homebrew/bin/nu"}
	if err := Check("/opt/homebrew/bin/nu", []string{"-l", "-c", "x"}, env); !errors.Is(err, ErrRefused) {
		t.Fatalf("nu: %v", err)
	}
	if err := Check("/opt/homebrew/bin/nu", []string{"-l", "-c", "x"}, []string{"HOME=/Users/someone"}); err != nil {
		t.Fatalf("nu is not the shell here: %v", err) // SHELL unset in env and process
	}
}

func TestRefuseInTest(t *testing.T) {
	ResetRefused()
	if err := RefuseInTest("open a terminal window"); !errors.Is(err, ErrRefused) || len(Refused()) != 1 {
		t.Fatalf("%v %v", err, Refused())
	}
	old := active
	active = func() bool { return false }
	defer func() { active = old }()
	if err := RefuseInTest("x"); err != nil {
		t.Fatal(err)
	}
}

func TestNoStartupFileFlagsAreShellSpecific(t *testing.T) {
	ResetRefused()
	bad := []string{"HOME=/Users/someone"}
	refusedCases := []struct {
		prog string
		args []string
		term bool
	}{
		{"/bin/zsh", []string{"-N", "-c", "x"}, false},  // zsh -N is AUTO_PUSHD
		{"/bin/zsh", []string{"-N"}, true},              // interactive zsh -N
		{"fish", []string{"-f", "x", "-c", "y"}, false}, // fish -f takes FEATURES
		{"fish", []string{"--features", "qmark-noglob", "-c", "y"}, false},
		{"/usr/local/bin/fish", []string{"-f", "qmark-noglob", "-c", "x"}, false},
		{"/bin/zsh", []string{"--no-config", "-c", "x"}, false},
		{"fish", []string{"--no-rcs", "-c", "x"}, false},
		{"/bin/tcsh", []string{"-c", "x"}, false},
	}
	for _, c := range refusedCases {
		var err error
		if c.term {
			err = CheckTerminal(c.prog, c.args, bad)
		} else {
			err = Check(c.prog, c.args, bad)
		}
		if !errors.Is(err, ErrRefused) {
			t.Errorf("%s %v: want refused, got %v", c.prog, c.args, err)
		}
	}
	allowed := []struct {
		prog string
		args []string
	}{
		{"fish", []string{"-N", "-c", "x"}},
		{"fish", []string{"--no-config", "-c", "x"}},
		{"/bin/zsh", []string{"-f", "-c", "x"}},
		{"/bin/zsh", []string{"-fc", "x"}},
		{"/bin/zsh", []string{"--no-rcs", "-c", "x"}},
		{"/bin/zsh", []string{"-o", "norcs", "-c", "x"}},
		{"/bin/tcsh", []string{"-f", "-c", "x"}},
	}
	for _, c := range allowed {
		if err := Check(c.prog, c.args, bad); err != nil {
			t.Errorf("%s %v: want allowed, got %v", c.prog, c.args, err)
		}
	}
}
