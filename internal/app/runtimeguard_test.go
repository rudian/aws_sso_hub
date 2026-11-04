package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/transfer"
)

func serveThroughGuard(t *testing.T, method, target, body string, header map[string]string) (code int, reached bool, seen string) {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		b, _ := io.ReadAll(r.Body)
		seen = string(b)
		w.WriteHeader(http.StatusOK)
	})
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	RuntimeGuard(next).ServeHTTP(rec, req)
	return rec.Code, reached, seen
}

func TestRuntimeGuardBlocksBrowserAndClipboardRead(t *testing.T) {
	for name, tc := range map[string]struct{ method, target, body string }{
		"browser open url (json)":   {"POST", "/wails/runtime", `{"object":9,"method":0,"args":{"url":"file:///etc/hosts"}}`},
		"browser any method (json)": {"POST", "/wails/runtime", `{"object":9,"method":3,"args":{}}`},
		"browser (query)":           {"POST", "/wails/runtime?object=9&method=0&args=%7B%7D", ""},
		"browser (get with query)":  {"GET", "/wails/runtime?object=9&method=0", ""},
		"clipboard text (json)":     {"POST", "/wails/runtime", `{"object":1,"method":1}`},
		"clipboard text (query)":    {"POST", "/wails/runtime?object=1&method=1", ""},
		"object order and spacing":  {"POST", "/wails/runtime", ` { "method" : 0 , "args": {"a":1}, "object" : 9 } `},
	} {
		code, reached, _ := serveThroughGuard(t, tc.method, tc.target, tc.body, nil)
		if code != http.StatusForbidden || reached {
			t.Errorf("%s: code=%d reached=%v", name, code, reached)
		}
	}
}

func TestRuntimeGuardRefusesChunkedBodies(t *testing.T) {
	code, reached, _ := serveThroughGuard(t, "POST", "/wails/runtime", `{"object":0,"method":0}`, map[string]string{"x-wails-chunk-id": "abc", "x-wails-chunk-index": "0", "x-wails-chunk-total": "2"})
	if code != http.StatusForbidden || reached {
		t.Errorf("code=%d reached=%v", code, reached)
	}
}

func TestRuntimeGuardPassesOtherCallsUnchanged(t *testing.T) {
	big := `{"object":0,"method":0,"args":{"methodID":123,"args":["` + strings.Repeat("x", 300000) + `"]}}`
	for name, body := range map[string]string{
		"clipboard set text": `{"object":1,"method":0,"args":{"text":"hello"}}`,
		"service call":       `{"object":0,"method":0,"args":{"methodID":42,"args":[]}}`,
		"window set title":   `{"object":6,"method":43,"args":{"title":"x"}}`,
		"events emit":        `{"object":3,"method":0,"args":{"name":"a"}}`,
		"large call":         big,
	} {
		code, reached, seen := serveThroughGuard(t, "POST", "/wails/runtime", body, nil)
		if code != http.StatusOK || !reached || seen != body {
			t.Errorf("%s: code=%d reached=%v identical=%v", name, code, reached, seen == body)
		}
	}
	code, reached, _ := serveThroughGuard(t, "POST", "/wails/runtime?object=0&method=0", "", nil)
	if code != http.StatusOK || !reached {
		t.Errorf("query form of a service call: code=%d reached=%v", code, reached)
	}
}

func TestRuntimeGuardRejectsUnparseableRequests(t *testing.T) {
	for name, tc := range map[string]struct{ target, body string }{
		"not json":       {"/wails/runtime", `{`},
		"no object":      {"/wails/runtime", `{"method":0}`},
		"no method":      {"/wails/runtime", `{"object":0}`},
		"string object":  {"/wails/runtime", `{"object":"9","method":0}`},
		"empty":          {"/wails/runtime", ""},
		"bad query":      {"/wails/runtime?object=x&method=0", ""},
		"missing method": {"/wails/runtime?object=0", ""},
	} {
		code, reached, _ := serveThroughGuard(t, "POST", tc.target, tc.body, nil)
		if code != http.StatusBadRequest || reached {
			t.Errorf("%s: code=%d reached=%v", name, code, reached)
		}
	}
	// Bodies above the limit are refused, not forwarded.
	huge := `{"object":0,"method":0,"args":"` + strings.Repeat("x", maxRuntimeBody+1) + `"}`
	if code, reached, _ := serveThroughGuard(t, "POST", "/wails/runtime", huge, nil); code != http.StatusRequestEntityTooLarge || reached {
		t.Errorf("huge body: code=%d reached=%v", code, reached)
	}
}

func TestRuntimeGuardLeavesOtherPathsAlone(t *testing.T) {
	for _, target := range []string{"/", "/assets/index.js", "/wails/runtime.js", "/wails/runtime/extra", "/wails/flags"} {
		code, reached, _ := serveThroughGuard(t, "POST", target, `{"object":9,"method":0}`, nil)
		if code != http.StatusOK || !reached {
			t.Errorf("%s: code=%d reached=%v", target, code, reached)
		}
	}
	// The body of a pass-through request is readable by the next handler.
	req := httptest.NewRequest("POST", "/wails/runtime", bytes.NewBufferString(`{"object":0,"method":0}`))
	var got string
	RuntimeGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { b, _ := io.ReadAll(r.Body); got = string(b) })).ServeHTTP(httptest.NewRecorder(), req)
	if got != `{"object":0,"method":0}` {
		t.Errorf("body not restored: %q", got)
	}
}

// The object and method numbers in the guard are those of this Wails release; changing the dependency must
// come with a fresh look at them (messageprocessor.go, messageprocessor_clipboard.go).
func TestRuntimeGuardMatchesPinnedWailsVersion(t *testing.T) {
	mod, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`github.com/wailsapp/wails/v3\s+(v\S+)`).FindSubmatch(mod)
	if m == nil {
		t.Fatal("wails dependency not found in go.mod")
	}
	if got := string(m[1]); got != "v3.1.0" {
		t.Errorf("wails is now %s: re-check the runtime object and method numbers in runtimeguard.go against it, then update this test", got)
	}
}

// wailsChunkThreshold is the body length (in characters) above which the web view's runtime splits a call
// into chunks (@wailsio/runtime, runtime.js).
const wailsChunkThreshold = 512 * 1024

// A call carrying the largest accepted import text, JSON-escaped and sent together with the per-item
// decisions, must stay below the chunking threshold: the guard refuses chunked bodies.
func TestRuntimeGuardImportCallStaysBelowChunkThreshold(t *testing.T) {
	type profile struct {
		Name      string `json:"name"`
		Portal    string `json:"portal"`
		AccountID string `json:"accountId"`
		RoleName  string `json:"roleName"`
		Region    string `json:"region"`
		Output    string `json:"output"`
	}
	var doc struct {
		Format   string    `json:"format"`
		Version  int       `json:"version"`
		Profiles []profile `json:"profiles"`
	}
	doc.Format, doc.Version = "aws-sso-hub.accounts", 1
	req := api.ImportRequest{Include: map[string]bool{}, Choices: map[string]string{}, Renames: map[string]string{}, ImportExtras: true}
	var text string
	for i := 0; ; i++ {
		name := fmt.Sprintf("acme-admin-%04d", i)
		doc.Profiles = append(doc.Profiles, profile{name, "acme", "111122223333", "AdministratorAccess", "us-east-1", "json"})
		b, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > transfer.MaxFileBytes {
			break
		}
		text = string(b)
	}
	// The dialog sends only the items the user changed; 200 of them is generous.
	for _, p := range doc.Profiles[:200] {
		req.Include["profile:"+p.Name] = true
		req.Choices["profile:"+p.Name] = "rename"
		req.Renames["profile:"+p.Name] = p.Name + "-2"
	}
	if len(text) < transfer.MaxFileBytes-1024 {
		t.Fatalf("the sample is far below the limit: %d bytes", len(text))
	}
	req.JSON = text
	body, err := json.Marshal(map[string]any{"object": 0, "method": 0, "args": map[string]any{"methodID": 123, "args": []any{req, "base"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) >= wailsChunkThreshold {
		t.Fatalf("an import call of %d bytes would be chunked (limit %d); lower transfer.MaxFileBytes", len(body), wailsChunkThreshold)
	}
	code, reached, _ := serveThroughGuard(t, "POST", "/wails/runtime", string(body), nil)
	if code != http.StatusOK || !reached {
		t.Errorf("code=%d reached=%v", code, reached)
	}
	t.Logf("import text %d bytes -> call body %d bytes", len(text), len(body))
}

// Bodies the web view would chunk (above 512 KiB) are refused whatever object they name.
func TestRuntimeGuardRefusesBodyAboveChunkThreshold(t *testing.T) {
	big := `{"object":0,"method":0,"args":{"methodID":123,"args":["` + strings.Repeat("x", wailsChunkThreshold) + `"]}}`
	hdr := map[string]string{"x-wails-chunk-id": "abc", "x-wails-chunk-index": "0", "x-wails-chunk-total": "2"}
	code, reached, _ := serveThroughGuard(t, "POST", "/wails/runtime", big[:wailsChunkThreshold/2], hdr)
	if code != http.StatusForbidden || reached {
		t.Errorf("chunk: code=%d reached=%v", code, reached)
	}
}
