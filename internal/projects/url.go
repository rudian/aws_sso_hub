package projects

import "strings"

// StripURLUserinfo removes the user (and password) from a remote URL. A token used as the user name, as in
// https://<token>@github.com/acme/api.git, is a secret too, so the whole userinfo goes rather than only the
// password. Handles scheme URLs and scp-like "user@host:path"; anything else comes back unchanged.
func StripURLUserinfo(raw string) string {
	if i := strings.Index(raw, "://"); i >= 0 {
		rest := raw[i+3:]
		end := strings.IndexAny(rest, "/?#")
		if end < 0 {
			end = len(rest)
		}
		if at := strings.LastIndex(rest[:end], "@"); at >= 0 {
			return raw[:i+3] + rest[at+1:]
		}
		return raw
	}
	// scp-like: no scheme, "user@host:path" with the "@" before the first ":" and no "/" before it.
	colon := strings.Index(raw, ":")
	if colon < 0 {
		return raw
	}
	head := raw[:colon]
	if at := strings.LastIndex(head, "@"); at >= 0 && !strings.Contains(head, "/") {
		return raw[at+1:]
	}
	return raw
}
