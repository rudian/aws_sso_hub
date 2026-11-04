package app

import "sync"

// recordedEvent is one event passed through fakeRuntime.Emit.
type recordedEvent struct {
	Name string
	Data any
}

// fakeRuntime records events and URLs. When browser is set, OpenURL calls it (the test plays the user's browser).
type fakeRuntime struct {
	mu       sync.Mutex
	events   []recordedEvent
	urls     []string
	revealed []string
	opened   []string
	folder   string
	saveTo   string // path the fake save dialog returns ("" = the user cancels)
	openFrom string // path the fake open dialog returns ("" = the user cancels)
	dialogs  []string
	shown    int
	browser  func(u string)
}

func (f *fakeRuntime) Emit(name string, data any) {
	f.mu.Lock()
	f.events = append(f.events, recordedEvent{name, data})
	f.mu.Unlock()
}

func (f *fakeRuntime) OpenURL(raw string) error {
	// Tests run their fake servers on loopback http, which sandbox mode allows.
	if err := checkOpenable(raw, true); err != nil {
		return err
	}
	f.mu.Lock()
	f.urls = append(f.urls, raw)
	b := f.browser
	f.mu.Unlock()
	if b != nil {
		go b(raw)
	}
	return nil
}

func (f *fakeRuntime) RevealPath(p string) error {
	f.mu.Lock()
	f.revealed = append(f.revealed, p)
	f.mu.Unlock()
	return nil
}
func (f *fakeRuntime) OpenFile(p string) error {
	f.mu.Lock()
	f.opened = append(f.opened, p)
	f.mu.Unlock()
	return nil
}
func (f *fakeRuntime) ChooseFolder(string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.folder, nil
}
func (f *fakeRuntime) ChooseSaveFile(title, defaultName string, filter FileFilter) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dialogs = append(f.dialogs, "save:"+defaultName+":"+filter.Pattern)
	return f.saveTo, nil
}
func (f *fakeRuntime) ChooseOpenFile(title string, filter FileFilter) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dialogs = append(f.dialogs, "open:"+filter.Pattern)
	return f.openFrom, nil
}
func (f *fakeRuntime) ShowWindow() { f.mu.Lock(); f.shown++; f.mu.Unlock() }

func (f *fakeRuntime) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.events))
	for i, e := range f.events {
		out[i] = e.Name
	}
	return out
}

func (f *fakeRuntime) count(name string) int {
	n := 0
	for _, e := range f.names() {
		if e == name {
			n++
		}
	}
	return n
}

func (f *fakeRuntime) snapshot() []recordedEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]recordedEvent(nil), f.events...)
}
