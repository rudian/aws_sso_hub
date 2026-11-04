package logx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	lg, c, err := New(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	lg.Info("login aws_secret_access_key=abc", "line", "aws_secret_access_key=abc", "key", "AKIAIOSFODNN7EXAMPLE")
	lg.Error("failed", "err", errors.New(`accessToken: "ACCESS-TOKEN-SECRET"`))
	lg.Debug("hidden at info level")
	c.Close()
	b, err := os.ReadFile(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "••••••••") {
		t.Fatalf("mask missing:\n%s", s)
	}
	for _, bad := range []string{"abc", "AKIAIOSFODNN7EXAMPLE", "ACCESS-TOKEN-SECRET", "hidden"} {
		if strings.Contains(s, bad) {
			t.Fatalf("%q leaked:\n%s", bad, s)
		}
	}
}

func TestRotates(t *testing.T) {
	dir := t.TempDir()
	w := &rotator{path: filepath.Join(dir, "app.log"), max: 4096}
	if err := w.open(); err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	line := strings.Repeat("x", 1000) + "\n"
	for i := 0; i < 60; i++ {
		if _, err := w.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"app.log", "app.log.1", "app.log.2", "app.log.3"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "app.log.4")); err == nil {
		t.Fatal("only 3 old files are kept")
	}
	if st, _ := os.Stat(filepath.Join(dir, "app.log")); st.Size() > 4096 {
		t.Fatalf("log too large: %d", st.Size())
	}
}
