package identity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/projects"
)

// undoFile restores one config file: the backup taken before the write, and the hash the app left behind
// (an undo is refused when the file changed since).
type undoFile struct {
	path      string
	file      string // display path
	backupID  string // "" when the file did not exist before
	existed   bool
	afterHash string
}

// undoRecord restores one value in projects.json.
type undoRecord struct {
	kind  string // project-identity | repo-override | repo-project
	id    string
	has   bool   // the value existed before
	name  string // identity kinds
	email string // identity kinds
	value string // repo-project: the previous project ID ("" = none)
}

// undoState is everything one Undo needs. It never leaves Go: the page only gets the ID, so a caller cannot
// make Undo write or delete a file the engine did not itself write.
type undoState struct {
	files   []undoFile
	records []undoRecord
}

// maxUndoStates bounds how many applies can still be undone (the oldest is forgotten first).
const maxUndoStates = 32

type undoStore struct {
	m     map[string]undoState
	order []string
}

// remember keeps st and returns its ID ("" when there is nothing to undo).
func (e *Engine) remember(st undoState) string {
	if len(st.files) == 0 && len(st.records) == 0 {
		return ""
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	id := hex.EncodeToString(b[:])
	e.undoMu.Lock()
	defer e.undoMu.Unlock()
	if e.undos.m == nil {
		e.undos.m = map[string]undoState{}
	}
	e.undos.m[id] = st
	e.undos.order = append(e.undos.order, id)
	for len(e.undos.order) > maxUndoStates {
		delete(e.undos.m, e.undos.order[0])
		e.undos.order = e.undos.order[1:]
	}
	return id
}

func (e *Engine) lookupUndo(id string) (undoState, bool) {
	e.undoMu.Lock()
	defer e.undoMu.Unlock()
	st, ok := e.undos.m[id]
	return st, ok
}

func (e *Engine) forgetUndo(id string) {
	e.undoMu.Lock()
	defer e.undoMu.Unlock()
	delete(e.undos.m, id)
	for i, o := range e.undos.order {
		if o == id {
			e.undos.order = append(e.undos.order[:i], e.undos.order[i+1:]...)
			break
		}
	}
}

// restoreFile puts one file back as it was before the write.
func (e *Engine) restoreFile(f undoFile) error {
	if !f.existed {
		if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
			return api.NewError(api.ErrFileIO, "cannot remove a git config file").WithParam("file", f.file).WithDetail(err.Error())
		}
		return nil
	}
	if e.d.Backup == nil || f.backupID == "" {
		return api.NewError(api.ErrBackupNotFound, "backup not found")
	}
	return e.restoreFrom(f.backupID, f.path)
}

// Undo reverses a change the engine made earlier: every config file goes back from its backup (refused with
// E_CONFIG_CHANGED when a file is no longer as the app left it; nothing is touched then), then the values in
// projects.json. A safety backup of each file is taken first, so the undo can be undone from the backups
// list. id is the UndoID an apply returned; an unknown or already used ID is refused.
func (e *Engine) Undo(ctx context.Context, id string) (api.IdentityChangeResult, error) {
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	tok, ok := e.lookupUndo(id)
	if !ok {
		return api.IdentityChangeResult{}, api.NewError(api.ErrInvalidInput, "nothing to undo").WithParam("field", "undoId")
	}
	for _, f := range tok.files {
		cur, ex, err := readFile(f.path)
		if err != nil {
			return api.IdentityChangeResult{}, err
		}
		if hashOf(cur, ex) != f.afterHash {
			return api.IdentityChangeResult{}, api.NewError(api.ErrConfigChanged, "the file changed since it was written").WithParam("file", f.file)
		}
		if f.existed {
			if e.d.Backup == nil {
				return api.IdentityChangeResult{}, api.NewError(api.ErrBackupNotFound, "backup not found")
			}
			if _, _, err := e.d.Backup.Get(f.backupID); err != nil {
				return api.IdentityChangeResult{}, api.AsAppError(err)
			}
		}
	}
	res := api.IdentityChangeResult{Outcomes: []api.RepoChangeOutcome{}, BackupIDs: []string{}}
	for _, f := range tok.files {
		kind := api.BackupGitLocal
		if g, err := e.git(ctx); err == nil && f.path == e.globalPath(g) {
			kind = api.BackupGitGlobal
		}
		if e.d.Backup != nil {
			if ent, err := e.d.Backup.Snapshot(f.path, f.file, kind, api.ReasonRestore, map[string]string{"file": f.file}); err == nil && ent.ID != "" {
				res.BackupIDs = append(res.BackupIDs, ent.ID)
			}
		}
		if err := e.restoreFile(f); err != nil {
			return res, api.AsAppError(err)
		}
		if kind == api.BackupGitGlobal && e.d.OnWrite != nil {
			// nil content = the file is gone (it did not exist before the write)
			b, _, _ := readFile(f.path)
			e.d.OnWrite(f.path, b)
		}
	}
	if len(tok.records) > 0 {
		err := e.d.Projects.Mutate(func(d *projects.Data) error {
			for _, r := range tok.records {
				switch r.kind {
				case "project-identity":
					if p := d.FindProject(r.id); p != nil {
						p.Identity = nil
						if r.has {
							p.Identity = &projects.Identity{Name: r.name, Email: r.email}
						}
					}
				case "repo-override":
					if p := d.FindRepo(r.id); p != nil {
						p.IdentityOverride = nil
						if r.has {
							p.IdentityOverride = &projects.Identity{Name: r.name, Email: r.email}
						}
					}
				case "repo-project":
					if p := d.FindRepo(r.id); p != nil && (r.value == "" || d.FindProject(r.value) != nil) {
						p.ProjectID = r.value
					}
				}
			}
			return nil
		})
		if err != nil {
			return res, api.NewError(api.ErrFileIO, "cannot write projects.json").WithParam("file", "projects.json").WithDetail(err.Error())
		}
	}
	e.forgetUndo(id)
	res.RecordUpdated = len(tok.records) > 0
	e.emit()
	return res, nil
}
