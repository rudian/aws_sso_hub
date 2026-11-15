package update

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildExe writes a tiny but valid PE32+ file whose only section is .rsrc, holding one RT_VERSION resource with the
// given fixed file version. mutate may change the resource section's bytes before they are written. No real binary
// is involved: the test makes its own fixture.
func buildExe(t testing.TB, path string, ver [4]uint16, mutate func(rsrc []byte) []byte) {
	t.Helper()
	le := binary.LittleEndian
	const rva = 0x1000
	// resource tree: type 16 -> id 1 -> language 0x409 -> data entry -> VS_VERSIONINFO
	r := make([]byte, 88)
	put := func(off int, vals ...uint32) {
		for i, v := range vals {
			le.PutUint32(r[off+i*4:], v)
		}
	}
	le.PutUint16(r[14:], 1)
	put(16, 16, 0x80000000|24)
	le.PutUint16(r[24+14:], 1)
	put(24+16, 1, 0x80000000|48)
	le.PutUint16(r[48+14:], 1)
	put(48+16, 0x409, 72)
	blob := make([]byte, 92)
	le.PutUint16(blob[0:], 92)
	le.PutUint16(blob[2:], 52)
	for i, c := range "VS_VERSION_INFO" {
		le.PutUint16(blob[6+i*2:], uint16(c))
	}
	le.PutUint32(blob[40:], fixedInfoSignature)
	le.PutUint32(blob[44:], 0x10000)
	le.PutUint32(blob[48:], uint32(ver[0])<<16|uint32(ver[1]))
	le.PutUint32(blob[52:], uint32(ver[2])<<16|uint32(ver[3]))
	put(72, rva+88, uint32(len(blob)), 0, 0)
	rs := append(r, blob...)
	if mutate != nil {
		rs = mutate(rs)
	}
	raw := (len(rs) + 0x1ff) &^ 0x1ff

	var b bytes.Buffer
	dos := make([]byte, 0x40)
	copy(dos, "MZ")
	le.PutUint32(dos[0x3c:], 0x40)
	b.Write(dos)
	b.WriteString("PE\x00\x00")
	_ = binary.Write(&b, le, pe.FileHeader{Machine: pe.IMAGE_FILE_MACHINE_AMD64, NumberOfSections: 1, SizeOfOptionalHeader: 240, Characteristics: 0x22})
	oh := pe.OptionalHeader64{Magic: 0x20b, ImageBase: 0x400000, SectionAlignment: 0x1000, FileAlignment: 0x200,
		SizeOfImage: 0x2000, SizeOfHeaders: 0x200, Subsystem: 2, NumberOfRvaAndSizes: 16}
	oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE] = pe.DataDirectory{VirtualAddress: rva, Size: uint32(len(rs))}
	_ = binary.Write(&b, le, oh)
	sh := pe.SectionHeader32{VirtualSize: uint32(len(rs)), VirtualAddress: rva, SizeOfRawData: uint32(raw), PointerToRawData: 0x200, Characteristics: 0x40000040}
	copy(sh.Name[:], ".rsrc")
	_ = binary.Write(&b, le, sh)
	for b.Len() < 0x200 {
		b.WriteByte(0)
	}
	b.Write(rs)
	for b.Len() < 0x200+raw {
		b.WriteByte(0)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestWinFileVersionTable(t *testing.T) {
	cases := []struct {
		in      string
		want    [4]uint16
		wantErr bool
	}{
		{"1.0.0", [4]uint16{1, 0, 0, 0}, false},
		{"1.0.0-rc.1", [4]uint16{1, 0, 0, 1}, false},
		{"1.0.0-rc.2", [4]uint16{1, 0, 0, 2}, false},
		{"v1.2.3", [4]uint16{1, 2, 3, 0}, false},
		{"1.2.3+build5", [4]uint16{1, 2, 3, 0}, false},
		{"1.2.3-rc.12+x", [4]uint16{1, 2, 3, 12}, false},
		{"65535.0.0", [4]uint16{65535, 0, 0, 0}, false},
		{"65536.0.0", [4]uint16{}, true},
		{"1.0.0-rc.0", [4]uint16{}, true},
		{"1.0.0-rc.01", [4]uint16{}, true},
		{"1.0.0-rc.", [4]uint16{}, true},
		{"1.0.0-beta.1", [4]uint16{}, true},
		{"1.0.0-rc.1.2", [4]uint16{}, true},
		{"1.0", [4]uint16{}, true},
		{"1.0.0.0", [4]uint16{}, true},
		{"1.0.x", [4]uint16{}, true},
		{"01.0.0", [4]uint16{}, true},
		{"-1.0.0", [4]uint16{}, true},
		{"", [4]uint16{}, true},
	}
	for _, c := range cases {
		got, err := winFileVersion(c.in)
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("winFileVersion(%q) = %v, %v; want %v (err %v)", c.in, got, err, c.want, c.wantErr)
		}
	}
}

func TestCheckExeVersionTable(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		exe      [4]uint16
		manifest string
		code     string // "" means accepted
	}{
		{"rc equal", [4]uint16{1, 0, 0, 2}, "1.0.0-rc.2", ""},
		{"final equal", [4]uint16{1, 0, 0, 0}, "1.0.0", ""},
		{"rc.1 exe under rc.2 manifest", [4]uint16{1, 0, 0, 1}, "1.0.0-rc.2", CodeBundleVersion},
		{"old exe under newer manifest", [4]uint16{0, 0, 1, 0}, "1.0.0", CodeBundleVersion},
		{"final exe under rc manifest", [4]uint16{1, 0, 0, 0}, "1.0.0-rc.2", CodeBundleVersion},
		{"rc exe under final manifest", [4]uint16{1, 0, 0, 2}, "1.0.0", CodeBundleVersion},
		{"patch differs", [4]uint16{1, 0, 1, 0}, "1.0.0", CodeBundleVersion},
		{"unmappable manifest version", [4]uint16{1, 0, 0, 0}, "1.0.0-beta.1", CodeBundleVersion},
	}
	for i, c := range cases {
		p := filepath.Join(dir, "x"+string(rune('a'+i))+".exe")
		buildExe(t, p, c.exe, nil)
		err := checkExeVersion(p, c.manifest)
		if got := codeOf(err); got != c.code || (err == nil) != (c.code == "") {
			t.Errorf("%s: err %v code %q, want code %q", c.name, err, got, c.code)
		}
	}
}

func TestReadExeFileVersionRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "codepass.exe")
	want := [4]uint16{1, 22, 333, 4444}
	buildExe(t, p, want, nil)
	got, err := readExeFileVersion(p)
	if err != nil || got != want {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestReadExeFileVersionMalformed(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.exe")
	buildExe(t, good, [4]uint16{1, 0, 0, 2}, nil)
	goodBytes, _ := os.ReadFile(good)

	muts := map[string]func([]byte) []byte{
		"truncated resource":     func(r []byte) []byte { return r[:40] },
		"empty resource":         func(r []byte) []byte { return r[:0] },
		"huge entry count":       func(r []byte) []byte { binary.LittleEndian.PutUint16(r[14:], 0xFFFF); return r },
		"wrong type id":          func(r []byte) []byte { binary.LittleEndian.PutUint32(r[16:], 24); return r },
		"data rva outside":       func(r []byte) []byte { binary.LittleEndian.PutUint32(r[72:], 0x9000); return r },
		"data size outside":      func(r []byte) []byte { binary.LittleEndian.PutUint32(r[76:], 0xFFFFFFF0); return r },
		"data rva below":         func(r []byte) []byte { binary.LittleEndian.PutUint32(r[72:], 4); return r },
		"bad key":                func(r []byte) []byte { r[88+6] = 'X'; return r },
		"bad signature":          func(r []byte) []byte { r[88+40] = 0; return r },
		"leaf is a directory":    func(r []byte) []byte { binary.LittleEndian.PutUint32(r[68:], 0x80000000|72); return r },
		"loop in tree":           func(r []byte) []byte { binary.LittleEndian.PutUint32(r[20:], 0x80000000); return r },
		"version blob too short": func(r []byte) []byte { binary.LittleEndian.PutUint32(r[76:], 20); return r },
	}
	for name, m := range muts {
		p := filepath.Join(dir, "m.exe")
		buildExe(t, p, [4]uint16{1, 0, 0, 2}, m)
		if v, err := readExeFileVersion(p); err == nil {
			t.Errorf("%s: accepted, version %v", name, v)
		}
	}

	files := map[string][]byte{
		"empty file":      {},
		"text":            []byte("hello, this is not an exe"),
		"MZ only":         []byte("MZ"),
		"truncated head":  goodBytes[:0x90],
		"no section data": goodBytes[:0x200],
	}
	for name, b := range files {
		p := filepath.Join(dir, "f.exe")
		_ = os.WriteFile(p, b, 0o644)
		if v, err := readExeFileVersion(p); err == nil {
			t.Errorf("%s: accepted, version %v", name, v)
		}
	}
	if _, err := readExeFileVersion(filepath.Join(dir, "missing.exe")); err == nil {
		t.Error("missing file accepted")
	}
	// an exe with no .rsrc section at all
	noRsrc := bytes.Replace(goodBytes, []byte(".rsrc"), []byte(".text"), 1)
	p := filepath.Join(dir, "nor.exe")
	_ = os.WriteFile(p, noRsrc, 0o644)
	if _, err := readExeFileVersion(p); err == nil || !strings.Contains(err.Error(), "no resource section") {
		t.Errorf("no .rsrc: %v", err)
	}
	if err := checkExeVersion(p, "1.0.0-rc.2"); codeOf(err) != CodeBundleInvalid {
		t.Errorf("unreadable exe code %q", codeOf(err))
	}
}

// A refusal must leave no staging folder and must not touch the installed exe: the gate reads the staged file only,
// and the controller removes the Wails staging folder on refusal (removeStaging).
func TestRefusalLeavesNoStagingFolder(t *testing.T) {
	install := t.TempDir()
	installed := filepath.Join(install, "codepass.exe")
	buildExe(t, installed, [4]uint16{1, 0, 0, 1}, nil)
	before, _ := os.ReadFile(installed)

	stage := filepath.Join(install, "wails-update-42")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(stage, "codepass.exe")
	buildExe(t, staged, [4]uint16{1, 0, 0, 7}, nil) // the manifest says rc.2

	err := checkExeVersion(staged, "1.0.0-rc.2")
	if codeOf(err) != CodeBundleVersion {
		t.Fatalf("not refused: %v", err)
	}
	removeStaging(staged, install)
	if _, err := os.Stat(stage); !os.IsNotExist(err) {
		t.Error("staging folder left behind")
	}
	after, err := os.ReadFile(installed)
	if err != nil || !bytes.Equal(before, after) {
		t.Error("installed exe changed")
	}
	entries, _ := os.ReadDir(install)
	if len(entries) != 1 {
		t.Errorf("install folder holds %d entries, want only the exe", len(entries))
	}
}
