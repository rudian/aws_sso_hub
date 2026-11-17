package taskstate

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// openCodePlugin is the plain-JS plugin of the spike (S0-03), reduced: it forwards only the event type, the
// session ID and the error type (plus its own clock and counter, which order the events, since each event is a
// separate process), as one JSON object on the stdin of `<exe> report opencode`. Prompts and
// messages (session.inbox.enqueued, error text) are never read. %s is the JSON-quoted path of the binary.
const openCodePlugin = `import { spawn } from "node:child_process"
const exe = %s
const wanted = new Set(%s)
wanted.delete("plugin.loaded")
let seq = 0
const send = (type, sessionID, errorType) => {
  const atMs = Date.now(), n = seq++ %% 1000000
  try {
    const c = spawn(exe, ["report", "opencode"], { stdio: ["pipe", "ignore", "ignore"], windowsHide: true })
    c.on("error", () => {})
    c.stdin.on("error", () => {})
    c.stdin.end(JSON.stringify({ type, atMs, seq: n, data: { sessionID, error: { type: errorType } } }))
    c.unref()
  } catch (_) {}
}
export default {
  id: "codepass-signals",
  async setup(ctx) {
    send("plugin.loaded")
    const ac = new AbortController()
    void (async () => {
      for await (const e of ctx.event.subscribe({ signal: ac.signal })) {
        if (!wanted.has(e.type)) continue
        send(e.type, e.data?.sessionID ?? e.durable?.aggregateID, e.data?.error?.type)
      }
    })()
    return () => ac.abort()
  },
}
`

// WriteOpenCodePlugin writes the plugin folder (index.js) under taskDir and returns it. The launcher passes
// OpenCodeConfigEnv(dir) in the child environment together with `--standalone`; no config file is written.
func WriteOpenCodePlugin(taskDir, exe string) (string, error) {
	dir := filepath.Join(taskDir, "opencode-plugin")
	q, err := json.Marshal(exe)
	if err != nil {
		return "", err
	}
	evs, _ := json.Marshal(OpenCodeEvents)
	src := []byte(fmt.Sprintf(openCodePlugin, string(q), string(evs)))
	if err := writePrivate(filepath.Join(dir, "index.js"), src); err != nil {
		return "", err
	}
	return dir, nil
}

// OpenCodeConfigEnv is the environment entry that loads the plugin for this launch only.
func OpenCodeConfigEnv(pluginDir string) string {
	b, _ := json.Marshal(map[string][]string{"plugins": {pluginDir}})
	return "OPENCODE_CONFIG_CONTENT=" + string(b)
}
