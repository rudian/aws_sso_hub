package identity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rudian/codepass/internal/api"
)

// undoFiles returns the files an apply recorded for its undo.
func (r *rig) undoFiles(res api.IdentityChangeResult) []undoFile {
	st, _ := r.eng.lookupUndo(res.UndoID)
	return st.files
}

// An undo ID the engine never handed out does nothing: the page cannot name a path, a backup or a record.
func TestUndoRefusesUnknownIDs(t *testing.T) {
	r := newRig(t)
	outside := filepath.Join(r.root, "outside", "Library", "LaunchAgents", "x.zshenv")
	victim := filepath.Join(r.root, "victim")
	r.write(victim, "")
	for _, id := range []string{"", "nope", "../../etc/passwd", outside} {
		if _, err := r.eng.Undo(bg, id); api.AsAppError(err).Code != api.ErrInvalidInput {
			t.Fatalf("%q: %v", id, err)
		}
	}
	if _, err := os.Stat(filepath.Dir(outside)); err == nil {
		t.Fatal("a folder was created from a forged undo")
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("an unrelated file was deleted")
	}
	if len(r.proj.Snapshot().Projects) != 0 {
		t.Fatal("a record was written")
	}
}

// An ID works once; a second use is refused, and the files stay as the first undo left them.
func TestUndoIDIsSingleUse(t *testing.T) {
	r := newRig(t)
	res, err := r.eng.ApplyGlobal(bg, acme(), "")
	if err != nil || res.UndoID == "" {
		t.Fatalf("%v %+v", err, res)
	}
	if _, err := r.eng.Undo(bg, res.UndoID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.eng.Undo(bg, res.UndoID); api.AsAppError(err).Code != api.ErrInvalidInput {
		t.Fatalf("%v", err)
	}
}

func TestUndoStatesAreBounded(t *testing.T) {
	r := newRig(t)
	first := r.eng.remember(undoState{records: []undoRecord{{kind: "repo-project", id: "x"}}})
	for i := 0; i < maxUndoStates; i++ {
		r.eng.remember(undoState{records: []undoRecord{{kind: "repo-project", id: "x"}}})
	}
	if _, ok := r.eng.lookupUndo(first); ok {
		t.Fatal("the oldest state must be forgotten")
	}
	if id := r.eng.remember(undoState{}); id != "" {
		t.Fatal("nothing to undo means no ID")
	}
}

// What an undo restores is what the engine recorded for the write: the global config path and its own backup.
func TestUndoWritesOnlyRecordedPaths(t *testing.T) {
	r := newRig(t)
	r.write(r.p.GitGlobalFile, "[core]\n\teditor = vim\n")
	res, err := r.eng.ApplyGlobal(bg, acme(), hashOf([]byte(r.read(r.p.GitGlobalFile)), true))
	if err != nil {
		t.Fatal(err)
	}
	fs := r.undoFiles(res)
	if len(fs) != 1 || fs[0].path != r.p.GitGlobalFile || fs[0].backupID == "" || !fs[0].existed {
		t.Fatalf("%+v", fs)
	}
}
