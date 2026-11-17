package terminal

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
)

// maxArchives is how many ended sessions' logs the manager keeps loaded from their files; the least recently
// read one is dropped beyond it (a file is read again on demand).
const maxArchives = 4

// The file starts with a header: magic "CPS1", the absolute offset of the first data byte (int64, big endian)
// and the length of the mode prologue (uint32), then the prologue, then the data. The offset keeps offsets
// stable: after the session is released or the app restarted, a page's older offset still gets a gap, never
// other bytes.
const (
	sbMagic     = "CPS1"
	sbHeaderLen = 4 + 8 + 4
)

func encodeScrollback(from int64, prologue, data []byte) []byte {
	b := make([]byte, 0, sbHeaderLen+len(prologue)+len(data))
	b = append(b, sbMagic...)
	b = binary.BigEndian.AppendUint64(b, uint64(from))
	b = binary.BigEndian.AppendUint32(b, uint32(len(prologue)))
	b = append(b, prologue...)
	return append(b, data...)
}

// decodeScrollback splits a file. A file without the header is taken as plain data from offset 0.
func decodeScrollback(b []byte) (from int64, prologue, data []byte) {
	if len(b) < sbHeaderLen || string(b[:4]) != sbMagic {
		return 0, nil, b
	}
	from = int64(binary.BigEndian.Uint64(b[4:12]))
	pl := int(binary.BigEndian.Uint32(b[12:16]))
	if from < 0 || pl < 0 || sbHeaderLen+pl > len(b) {
		return 0, nil, nil
	}
	return from, b[sbHeaderLen : sbHeaderLen+pl], b[sbHeaderLen+pl:]
}

// archiveTail is the tail offset an ended session's file reaches (0 when there is none), read from the header
// and the file size without loading the data.
func archiveTail(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	var h [sbHeaderLen]byte
	if _, err := io.ReadFull(f, h[:]); err != nil || string(h[:4]) != sbMagic {
		return 0
	}
	st, err := f.Stat()
	if err != nil {
		return 0
	}
	from := int64(binary.BigEndian.Uint64(h[4:12]))
	pl := int64(binary.BigEndian.Uint32(h[12:16]))
	n := st.Size() - sbHeaderLen - pl
	if from < 0 || n < 0 {
		return 0
	}
	return from + n
}

// Scrollback files (plan 4.7, A2-02): when a session ends, its last output (capped) is written to
// <data>/sessions/<task-id>/scrollback with mode 0600, so an ended task can still be read after a restart.
//
// The file holds terminal output, which can contain anything the task printed. It lives in the task's own
// folder under the data folder, which backups (the AWS and Git files), exports (profiles) and the app log
// (which never receives terminal bytes) do not read. This package never logs: terminal.go and the files here
// import no logger (TestNoLogging).

// ScrollbackPath is the file for a session ID. It is "" for an ID that is not valid.
func ScrollbackPath(dataDir, id string) string {
	if dataDir == "" || !ValidID(id) {
		return ""
	}
	return filepath.Join(dataDir, "sessions", id, "scrollback")
}

func (m *Manager) scrollbackOn() bool {
	return m.cfg.DataDir != "" && (m.cfg.ScrollbackEnabled == nil || m.cfg.ScrollbackEnabled())
}

// saveScrollback writes the file, or removes an old one when the setting is off. Errors are not reported: a
// missing scrollback is not worth interrupting anything for.
func (m *Manager) saveScrollback(s *Session) {
	// FU-A2-02b: whatever happens to the file, a copy of the older one loaded earlier is stale from now on
	defer func() {
		m.mu.Lock()
		m.dropArchiveLocked(s.id)
		m.mu.Unlock()
	}()
	path := ScrollbackPath(m.cfg.DataDir, s.id)
	if path == "" {
		return
	}
	if !m.scrollbackOn() {
		_ = os.Remove(path)
		return
	}
	data, prologue, from := s.log.Snapshot(m.cfg.ScrollbackCap)
	if len(data) == 0 {
		_ = os.Remove(path)
		return
	}
	_ = writeFile0600(path, encodeScrollback(from, prologue, data))
}

func writeFile0600(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".scrollback-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp)
		if werr != nil {
			return werr
		}
		return cerr
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// archive loads (once, while it stays among the most recently read) the scrollback file of an ended session
// the manager no longer holds.
func (m *Manager) archive(id string) *Log {
	m.mu.Lock()
	if l := m.archives[id]; l != nil {
		m.touchArchiveLocked(id)
		m.mu.Unlock()
		return l
	}
	gen := m.archGen[id]
	m.mu.Unlock()
	path := ScrollbackPath(m.cfg.DataDir, id)
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if h := m.afterArchiveRead; h != nil {
		h(id) // test seam: a Release, a new run or a save happens between the read and the cache
	}
	if err != nil || len(b) == 0 {
		return nil
	}
	from, prologue, data := decodeScrollback(b)
	if len(data) == 0 {
		return nil
	}
	modes := defaultModes()
	if len(prologue) > 0 {
		p := newParser()
		p.feed(bytes.Clone(prologue), 0)
		modes = p.cur
	}
	l := newLogAt(m.cfg.LogCap, chunkSize, from, modes)
	l.Append(data)
	m.mu.Lock()
	defer m.mu.Unlock()
	if old := m.archives[id]; old != nil {
		return old
	}
	if m.sessions[id] != nil || m.starting[id] || m.archGen[id] != gen {
		// FU-A2-02b: a run of this ID is starting or exists; the file is the previous run's (or about to be
		// replaced), so answer from it once but never keep it
		return l
	}
	m.archives[id] = l
	m.archOrder = append(m.archOrder, id)
	for len(m.archOrder) > maxArchives {
		delete(m.archives, m.archOrder[0])
		m.archOrder = m.archOrder[1:]
	}
	return l
}

// touchArchiveLocked marks id as the most recently read archive.
func (m *Manager) touchArchiveLocked(id string) {
	for i, x := range m.archOrder {
		if x == id {
			m.archOrder = append(append(m.archOrder[:i:i], m.archOrder[i+1:]...), id)
			return
		}
	}
}

// dropArchiveLocked forgets a loaded archive.
func (m *Manager) dropArchiveLocked(id string) {
	m.archGen[id]++
	delete(m.archives, id)
	for i, x := range m.archOrder {
		if x == id {
			m.archOrder = append(m.archOrder[:i:i], m.archOrder[i+1:]...)
			return
		}
	}
}
