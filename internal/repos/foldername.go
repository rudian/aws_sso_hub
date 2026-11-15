package repos

import (
	"strings"
	"unicode/utf8"

	"github.com/rudian/codepass/internal/api"
)

// Reasons ValidateFolderName reports in the "reason" parameter of its error.
const (
	FolderEmpty     = "empty"
	FolderDots      = "dots"      // "." or ".."
	FolderSeparator = "separator" // "/" or "\"
	FolderBadChar   = "bad-char"  // a control character or one of < > : " | ? *
	FolderReserved  = "reserved"  // a Windows device name (CON, PRN, AUX, NUL, COM1-9, LPT1-9), also with an extension
	FolderTrailing  = "trailing"  // ends in a dot or a space
	FolderTooLong   = "too-long"
)

// reservedDevice lists the Windows device names. A name is reserved when what comes before its first dot is one
// of them in any case, so "con", "NUL.txt" and "com1.tar.gz" are refused. The superscript digits count too: the
// Windows file API treats COM and LPT followed by one of them as the same devices.
var reservedDevice = func() map[string]bool {
	m := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true, "CONIN$": true, "CONOUT$": true}
	for _, d := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "¹", "²", "³"} {
		m["COM"+d], m["LPT"+d] = true, true
	}
	return m
}()

// ValidateFolderName checks a local folder name for a clone. It must hold on every platform, because a
// repository cloned on a Mac may be copied to a Windows machine: no path separator, not "." or "..", no reserved
// Windows device name, no trailing dot or space, no character Windows forbids in a file name, at most 255 bytes.
// It returns nil, or an E_INVALID_INPUT error with field=folderName and a reason (the Folder* constants).
func ValidateFolderName(name string) *api.AppError {
	bad := func(reason string) *api.AppError {
		return api.NewError(api.ErrInvalidInput, "invalid folder name").WithParam("field", "folderName").WithParam("reason", reason)
	}
	switch {
	case name == "":
		return bad(FolderEmpty)
	case name == "." || name == "..":
		return bad(FolderDots)
	case strings.ContainsAny(name, `/\`):
		return bad(FolderSeparator)
	case !utf8.ValidString(name):
		return bad(FolderBadChar)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return bad(FolderBadChar)
		}
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return bad(FolderTrailing)
	}
	stem := name
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if reservedDevice[strings.ToUpper(strings.TrimRight(stem, " "))] {
		return bad(FolderReserved)
	}
	if len(name) > 255 {
		return bad(FolderTooLong)
	}
	return nil
}
