package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rudian/codepass/internal/paths"
	"github.com/rudian/codepass/internal/projects"
	"github.com/rudian/codepass/internal/store"
)

// ErrStartupStopped means OpenContainer told the person why the app cannot start and the caller should quit
// quietly. Nothing was written.
var ErrStartupStopped = errors.New("startup stopped after telling the user")

// OpenContainer is NewContainer plus the startup handling of projects.json (FU-18d), which must happen before the
// window opens:
//   - a file written by a newer Codepass: tell(message) and return ErrStartupStopped; the file is never touched;
//   - a damaged file: move it aside (projects.Quarantine) and build the container again with a fresh store.
//
// tell shows a native message; the settings (and so the UI language) are not readable at that point, so the
// message carries both languages.
func OpenContainer(p paths.Paths, rt Runtime, o ContainerOptions, tell func(msg string)) (*Container, error) {
	c, err := NewContainer(p, rt, o)
	if err == nil {
		return c, nil
	}
	var newer *store.NewerVersionError
	switch {
	case errors.As(err, &newer):
		if o.Logger != nil {
			o.Logger.Error("projects.json was written by a newer version", "fileVersion", newer.FileVersion, "appVersion", newer.AppVersion)
		}
		if tell != nil {
			tell(newerVersionMessage(newer))
		}
		return nil, ErrStartupStopped
	case errors.Is(err, store.ErrNewerVersion):
		if tell != nil {
			tell(newerVersionMessage(nil))
		}
		return nil, ErrStartupStopped
	case errors.Is(err, store.ErrCorrupt):
		// Only projects.json is moved aside; another damaged store file is reported as it is.
		var sf *storeFileError
		if !errors.As(err, &sf) || sf.File != projectsFile(p) {
			return nil, err
		}
		moved, qerr := projects.Quarantine(sf.File)
		if qerr != nil {
			return nil, fmt.Errorf("projects.json is damaged and cannot be moved aside: %w", qerr)
		}
		if o.Logger != nil {
			o.Logger.Warn("projects.json was damaged; moved aside and starting with a new one", "movedTo", moved)
		}
		if tell != nil {
			tell(damagedMessage(moved))
		}
		return NewContainer(p, rt, o)
	}
	return nil, err
}

// storeFileError names the store file whose opening failed, so the caller acts on the right file.
type storeFileError struct {
	File string
	Err  error
}

func (e *storeFileError) Error() string { return e.File + ": " + e.Err.Error() }
func (e *storeFileError) Unwrap() error { return e.Err }

// releasesAddress is ReleasesURL as people read it: without "https://" and "/latest".
func releasesAddress() string {
	return strings.TrimSuffix(strings.TrimPrefix(ReleasesURL, "https://"), "/latest")
}

// newerVersionMessage keeps the schema numbers out of the dialog (they are in the log).
func newerVersionMessage(_ *store.NewerVersionError) string {
	addr := releasesAddress()
	return "你的项目数据是由更新版本的 Codepass 写入的。这个版本不会打开它，也没有改动它。请打开较新的 Codepass，或到 " + addr + " 下载最新版本。\n\n" +
		"Your project data was written by a newer Codepass. This copy will not open it and changed nothing. Open the newer Codepass, or download the latest version from " + addr + "."
}

func damagedMessage(movedTo string) string {
	return "项目分组的数据坏了，已另存为 " + movedTo + "。仓库本身没有动，分组要重新建。\n\n" +
		"Your project list was damaged and was kept as " + movedTo + ". Your repositories are untouched; the projects start empty."
}
