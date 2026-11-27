package accounts

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/sso"
)

func (e *Engine) planRenamePortal(oldName, newName string) (*awsconfig.Snapshot, []byte, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return nil, nil, err
	}
	if !awsconfig.SessionNameRE.MatchString(newName) {
		return nil, nil, api.NewError(api.ErrInvalidName, "invalid portal name").WithParam("name", newName)
	}
	if _, ok := snap.Session(oldName); !ok {
		return nil, nil, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", oldName)
	}
	if _, ok := snap.Session(newName); ok || e.IsDraft(newName) {
		return nil, nil, api.NewError(api.ErrPortalExists, "portal exists").WithParam("portal", newName)
	}
	after, rerr := awsconfig.RenameSession(snap.ConfigBytes(), oldName, newName)
	if rerr != nil {
		return nil, nil, rerr
	}
	return snap, after, nil
}

func (e *Engine) PreviewRenamePortal(oldName, newName string) (api.FilePreview, error) {
	snap, after, err := e.planRenamePortal(oldName, newName)
	if err != nil {
		return api.FilePreview{}, err
	}
	return e.preview(e.d.Paths.AWSConfigFile, snap.ConfigBytes(), after), nil
}

// RenamePortal renames the sso-session, moves the token file and the session record, and drops the old
// role caches (they are keyed by the session name).
func (e *Engine) RenamePortal(oldName, newName, baseHash string) (api.WriteResult, error) {
	e.stopAutoSignIn(oldName) // a sign-in page left open would sign in under the old name
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	snap, after, err := e.planRenamePortal(oldName, newName)
	if err != nil {
		return api.WriteResult{}, err
	}
	roles := e.sessionRoles(snap, oldName)
	id, werr := e.writeConfig(snap, baseHash, after, api.ReasonPortal, map[string]string{"portal": oldName})
	if werr != nil {
		return api.WriteResult{}, werr
	}
	dir := sso.TokenCacheDir(e.d.Paths.Home)
	oldTok := filepath.Join(dir, sso.TokenCacheKey(oldName, "")+".json")
	newTok := filepath.Join(dir, sso.TokenCacheKey(newName, "")+".json")
	if _, serr := os.Stat(oldTok); serr == nil {
		_ = os.Rename(oldTok, newTok)
	}
	e.deleteRoleCaches(oldName, roles)
	recMu.Lock()
	if r, ok := e.d.Store.SessionRecord(oldName); ok {
		_ = e.d.Store.SetSessionRecord(newName, r)
		_ = e.d.Store.DeleteSessionRecord(oldName)
	}
	recMu.Unlock()
	e.mu.Lock()
	delete(e.lastSig, oldName)
	delete(e.autoDone, oldName)
	e.mu.Unlock()
	e.EmitState(context.Background())
	return api.WriteResult{File: e.display(e.d.Paths.AWSConfigFile), BackupID: id}, nil
}

func (e *Engine) planRemovePortal(name string) (*awsconfig.Snapshot, []byte, error) {
	snap, err := e.Snapshot()
	if err != nil {
		return nil, nil, err
	}
	if _, ok := snap.Session(name); !ok {
		return nil, nil, api.NewError(api.ErrPortalNotFound, "portal not found").WithParam("portal", name)
	}
	n := 0
	for _, p := range snap.Profiles() {
		if p.SSOSession == name {
			n++
		}
	}
	if n > 0 {
		return nil, nil, api.NewError(api.ErrPortalInUse, "profiles still use the portal").WithParam("portal", name).WithParam("n", itoa(n))
	}
	after, rerr := awsconfig.RemoveSession(snap.ConfigBytes(), name)
	if rerr != nil {
		return nil, nil, rerr
	}
	return snap, after, nil
}

func (e *Engine) PreviewRemovePortal(name string) (api.FilePreview, error) {
	snap, after, err := e.planRemovePortal(name)
	if err != nil {
		return api.FilePreview{}, err
	}
	return e.preview(e.d.Paths.AWSConfigFile, snap.ConfigBytes(), after), nil
}

// RemovePortal deletes an unused sso-session, its token file and its session record (no server logout).
func (e *Engine) RemovePortal(name, baseHash string) (api.WriteResult, error) {
	e.stopAutoSignIn(name)
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	snap, after, err := e.planRemovePortal(name)
	if err != nil {
		return api.WriteResult{}, err
	}
	id, werr := e.writeConfig(snap, baseHash, after, api.ReasonPortal, map[string]string{"portal": name})
	if werr != nil {
		return api.WriteResult{}, werr
	}
	tok := filepath.Join(sso.TokenCacheDir(e.d.Paths.Home), sso.TokenCacheKey(name, "")+".json")
	if rerr := os.Remove(tok); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
		return api.WriteResult{}, api.NewError(api.ErrFileIO, "cannot delete the token file").WithParam("file", e.display(tok))
	}
	recMu.Lock()
	_ = e.d.Store.DeleteSessionRecord(name)
	recMu.Unlock()
	e.mu.Lock()
	delete(e.lastSig, name)
	delete(e.autoDone, name)
	e.mu.Unlock()
	e.EmitState(context.Background())
	return api.WriteResult{File: e.display(e.d.Paths.AWSConfigFile), BackupID: id}, nil
}
