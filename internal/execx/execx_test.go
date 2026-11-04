package execx

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func envHas(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func envKey(env []string, key string) bool {
	for _, e := range env {
		if strings.HasPrefix(e, key+"=") {
			return true
		}
	}
	return false
}

func TestChildEnv(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
	t.Setenv("AWS_PROFILE", "x")
	t.Setenv("GIT_ASKPASS", "/bin/true")
	t.Setenv("AWS_PAGER", "less")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	Configure(Options{ExtraEnv: []string{"HOME=/sandbox/home", "ZED=1"}, DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	env := ChildEnv()
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_PROFILE", "GIT_ASKPASS"} {
		if envKey(env, k) {
			t.Errorf("%s not stripped", k)
		}
	}
	for _, kv := range []string{"AWS_PAGER=", "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never"} {
		if !envHas(env, kv) {
			t.Errorf("missing %s", kv)
		}
	}
	if !envKey(env, "PATH") {
		t.Error("PATH missing")
	}
	if n := len(env); n < 2 || env[n-2] != "HOME=/sandbox/home" || env[n-1] != "ZED=1" {
		t.Errorf("ExtraEnv must be last: %v", env[len(env)-2:])
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "AWS_PAGER=") && kv != "AWS_PAGER=" {
			t.Errorf("AWS_PAGER duplicated: %s", kv)
		}
	}
}

func fakeExe(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLookPathPreferredDirs(t *testing.T) {
	dir := t.TempDir()
	want := fakeExe(t, dir, "fakeawsx")
	Configure(Options{PreferredDirs: []string{dir}, DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	if !strings.HasPrefix(EnvPATH(), dir+string(os.PathListSeparator)) {
		t.Errorf("preferred dir not first: %s", EnvPATH())
	}
	got, err := LookPath("fakeawsx")
	if err != nil || got != want {
		t.Fatalf("LookPath = %q, %v; want %q", got, err, want)
	}
	if _, err := LookPath("definitely-not-a-binary-xyz"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFindAWSOverride(t *testing.T) {
	dir := t.TempDir()
	p := fakeExe(t, dir, "aws")
	Configure(Options{PreferredDirs: []string{}, DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	got, err := FindAWS(p)
	if err != nil || got != p {
		t.Fatalf("FindAWS(override) = %q, %v", got, err)
	}
	if _, err := FindAWS(filepath.Join(dir, "missing")); err != ErrNotFound {
		t.Errorf("missing override: %v", err)
	}
	if _, err := FindAWS(dir); err != ErrNotFound {
		t.Errorf("directory override: %v", err)
	}
}

func TestFindAWSKeepsSymlinkPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	dir := t.TempDir()
	real := fakeExe(t, dir, "real-aws")
	link := filepath.Join(dir, "aws")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	Configure(Options{PreferredDirs: []string{dir}, DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	got, err := FindAWS("")
	if err != nil || got != link {
		t.Fatalf("FindAWS = %q, %v; want %q (unresolved)", got, err, link)
	}
}

func TestHelperPath(t *testing.T) {
	if got := HelperPath(`C:\Program Files\Amazon\AWSCLIV2\aws.exe`, "windows"); got != "C:/Program Files/Amazon/AWSCLIV2/aws.exe" {
		t.Errorf("windows: %q", got)
	}
	if got := HelperPath("/opt/homebrew/bin/aws", "darwin"); got != "/opt/homebrew/bin/aws" {
		t.Errorf("darwin: %q", got)
	}
	if got := HelperPath(`/a\b`, "darwin"); got != `/a\b` {
		t.Errorf("darwin must not rewrite: %q", got)
	}
}

func TestDedupe(t *testing.T) {
	got := Dedupe([]string{"/a", "/b", "/a", "", "/B"}, false)
	if strings.Join(got, ",") != "/a,/b,/B" {
		t.Errorf("case-sensitive: %v", got)
	}
	got = Dedupe([]string{"C:\\A", "c:\\a", "D"}, true)
	if strings.Join(got, ",") != `C:\A,D` {
		t.Errorf("fold: %v", got)
	}
}

func TestRunReportsExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	Configure(Options{DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	res, err := Run(Command(context.Background(), "/bin/sh", "-c", "echo out; echo err >&2; exit 3"), nil)
	if err != nil || res.ExitCode != 3 || strings.TrimSpace(string(res.Stdout)) != "out" || strings.TrimSpace(string(res.Stderr)) != "err" {
		t.Fatalf("unexpected: %+v %v", res, err)
	}
	res, err = Run(Command(context.Background(), "/bin/cat"), []byte("hello"))
	if err != nil || string(res.Stdout) != "hello" {
		t.Fatalf("stdin: %+v %v", res, err)
	}
	if _, err := Run(Command(context.Background(), "/no/such/binary"), nil); err == nil {
		t.Error("expected start error")
	}
}

func TestChildEnvDropsGitRedirection(t *testing.T) {
	set := map[string]string{
		"GIT_DIR": "/tmp/x", "GIT_WORK_TREE": "/tmp/x", "GIT_INDEX_FILE": "/tmp/x", "GIT_SSH_COMMAND": "touch x",
		"GIT_SSH": "x", "GIT_PROXY_COMMAND": "x", "GIT_EXEC_PATH": "/tmp/x", "GIT_CONFIG_COUNT": "1",
		"GIT_CONFIG_KEY_0": "core.pager", "GIT_CONFIG_VALUE_0": "x", "GIT_CONFIG_PARAMETERS": "'a.b=c'",
		"GIT_CEILING_DIRECTORIES": "/tmp", "GIT_NAMESPACE": "n", "GIT_COMMON_DIR": "/tmp/x",
		"GIT_OBJECT_DIRECTORY": "/tmp/x", "GIT_ALTERNATE_OBJECT_DIRECTORIES": "/tmp/x",
	}
	for k, v := range set {
		t.Setenv(k, v)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", "/tmp/global.cfg")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	Configure(Options{DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	env := ChildEnv()
	for k := range set {
		if envKey(env, k) {
			t.Errorf("%s must not reach children", k)
		}
	}
	if !envHas(env, "GIT_CONFIG_GLOBAL=/tmp/global.cfg") || !envHas(env, "GIT_CONFIG_NOSYSTEM=1") {
		t.Error("GIT_CONFIG_GLOBAL and GIT_CONFIG_NOSYSTEM must be kept")
	}
}

func TestLookPathIgnoresRelativeEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix PATH semantics")
	}
	bin := t.TempDir()
	fakeExe(t, bin, "relpathtool")
	cwd := t.TempDir()
	fakeExe(t, cwd, "relpathtool2")
	t.Chdir(cwd)
	t.Setenv("PATH", ".:"+bin)
	Configure(Options{PreferredDirs: []string{}, DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	if _, err := LookPath("relpathtool2"); err != ErrNotFound {
		t.Errorf("a program in the working directory must not be found through PATH: %v", err)
	}
	if got, err := LookPath("relpathtool"); err != nil || got != filepath.Join(bin, "relpathtool") {
		t.Errorf("absolute entry: %q %v", got, err)
	}
	if _, err := LookPath("./relpathtool2"); err != ErrNotFound {
		t.Errorf("a relative path with a separator must be refused: %v", err)
	}
}

func TestValidateToolPathWindowsSyntax(t *testing.T) {
	good := []string{`C:\Program Files\Amazon\AWSCLIV2\aws.exe`, `C:/Git/cmd/GIT.EXE`}
	for _, p := range good {
		kind := "aws"
		if strings.Contains(strings.ToLower(p), "git") {
			kind = "git"
		}
		if err := ValidateToolPath(kind, p, "windows"); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	bad := map[string]string{
		`bin\aws.exe`:                 `aws`,
		`C:\tools\build.exe`:          `aws`,
		`C:\tools\aws.cmd`:            `aws`,
		`C:\tools "x"\aws.exe`:        `aws`,
		"C:\\tools\\aws.exe\n":        `aws`,
		`C:\tools\git.exe`:            `aws`,
		`\\server\share\aws.exe`:      ``, // accepted shape; covered below
		`C:\tools\aws.exe` + "\u2028": `aws`,
	}
	for p, kind := range bad {
		if err := ValidateToolPath(kind, p, "windows"); err == nil {
			t.Errorf("%q must be rejected", p)
		}
	}
	if err := ValidateToolPath("aws", "", "windows"); err != nil {
		t.Errorf("empty means automatic: %v", err)
	}
}

func TestValidateToolPathUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	root := t.TempDir()
	mk := func(dirMode os.FileMode, rel string, mode os.FileMode) string {
		t.Helper()
		d := filepath.Join(root, filepath.Dir(rel))
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, dirMode); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(root, rel)
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ok := mk(0o700, "ok/aws", 0o755)
	if err := ValidateToolPath("aws", ok, runtime.GOOS); err != nil {
		t.Errorf("0755 aws in a 0700 folder: %v", err)
	}
	if err := ValidateToolPath("git", ok, runtime.GOOS); err == nil {
		t.Error("an aws file must not pass as git")
	}
	if err := ValidateToolPath("aws", "", runtime.GOOS); err != nil {
		t.Errorf("empty: %v", err)
	}
	// A symlink (Homebrew style) to a safe file is accepted.
	link := filepath.Join(root, "ok", "git")
	target := mk(0o700, "ok/realgit", 0o755)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := ValidateToolPath("git", link, runtime.GOOS); err != nil {
		t.Errorf("symlink: %v", err)
	}
	cases := map[string]string{
		"relative":       "bin/aws",
		"wrong name":     mk(0o700, "wrong/build.sh", 0o755),
		"not executable": mk(0o700, "noexec/aws", 0o644),
		"writable dir":   mk(0o777, "open/aws", 0o755),
		"writable file":  mk(0o700, "wfile/aws", 0o775),
		"quote":          mk(0o700, `q"x/aws`, 0o755),
		"missing":        filepath.Join(root, "missing", "aws"),
	}
	if err := os.MkdirAll(filepath.Join(root, "isdir", "aws"), 0o700); err != nil {
		t.Fatal(err)
	}
	cases["directory"] = filepath.Join(root, "isdir", "aws")
	for name, p := range cases {
		if err := ValidateToolPath("aws", p, runtime.GOOS); err == nil {
			t.Errorf("%s (%s) must be rejected", name, p)
		}
	}
}

func TestFindRejectsInvalidOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "build.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	Configure(Options{PreferredDirs: []string{}, DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	for _, p := range []string{script, "aws", "bin/git"} {
		if _, err := FindAWS(p); err != ErrNotFound {
			t.Errorf("FindAWS(%q) = %v", p, err)
		}
		if _, err := FindGit(context.Background(), p); err != ErrNotFound {
			t.Errorf("FindGit(%q) = %v", p, err)
		}
	}
}

func TestStartRunsDetachedWithEnvAndDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	t.Setenv("AWS_PROFILE", "inherited")
	Configure(Options{DisableLoginShell: true})
	t.Cleanup(func() { Configure(Options{}) })
	dir := t.TempDir()
	if err := Start("/bin/sh", []string{"-c", `printf '%s %s' "$AWS_PROFILE" "$PWD" > marker`}, dir, []string{"AWS_PROFILE=initech-admin"}, true); err != nil {
		t.Fatal(err)
	}
	var b []byte
	for i := 0; i < 100; i++ {
		var err error
		if b, err = os.ReadFile(filepath.Join(dir, "marker")); err == nil && len(b) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := string(b)
	if !strings.HasPrefix(got, "initech-admin ") {
		t.Errorf("extra env missing: %q", got)
	}
	if err := Start("/no/such/program", nil, "", nil, false); err == nil {
		t.Error("expected start error")
	}
}
