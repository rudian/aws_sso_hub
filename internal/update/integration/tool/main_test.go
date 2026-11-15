package main

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The harness API must answer the repository-access check (GET /repos/{repo}),
// which non-production channels run when no release is found.
func TestServeAnswersRepoAccessCheck(t *testing.T) {
	dir := t.TempDir()
	addr := filepath.Join(dir, "addr")
	go serve(dir, "acme/codepass-test", addr)
	var base string
	for i := 0; i < 100; i++ {
		if b, err := os.ReadFile(addr); err == nil && len(b) > 0 {
			base = string(b)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if base == "" {
		t.Fatal("server did not start")
	}
	resp, err := http.Get(base + "/repos/acme/codepass-test")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("repo check status %d, want 200", resp.StatusCode)
	}
}
