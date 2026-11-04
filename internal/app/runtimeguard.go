package app

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// Runtime object and method numbers of Wails v3.1.0 (pkg/application/messageprocessor*.go). They are part of
// the wire format of /wails/runtime and may change with the library; runtimeguard_test.go fails when the
// version in go.mod changes so that they are checked again on every upgrade.
const (
	runtimeObjectClipboard = 1
	runtimeObjectBrowser   = 9
	clipboardMethodText    = 1

	// maxRuntimeBody bounds what the guard buffers. The app's own calls are far smaller; larger or chunked
	// bodies are refused rather than inspected piecemeal.
	maxRuntimeBody = 4 << 20

	chunkHeader = "x-wails-chunk-id"
)

// RuntimeGuard is asset-server middleware that filters calls to the Wails runtime endpoint before the library
// handles them. Page scripts can reach every runtime object through it, and two of them are not wanted here:
//
//   - Browser (all methods) opens URLs of any scheme the library's small deny-list lets through. Every URL the
//     app opens goes through SystemService.OpenURL or an engine, which accept https only.
//   - Clipboard.Text reads whatever the user copied from other applications.
//
// Chunked bodies cannot be inspected, so they are refused; the app never sends calls that large. Requests
// that are not valid runtime calls get 400; everything else reaches the library unchanged.
//
// Residual risk: bound service methods stay callable by any script that runs in the web view. The primary
// defence there is the Content-Security-Policy of the production page and the absence of HTML-injection sinks
// in the frontend; the postMessage channel carries window and drag messages only.
func RuntimeGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/wails/runtime" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get(chunkHeader) != "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		var body []byte
		if r.Body != nil {
			var err error
			body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, maxRuntimeBody))
			if err != nil {
				http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		object, method, ok := runtimeTarget(body, r)
		if !ok {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if object == runtimeObjectBrowser || (object == runtimeObjectClipboard && method == clipboardMethodText) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// runtimeTarget extracts the object and method the way the library does: from the JSON body, or from the
// query parameters when the body is empty.
func runtimeTarget(body []byte, r *http.Request) (object, method int, ok bool) {
	if len(body) > 0 {
		var req struct {
			Object *int `json:"object"`
			Method *int `json:"method"`
		}
		if json.Unmarshal(body, &req) != nil || req.Object == nil || req.Method == nil {
			return 0, 0, false
		}
		return *req.Object, *req.Method, true
	}
	q := r.URL.Query()
	o, oerr := strconv.Atoi(q.Get("object"))
	m, merr := strconv.Atoi(q.Get("method"))
	if oerr != nil || merr != nil {
		return 0, 0, false
	}
	return o, m, true
}
