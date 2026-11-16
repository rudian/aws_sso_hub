package update

import (
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Platform-independent half of the Windows update gate (plan 4.14 item 4 for Windows). It reads the fixed file
// version out of an exe's VS_VERSION_INFO resource and compares it with the manifest version. It has no build tag,
// so its table tests run on every platform; gate_windows.go calls it.

// winFileVersion maps a manifest version to the four-part Windows file version the release script writes into
// build/windows/info.json (build/release/lib.sh win_file_version): 1.0.0 is 1.0.0.0 and 1.0.0-rc.N is 1.0.0.N.
// Build metadata ("+x") does not count. Any other pre-release form has no mapping and is refused: the release
// script cannot produce it either.
func winFileVersion(manifestVersion string) ([4]uint16, error) {
	var out [4]uint16
	v := strings.TrimPrefix(manifestVersion, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	rc := "0"
	if i := strings.Index(v, "-"); i >= 0 {
		pre := v[i+1:]
		v = v[:i]
		n, ok := strings.CutPrefix(pre, "rc.")
		if !ok || !plainNumber(n) || n == "0" {
			return out, fmt.Errorf("manifest version %q has no Windows file version (only X.Y.Z and X.Y.Z-rc.N do)", manifestVersion)
		}
		rc = n
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("manifest version %q is not X.Y.Z", manifestVersion)
	}
	parts = append(parts, rc)
	for i, p := range parts {
		if !plainNumber(p) {
			return out, fmt.Errorf("manifest version %q is not X.Y.Z", manifestVersion)
		}
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return out, fmt.Errorf("manifest version %q does not fit a Windows file version", manifestVersion)
		}
		out[i] = uint16(n)
	}
	return out, nil
}

// plainNumber is digits only, with no leading zero (semver).
func plainNumber(s string) bool {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func fmtWinVersion(v [4]uint16) string { return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3]) }

// maxRsrcSize bounds what is read from a downloaded file: an icon-and-manifest resource section is well under 1 MiB.
const maxRsrcSize = 32 << 20

const (
	rtVersion          = 16
	fixedInfoSignature = 0xFEEF04BD
)

// readExeFileVersion returns the fixed file version (dwFileVersionMS/LS) of the exe's VS_VERSION_INFO resource.
// Any malformed structure is an error: the file is untrusted input.
func readExeFileVersion(path string) ([4]uint16, error) {
	var zero [4]uint16
	f, err := pe.Open(path)
	if err != nil {
		return zero, fmt.Errorf("not a Windows executable: %w", err)
	}
	defer f.Close()
	sec := f.Section(".rsrc")
	if sec == nil {
		return zero, fmt.Errorf("the executable has no resource section, so no file version")
	}
	if sec.Size == 0 || sec.Size > maxRsrcSize {
		return zero, fmt.Errorf("the executable's resource section has an unreasonable size")
	}
	data, err := io.ReadAll(io.LimitReader(sec.Open(), maxRsrcSize))
	if err != nil {
		return zero, fmt.Errorf("reading the resource section: %w", err)
	}
	return parseVersionResource(data, sec.VirtualAddress)
}

// parseVersionResource walks the resource tree in rsrc (the section's bytes, loaded at rsrcRVA) to the first
// RT_VERSION leaf and reads the fixed file info from it.
func parseVersionResource(rsrc []byte, rsrcRVA uint32) ([4]uint16, error) {
	var zero [4]uint16
	le := binary.LittleEndian
	// dir returns the target offset of the first entry of the directory at off (wantID < 0), or of the ID entry
	// equal to wantID, and whether that target is a subdirectory.
	dir := func(off uint32, wantID int64) (next uint32, isDir bool, err error) {
		if uint64(off)+16 > uint64(len(rsrc)) {
			return 0, false, fmt.Errorf("resource directory out of range")
		}
		named, ids := uint32(le.Uint16(rsrc[off+12:])), uint32(le.Uint16(rsrc[off+14:]))
		n := named + ids
		if uint64(off)+16+uint64(n)*8 > uint64(len(rsrc)) {
			return 0, false, fmt.Errorf("resource directory entries out of range")
		}
		for i := uint32(0); i < n; i++ {
			e := off + 16 + i*8
			name, target := le.Uint32(rsrc[e:]), le.Uint32(rsrc[e+4:])
			if wantID >= 0 && (i < named || name&0x80000000 != 0 || int64(name) != wantID) {
				continue
			}
			return target & 0x7FFFFFFF, target&0x80000000 != 0, nil
		}
		return 0, false, fmt.Errorf("no file version resource")
	}
	// Level 1: type RT_VERSION. Level 2: the first name or id. Level 3: the first language.
	off, isDir, err := dir(0, rtVersion)
	if err != nil || !isDir {
		return zero, fmt.Errorf("no file version resource")
	}
	for level := 0; level < 2; level++ {
		off, isDir, err = dir(off, -1)
		if err != nil {
			return zero, err
		}
		if level == 0 && !isDir {
			return zero, fmt.Errorf("malformed resource tree")
		}
	}
	if isDir {
		return zero, fmt.Errorf("malformed resource tree")
	}
	if uint64(off)+16 > uint64(len(rsrc)) {
		return zero, fmt.Errorf("resource data entry out of range")
	}
	rva, size := le.Uint32(rsrc[off:]), le.Uint32(rsrc[off+4:])
	if rva < rsrcRVA || uint64(rva-rsrcRVA)+uint64(size) > uint64(len(rsrc)) {
		return zero, fmt.Errorf("version resource out of range")
	}
	return parseVersionInfo(rsrc[rva-rsrcRVA : rva-rsrcRVA+size])
}

// parseVersionInfo reads a VS_VERSIONINFO block: wLength, wValueLength, wType, the key "VS_VERSION_INFO", padding to
// 4 bytes, then VS_FIXEDFILEINFO.
func parseVersionInfo(b []byte) ([4]uint16, error) {
	var zero [4]uint16
	const key = "VS_VERSION_INFO"
	hdr := 6 + (len(key)+1)*2
	fixed := (hdr + 3) &^ 3
	if len(b) < fixed+16 {
		return zero, fmt.Errorf("version resource is too short")
	}
	units := make([]uint16, len(key))
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(b[6+i*2:])
	}
	if string(utf16.Decode(units)) != key || binary.LittleEndian.Uint16(b[6+len(key)*2:]) != 0 {
		return zero, fmt.Errorf("version resource has no VS_VERSION_INFO key")
	}
	if binary.LittleEndian.Uint16(b[2:]) < 16 || binary.LittleEndian.Uint32(b[fixed:]) != fixedInfoSignature {
		return zero, fmt.Errorf("version resource has no fixed file info")
	}
	ms, ls := binary.LittleEndian.Uint32(b[fixed+8:]), binary.LittleEndian.Uint32(b[fixed+12:])
	return [4]uint16{uint16(ms >> 16), uint16(ms), uint16(ls >> 16), uint16(ls)}, nil
}

// checkExeVersion is the file-version half of the gate: the exe at path must carry the file version that the
// manifest version maps to. Every failure is a coded error.
func checkExeVersion(path, manifestVersion string) error {
	want, err := winFileVersion(manifestVersion)
	if err != nil {
		return codedf(CodeBundleVersion, "%v", err)
	}
	got, err := readExeFileVersion(path)
	if err != nil {
		return codedf(CodeBundleInvalid, "staged executable: %v", err)
	}
	if got != want {
		return codedf(CodeBundleVersion, "executable file version %s does not equal manifest version %s (expected %s)",
			fmtWinVersion(got), manifestVersion, fmtWinVersion(want))
	}
	return nil
}
