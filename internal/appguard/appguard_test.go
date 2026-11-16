package appguard

import (
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeLister struct {
	procs []Proc
	err   error
}

func (f fakeLister) List() ([]Proc, error) { return f.procs, f.err }

// exitingLister lists procs except those the terminator was asked to quit (the old app exits when asked).
type exitingLister struct {
	procs []Proc
	term  *fakeTerm
}

func (l exitingLister) List() ([]Proc, error) {
	var out []Proc
	for _, p := range l.procs {
		quit := false
		for _, q := range l.term.quit {
			quit = quit || q == p.PID
		}
		if !quit {
			out = append(out, p)
		}
	}
	return out, nil
}

type fakeTerm struct {
	quitErr   error
	quit      []int
	aliveFor  int // Alive reports true this many times, then false; -1 = forever
	aliveSeen int
}

func (f *fakeTerm) Quit(pid int) error { f.quit = append(f.quit, pid); return f.quitErr }
func (f *fakeTerm) Alive(int) bool {
	if f.aliveFor < 0 {
		return true
	}
	f.aliveSeen++
	return f.aliveSeen <= f.aliveFor
}

type fakeFiles struct {
	exist map[string]bool
	moves [][2]string
	trash string
	err   error
	// Recycle Bin (Windows) fakes
	recycle  bool
	recycled []string
	recErr   error
	// beforeMove runs at the start of every Rename and Recycle (tests assert what happened before)
	beforeMove func()
}

func (f *fakeFiles) Exists(p string) bool { return f.exist[p] }
func (f *fakeFiles) Rename(a, b string) error {
	if f.beforeMove != nil {
		f.beforeMove()
	}
	if f.err != nil {
		return f.err
	}
	f.moves = append(f.moves, [2]string{a, b})
	f.exist[b] = true
	delete(f.exist, a)
	return nil
}
func (f *fakeFiles) TrashDir() string { return f.trash }
func (f *fakeFiles) CanRecycle() bool { return f.recycle }
func (f *fakeFiles) Recycle(p string) error {
	if f.beforeMove != nil {
		f.beforeMove()
	}
	if f.recErr != nil {
		return f.recErr
	}
	f.recycled = append(f.recycled, p)
	delete(f.exist, p)
	return nil
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time        { return c.t }
func (c *fakeClock) Sleep(d time.Duration) { c.t = c.t.Add(d) }

type fakePrompter struct {
	quit      bool // answer to AskQuit
	trash     bool // answer to AskTrash
	askedQuit int
	askedTrsh int
	bundles   []string
	lang      string
	told      []string
}

func (p *fakePrompter) AskQuit(_ []Instance, lang string) bool {
	p.askedQuit++
	p.lang = lang
	return p.quit
}
func (p *fakePrompter) AskTrash(b []string, _ string) bool {
	p.askedTrsh++
	p.bundles = b
	return p.trash
}
func (p *fakePrompter) Tell(m string) { p.told = append(p.told, m) }

type choice int

const (
	cCancel choice = iota
	cQuit
	cTrash
)

const oldExe = "/Applications/AWS SSO Hub.app/Contents/MacOS/AWS SSO Hub"
const oldBundle = "/Applications/AWS SSO Hub.app"

func setup(procs []Proc, answer choice) (Deps, *fakeTerm, *fakeFiles, *fakePrompter) {
	term := &fakeTerm{aliveFor: 2}
	files := &fakeFiles{exist: map[string]bool{oldBundle: true}, trash: "/Users/alex/.Trash"}
	pr := &fakePrompter{quit: answer != cCancel, trash: answer == cTrash}
	return Deps{Lister: exitingLister{procs: procs, term: term}, Term: term, Files: files, Clock: &fakeClock{t: time.Unix(0, 0)},
		Prompter: pr, SelfPID: 100, SelfPath: "/Applications/Codepass.app/Contents/MacOS/Codepass"}, term, files, pr
}

func TestDetect(t *testing.T) {
	procs := []Proc{
		{1, "/sbin/launchd"},
		{100, "/Applications/Codepass.app/Contents/MacOS/Codepass"},
		{200, oldExe},
		{201, "/Users/alex/acme_api/bin/aws-sso-hub"},
		{202, `C:\Program Files\AWS SSO Hub\AWS SSO Hub.exe`},
		{203, "/Applications/AWS SSO Hub.app/Contents/MacOS/other"},
		{100, oldExe}, // own pid is never matched
	}
	got, err := Detect(fakeLister{procs: procs}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].PID != 200 || got[0].Bundle != oldBundle || got[1].Bundle != "" || got[2].PID != 202 {
		t.Fatalf("got %+v", got)
	}
}

func TestNoOldAppProceedsWithoutPrompt(t *testing.T) {
	d, term, _, pr := setup([]Proc{{1, "/sbin/launchd"}}, cQuit)
	if Run(d) != Proceed || pr.askedQuit != 0 || len(term.quit) != 0 {
		t.Fatal("must not prompt or touch anything")
	}
}

func TestListerErrorNeverBlocks(t *testing.T) {
	d, _, _, pr := setup(nil, cQuit)
	d.Lister = fakeLister{err: errors.New("ps failed")}
	if Run(d) != Proceed || pr.askedQuit != 0 {
		t.Fatal("a failed process list must not block the start")
	}
}

func TestQuitItThenKeepIt(t *testing.T) {
	d, term, files, pr := setup([]Proc{{200, oldExe}}, cQuit)
	d.Lang = "zh-CN"
	if Run(d) != Proceed {
		t.Fatal("expected Proceed")
	}
	if pr.askedQuit != 1 || pr.lang != "zh-CN" || len(term.quit) != 1 || term.quit[0] != 200 || len(files.moves) != 0 {
		t.Fatalf("asked=%d quit=%v moves=%v", pr.askedQuit, term.quit, files.moves)
	}
	// the second prompt names the exact bundle and "Keep it" moves nothing
	if pr.askedTrsh != 1 || len(pr.bundles) != 1 || pr.bundles[0] != oldBundle {
		t.Fatalf("trash prompt: asked=%d bundles=%v", pr.askedTrsh, pr.bundles)
	}
}

func TestCancelExits(t *testing.T) {
	d, term, files, pr := setup([]Proc{{200, oldExe}}, cCancel)
	if Run(d) != Exit || len(term.quit) != 0 || len(files.moves) != 0 || pr.askedTrsh != 0 {
		t.Fatal("cancel must change nothing")
	}
}

func TestStillRunningAfterTimeout(t *testing.T) {
	d, term, _, pr := setup([]Proc{{200, oldExe}}, cQuit)
	term.aliveFor = -1
	clock := d.Clock.(*fakeClock)
	start := clock.t
	if Run(d) != Exit || len(pr.told) != 1 {
		t.Fatal("expected Exit with a message")
	}
	if clock.t.Sub(start) < 10*time.Second {
		t.Fatalf("waited only %v", clock.t.Sub(start))
	}
}

func TestQuitErrorExits(t *testing.T) {
	d, term, _, pr := setup([]Proc{{200, oldExe}}, cQuit)
	term.quitErr = errors.New("denied")
	if Run(d) != Exit || len(pr.told) != 1 {
		t.Fatal("expected Exit")
	}
}

func TestTrashMovesBundleAfterExit(t *testing.T) {
	d, term, files, _ := setup([]Proc{{200, oldExe}}, cTrash)
	files.exist["/Users/alex/.Trash/AWS SSO Hub.app"] = true // name taken
	if Run(d) != Proceed {
		t.Fatal("expected Proceed")
	}
	if len(term.quit) != 1 || len(files.moves) != 1 || files.moves[0] != [2]string{oldBundle, "/Users/alex/.Trash/AWS SSO Hub 2.app"} {
		t.Fatalf("quit=%v moves=%v", term.quit, files.moves)
	}
}

func TestTrashOnlyTheOldBundle(t *testing.T) {
	// a dev binary has no bundle, and a differently named bundle is never moved: Trash is not offered
	for _, exe := range []string{"/Users/alex/acme_api/bin/aws-sso-hub", "/Applications/Other.app/Contents/MacOS/aws_sso_hub"} {
		d, term, files, pr := setup([]Proc{{200, exe}}, cTrash)
		if Run(d) != Proceed || pr.askedTrsh != 0 || len(term.quit) != 1 || len(files.moves) != 0 {
			t.Fatalf("%s: trash asked=%d quit=%v moves=%v", exe, pr.askedTrsh, term.quit, files.moves)
		}
	}
}

func TestNoTrashFolderOnThisPlatform(t *testing.T) {
	d, _, files, pr := setup([]Proc{{200, oldExe}}, cTrash)
	files.trash = ""
	if Run(d) != Proceed || pr.askedTrsh != 0 {
		t.Fatal("Trash must not be offered without a Trash folder")
	}
}

func TestTrashMoveFailureTellsPathAndProceeds(t *testing.T) {
	d, _, files, pr := setup([]Proc{{200, oldExe}}, cTrash)
	files.err = errors.New("permission denied")
	if Run(d) != Proceed || len(pr.told) != 1 || !strings.Contains(pr.told[0], oldBundle) {
		t.Fatalf("a failed move must not stop Codepass: told=%v", pr.told)
	}
}

func TestMissingBundleIsReported(t *testing.T) {
	d, _, files, pr := setup([]Proc{{200, oldExe}}, cTrash)
	delete(files.exist, oldBundle) // a translocated copy disappears when the app exits
	if Run(d) != Proceed || len(pr.told) != 1 || !strings.Contains(pr.told[0], oldBundle) {
		t.Fatalf("told=%v", pr.told)
	}
}

// changingLister returns a different list on each call.
type changingLister struct {
	lists [][]Proc
	n     int
}

func (c *changingLister) List() ([]Proc, error) {
	i := c.n
	if i >= len(c.lists) {
		i = len(c.lists) - 1
	}
	c.n++
	return c.lists[i], nil
}

func TestPIDIsCheckedAgainAfterThePrompt(t *testing.T) {
	// the old app quit while the prompt was open and its PID went to another program: nothing is signalled
	d, term, _, _ := setup(nil, cQuit)
	d.Lister = &changingLister{lists: [][]Proc{{{200, oldExe}}, {{200, "/usr/bin/vim"}}}}
	if Run(d) != Proceed || len(term.quit) != 0 {
		t.Fatalf("signalled %v", term.quit)
	}
}

func TestParsePS(t *testing.T) {
	got := parsePS("  1    0 /sbin/launchd\n 200  501 /Applications/AWS SSO Hub.app/Contents/MacOS/AWS SSO Hub\n 300  502 /Applications/AWS SSO Hub.app/Contents/MacOS/AWS SSO Hub\nbad line\n", 501)
	if len(got) != 1 || got[0].PID != 200 || got[0].Path != oldExe {
		t.Fatalf("only this user's processes are kept: %+v", got)
	}
}

func TestOldAppStartedAgainDuringPromptIsQuitBeforeTrash(t *testing.T) {
	d, term, files, _ := setup(nil, cTrash)
	cl := &changingLister{lists: [][]Proc{{{200, oldExe}}, {{300, oldExe}}, {}}}
	d.Lister = cl
	files.beforeMove = func() {
		if len(term.quit) != 1 || term.quit[0] != 300 {
			t.Errorf("the bundle was moved before the relaunched instance (pid 300) was quit: %v", term.quit)
		}
	}
	if Run(d) != Proceed || len(term.quit) != 1 || term.quit[0] != 300 {
		t.Fatalf("the new instance must be signalled: %v", term.quit)
	}
	if len(files.moves) != 1 {
		t.Fatalf("the bundle is moved to the Trash once the relaunched instance has quit: %v", files.moves)
	}
}

func TestStillRunningAfterQuitNeverTrashesOrProceeds(t *testing.T) {
	d, term, files, pr := setup(nil, cTrash)
	// the old app quits, but a new instance appears before the final check
	d.Lister = &changingLister{lists: [][]Proc{{{200, oldExe}}, {{200, oldExe}}, {{400, oldExe}}}}
	if Run(d) != Exit || len(files.moves) != 0 || pr.askedTrsh != 0 || len(pr.told) != 1 || len(term.quit) != 1 {
		t.Fatalf("outcome/moves=%v asked=%d told=%v", files.moves, pr.askedTrsh, pr.told)
	}
}

const winExe = `C:\Users\alex\Downloads\AWS SSO Hub 0.0.1\AWS SSO Hub.exe`

func TestWindowsRecyclesTheOldExeOnly(t *testing.T) {
	d, term, files, pr := setup([]Proc{{200, winExe}}, cTrash)
	files.recycle, files.trash = true, ""
	files.exist[winExe] = true
	if Run(d) != Proceed || len(term.quit) != 1 {
		t.Fatal("expected Proceed after quit")
	}
	if pr.askedTrsh != 1 || len(pr.bundles) != 1 || pr.bundles[0] != winExe {
		t.Fatalf("the prompt must name the exact exe: %v", pr.bundles)
	}
	if len(files.recycled) != 1 || files.recycled[0] != winExe || len(files.moves) != 0 {
		t.Fatalf("recycled=%v moves=%v", files.recycled, files.moves)
	}
}

func TestWindowsKeepItMovesNothing(t *testing.T) {
	d, _, files, pr := setup([]Proc{{200, winExe}}, cQuit)
	files.recycle, files.trash = true, ""
	files.exist[winExe] = true
	if Run(d) != Proceed || pr.askedTrsh != 1 || len(files.recycled) != 0 {
		t.Fatalf("asked=%d recycled=%v", pr.askedTrsh, files.recycled)
	}
}

func TestWindowsNeverRecyclesOtherExe(t *testing.T) {
	d, _, files, pr := setup(nil, cTrash)
	files.recycle, files.trash = true, ""
	for _, exe := range []string{`C:\tools\aws-sso-hub`, `C:\tools\notes.exe`} {
		files.exist[exe] = true
		if offerTrash(d, []Instance{{PID: 1, Path: exe}}); len(files.recycled) != 0 || pr.askedTrsh != 0 {
			t.Fatalf("%s: recycled=%v", exe, files.recycled)
		}
	}
	// trashAll refuses a path that is not the old exe even if called directly
	if msgs := trashAll(d, []string{`C:\Windows\System32\cmd.exe`}); len(msgs) != 1 || len(files.recycled) != 0 {
		t.Fatalf("msgs=%v recycled=%v", msgs, files.recycled)
	}
}

func TestWindowsRecycleFailureTellsPathAndProceeds(t *testing.T) {
	d, _, files, pr := setup([]Proc{{200, winExe}}, cTrash)
	files.recycle, files.trash = true, ""
	files.exist[winExe] = true
	files.recErr = errors.New("access denied")
	if Run(d) != Proceed || len(pr.told) != 1 || !strings.Contains(pr.told[0], winExe) {
		t.Fatalf("told=%v", pr.told)
	}
}
