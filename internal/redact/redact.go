// Package redact masks secrets (AWS key IDs, secret keys, session tokens, OAuth tokens, URL passwords)
// in free text. Every string that leaves Go (errors, logs, diffs, events) goes through it.
package redact

import (
	"regexp"
	"strings"
)

// Mask replaces secret values.
const Mask = "••••••••"

const keyIDPrefixes = `(?:AKIA|ASIA|AIDA|AROA|APKA)`

var (
	// username=ASIA...%SESSIONTOKEN as printed by the credential helper: keep only the masked key id.
	reHelperUser = regexp.MustCompile(`(username=)(` + keyIDPrefixes + `[A-Z0-9]{12,128})%[^\s]*`)
	reKeyID      = regexp.MustCompile(`\b` + keyIDPrefixes + `[A-Z0-9]{12,128}\b`)
	reSecretKV   = regexp.MustCompile(`(?i)\b(aws_secret_access_key|aws_session_token|aws_security_token|accessToken|refreshToken|clientSecret|SecretAccessKey|SessionToken|password)(["']?\s*[:=]\s*)(?:"([^"]*)"|'([^']*)'|([^\s"',}]+))`)
	reUserinfo   = regexp.MustCompile(`(?i)([a-z][a-z0-9+.\-]*://[^/\s:@]*):([^/\s@]+)@`)
	// reBareUser matches a URL whose userinfo is a single token (no password part).
	reBareUser = regexp.MustCompile(`(?i)([a-z][a-z0-9+.\-]*://)([^/\s:@]+)@`)
	// reExtraHeader matches a git extraheader setting; its value is usually a complete Authorization header.
	reExtraHeader = regexp.MustCompile(`(?i)(\bextraheader\b\s*[=:]\s*)([^\r\n]*[^\s])`)
	// reAuthHeader matches an Authorization header with a credential.
	reAuthHeader = regexp.MustCompile(`(?i)(\bauthorization\s*:\s*(?:bearer|basic|token)\s+)([^\s"']+)`)
	// reCodeCommitUser is the user name of a CodeCommit HTTPS Git credential: <iam user>-at-<account id>.
	reCodeCommitUser = regexp.MustCompile(`-at-[0-9]{12}$`)
)

var tokenUserPrefixes = []string{"ghp_", "gho_", "ghs_", "github_pat_", "glpat-", "xox"}

// looksLikeTokenUser reports whether a bare URL user name is probably an access token rather than a name.
func looksLikeTokenUser(u string) bool {
	if reCodeCommitUser.MatchString(u) {
		return false
	}
	if len(u) >= 20 {
		return true
	}
	lu := strings.ToLower(u)
	for _, p := range tokenUserPrefixes {
		if strings.HasPrefix(lu, p) {
			return true
		}
	}
	return false
}

// String masks key IDs, secret values and URL passwords in s.
func String(s string) string {
	if s == "" {
		return s
	}
	s = reHelperUser.ReplaceAllStringFunc(s, func(m string) string {
		sub := reHelperUser.FindStringSubmatch(m)
		return sub[1] + MaskKeyID(sub[2])
	})
	s = reKeyID.ReplaceAllStringFunc(s, MaskKeyID)
	s = reSecretKV.ReplaceAllStringFunc(s, func(m string) string {
		sub := reSecretKV.FindStringSubmatch(m)
		head := sub[1] + sub[2]
		switch {
		case sub[3] != "":
			return head + `"` + Mask + `"`
		case sub[4] != "":
			return head + `'` + Mask + `'`
		case sub[5] != "":
			return head + Mask
		}
		return m // empty quoted value: nothing to hide
	})
	s = reExtraHeader.ReplaceAllString(s, "${1}"+Mask)
	s = reAuthHeader.ReplaceAllString(s, "${1}"+Mask)
	s = reUserinfo.ReplaceAllString(s, "${1}:"+Mask+"@")
	return reBareUser.ReplaceAllStringFunc(s, func(m string) string {
		sub := reBareUser.FindStringSubmatch(m)
		if looksLikeTokenUser(sub[2]) {
			return sub[1] + Mask + "@"
		}
		return m
	})
}

// MaskKeyID returns the first 4 + 12 bullets + last 4 characters; "" when id has fewer than 8 characters.
func MaskKeyID(id string) string {
	if len(id) < 8 {
		return ""
	}
	return id[:4] + strings.Repeat("•", 12) + id[len(id)-4:]
}

// URL masks the password in URL userinfo: https://user:pass@h becomes https://user:••••••••@h.
func URL(raw string) string {
	return reUserinfo.ReplaceAllString(raw, "${1}:"+Mask+"@")
}

// Tail returns the last `lines` lines of s, each passed through String. Empty trailing lines are dropped.
func Tail(s string, lines int) string {
	if lines <= 0 || s == "" {
		return ""
	}
	parts := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) > lines {
		parts = parts[len(parts)-lines:]
	}
	for i, p := range parts {
		parts[i] = String(strings.TrimRight(p, "\r"))
	}
	return strings.Join(parts, "\n")
}
