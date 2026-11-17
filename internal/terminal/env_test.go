package terminal

import (
	"strings"
	"testing"
)

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, kv := range env {
		i := strings.IndexByte(kv, '=')
		m[kv[:i]] = kv[i+1:]
	}
	return m
}

func TestBuildEnvContents(t *testing.T) {
	base := []string{
		"PATH=/usr/bin", "HOME=/home/alex", "TERM=dumb", "COLORTERM=no", "PWD=/stale", "OLDPWD=/stale",
		"AWSSSOHUB_SANDBOX=/tmp/x", "AWSSSOHUB_FOO=1", "WAILS_DEBUG=1", "WAILS_VITE_PORT=9245", "CODEPASS_TASK=old",
		"HTTPS_PROXY=http://proxy.example.com:3128", "NO_PROXY=localhost", "LANG=C", "AWS_PROFILE=initech-admin",
	}
	got := BuildEnv(base, "darwin", []string{"CODEPASS_TASK=t-1", "CLAUDE_CONFIG_DIR=/tmp/cfg", "AWSSSOHUB_X=2", "WAILS_Y=3"})
	m := envMap(got)
	if m["TERM"] != "xterm-256color" || m["COLORTERM"] != "truecolor" {
		t.Errorf("TERM/COLORTERM: %v", m)
	}
	if m["LANG"] != "en_US.UTF-8" {
		t.Errorf("a non-UTF-8 LANG must be replaced: %q", m["LANG"])
	}
	for k := range m {
		if strings.HasPrefix(k, "AWSSSOHUB_") || strings.HasPrefix(k, "WAILS_") {
			t.Errorf("%s leaked into the child environment", k)
		}
	}
	if _, ok := m["PWD"]; ok {
		t.Error("PWD must not be inherited")
	}
	if m["CODEPASS_TASK"] != "t-1" || m["CLAUDE_CONFIG_DIR"] != "/tmp/cfg" {
		t.Errorf("extra variables missing or the app's own CODEPASS_TASK inherited: %v", m)
	}
	if m["PATH"] != "/usr/bin" || m["HOME"] != "/home/alex" || m["HTTPS_PROXY"] == "" || m["NO_PROXY"] != "localhost" || m["AWS_PROFILE"] != "initech-admin" {
		t.Errorf("inherited variables lost: %v", m)
	}
	seen := map[string]bool{}
	for _, kv := range got {
		k := kv[:strings.IndexByte(kv, '=')]
		if seen[k] {
			t.Errorf("duplicate %s", k)
		}
		seen[k] = true
	}
}

func TestBuildEnvKeepsAUTF8LangAndAddsOneWhenMissing(t *testing.T) {
	if m := envMap(BuildEnv([]string{"LANG=zh_CN.UTF-8"}, "darwin", nil)); m["LANG"] != "zh_CN.UTF-8" {
		t.Errorf("%v", m)
	}
	if m := envMap(BuildEnv(nil, "darwin", nil)); m["LANG"] != "en_US.UTF-8" {
		t.Errorf("apps started from Finder have no LANG: %v", m)
	}
}

func TestBuildEnvWindowsNamesAreCaseInsensitive(t *testing.T) {
	got := BuildEnv([]string{"Path=C:\\Windows", "wails_x=1", "AwsSsoHub_Y=1", "term=dumb", "=C:=C:\\"}, "windows", []string{"PATH=C:\\tools"})
	m := envMap(got)
	if len(m) != 4 { // Path, TERM, COLORTERM, LANG
		t.Fatalf("%v", got)
	}
	if m["PATH"] != "C:\\tools" || m["TERM"] != "xterm-256color" {
		t.Fatalf("%v", got)
	}
}

func TestLaunchArgv(t *testing.T) {
	got := LaunchArgv("darwin", "/bin/zsh", "claude", "--session-id", "x y", "--", "-h; rm -rf ~")
	want := []string{"/bin/zsh", "-l", "-i", "-c", `"$@"`, "codepass", "claude", "--session-id", "x y", "--", "-h; rm -rf ~"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("%q", got)
	}
	if w := LaunchArgv("windows", "", "claude.exe", "--resume"); strings.Join(w, " ") != "claude.exe --resume" {
		t.Fatalf("%q", w)
	}
	if DefaultShell("darwin", "not/absolute") != "/bin/zsh" || DefaultShell("linux", "") != "/bin/sh" {
		t.Fatal("default shell")
	}
}
