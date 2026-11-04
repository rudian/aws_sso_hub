package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/backup"
	"github.com/rudian/aws_sso_hub/internal/version"
)

type selfTestTool struct {
	Status api.ToolStatus `json:"status"`
	Path   string         `json:"path"`
}

type selfTestReport struct {
	OK       bool         `json:"ok"`
	Version  string       `json:"version"`
	Sandbox  bool         `json:"sandbox"`
	Profiles int          `json:"profiles"`
	Portals  int          `json:"portals"`
	Repos    int          `json:"repos"`
	AWSCLI   selfTestTool `json:"awsCli"`
	Git      selfTestTool `json:"git"`
	Errors   []string     `json:"errors"`
}

// RunSelfTest calls the four state methods once and writes the JSON report to file (sandbox smoke test).
// The caller quits the app afterwards.
func RunSelfTest(c *Container, file string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	rep := selfTestReport{Version: version.Version, Sandbox: c.Paths.Sandbox, Errors: []string{}}
	note := func(err error) {
		if err != nil {
			rep.Errors = append(rep.Errors, string(api.AsAppError(err).Code))
		}
	}
	acc := &AccountService{c: c}
	repo := &RepoService{c: c}
	glob := &GitGlobalService{c: c}
	sys := &SystemService{c: c}

	st, err := acc.GetState(ctx)
	note(err)
	rep.Profiles = len(st.Profiles)
	for _, p := range st.Portals {
		if !p.Draft {
			rep.Portals++
		}
	}
	rs, err := repo.GetState(ctx)
	note(err)
	rep.Repos = rs.Counts.All
	_, err = glob.GetState(ctx)
	note(err)
	env, err := sys.CheckEnvironment(ctx)
	note(err)
	rep.AWSCLI = selfTestTool{Status: env.AWSCLI.Status, Path: env.AWSCLI.Path}
	rep.Git = selfTestTool{Status: env.Git.Status, Path: env.Git.Path}
	rep.OK = len(rep.Errors) == 0

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return err
	}
	return backup.WriteFileAtomic(file, append(b, '\n'), 0o600)
}
