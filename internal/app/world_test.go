package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsapi/awsapitest"
	"github.com/rudian/aws_sso_hub/internal/gitx/gitxtest"
	"github.com/rudian/aws_sso_hub/internal/logx"
	"github.com/rudian/aws_sso_hub/internal/paths"
	"github.com/rudian/aws_sso_hub/internal/sso"
	"github.com/rudian/aws_sso_hub/internal/sso/ssotest"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The secrets the bridge must never carry.
var secretStrings = []string{
	"AKIAIOSFODNN7EXAMPLE", "fakeSecretValueDoNotUse", "ACCESS-TOKEN-SECRET", "REFRESH-TOKEN-SECRET",
	"CLIENT-SECRET-VALUE", "ASIAROLEKEY00EXAMPLE",
}

const (
	hostAP      = "git-codecommit.us-east-1.amazonaws.com"
	acctAcme    = "111122223333"
	acctInitech = "777788889999"
	portalURL   = "https://d-1234567890.awsapps.com/start"
)

func ccURL(name string) string { return "https://" + hostAP + "/v1/repos/" + name }

const configText = `[profile acme]
region = us-east-1
output = json

[profile globex]
region = eu-west-1

[sso-session initech]
sso_start_url  = ` + portalURL + `
sso_region     = us-west-2
sso_registration_scopes = sso:account:access

[profile initech-admin]
sso_session    = initech
sso_account_id = ` + acctInitech + `
sso_role_name  = AdministratorAccess
region         = us-west-2

[sso-session other]
sso_start_url  = https://d-0000000000.awsapps.com/start
sso_region     = us-east-1
`

const credentialsText = `[acme]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = fakeSecretValueDoNotUse0000000000000000

[globex]
aws_access_key_id = AKIAI44QH8DHBEXAMPLE
aws_secret_access_key = fakeSecretValueDoNotUse1111111111111111
`

const globalGit = "[credential]\n\thelper = \n\thelper = !aws codecommit credential-helper $@ --profile acme\n\tusehttppath = true\n" +
	"[credential \"https://" + hostAP + "/v1/repos/infra-terraform\"]\n\thelper = \n\thelper = !aws --profile initech-admin codecommit credential-helper $@\n"

// world is a complete sandbox: files, fake AWS servers, a fake browser and a container wired to them.
type world struct {
	t    *testing.T
	p    paths.Paths
	rt   *fakeRuntime
	c    *Container
	oidc *ssotest.OIDC
	por  *ssotest.Portal
	aws  *awsapitest.Fake
	cc   *httptest.Server
	fake string // path of the fake aws binary

	acc  *AccountService
	mig  *MigrationService
	repo *RepoService
	glob *GitGlobalService
	sys  *SystemService

	mu       sync.Mutex
	recorded []string // JSON of every bound-method result and error
}

func newWorld(t *testing.T, opts ...func(*ContainerOptions)) *world {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	p := paths.ForRoot(root)
	if err := p.EnsureAppDirs(); err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, p: p, rt: &fakeRuntime{}}
	active = w
	t.Cleanup(func() { active = nil })
	mk := func(path, content string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk(p.AWSConfigFile, configText)
	mk(p.AWSCredentialsFile, credentialsText)
	mk(p.GitGlobalFile, globalGit)

	w.oidc, w.por, w.aws = ssotest.NewOIDC(t), ssotest.NewPortal(t), awsapitest.New(t)
	const roleKey = "ASIAROLEKEY00EXAMPLE"
	w.por.SetKeyID(roleKey)
	w.por.SetAccounts([]ssotest.Account{{ID: acctAcme, Name: "acme", Email: "s@example.com"}})
	w.por.SetRoles(acctAcme, "AdministratorAccess")
	w.por.SetRoles(acctInitech, "AdministratorAccess")
	w.aws.Identities["AKIAIOSFODNN7EXAMPLE"] = awsapitest.Identity{Account: acctAcme, Arn: "arn:aws:iam::" + acctAcme + ":user/acme-dev", UserID: "AIDA1"}
	w.aws.Identities[roleKey] = awsapitest.Identity{Account: acctAcme, Arn: "arn:aws:sts::" + acctAcme + ":assumed-role/AWSReservedSSO_AdministratorAccess_abc/alex", UserID: "AROA1:alex"}
	w.aws.Keys["acme-dev"] = []awsapitest.Key{{ID: "AKIAIOSFODNN7EXAMPLE", Status: "Active"}}

	// a second portal that is already signed in with tokens that must never leave the process
	tok := &sso.CachedToken{StartURL: "https://d-0000000000.awsapps.com/start", Region: "us-east-1", AccessToken: "ACCESS-TOKEN-SECRET",
		ExpiresAt: time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second), ClientID: "CID-OTHER", ClientSecret: "CLIENT-SECRET-VALUE",
		RegistrationExpiresAt: time.Now().Add(80 * 24 * time.Hour), RefreshToken: "REFRESH-TOKEN-SECRET"}
	if err := sso.WriteToken(sso.TokenCacheDir(p.Home), sso.TokenCacheKey("other", ""), tok); err != nil {
		t.Fatal(err)
	}

	w.cc = httptest.NewServer(http.HandlerFunc(w.serveCodeCommit))
	t.Cleanup(w.cc.Close)

	gs := gitxtest.New(t) // also configures execx without a login shell
	w.fake = gs.FakeAWS(t, filepath.Join(root, "fakebin"), "ok")

	// hermetic: never run the real aws binary, even for `--version`
	mk(p.SettingsFile, `{"awsCliPath": `+strconv.Quote(w.fake)+`}`)

	w.rt.browser = func(u string) { _ = ssotest.Browser()(u) }
	logger, logCloser, err := logx.New(p.LogDir, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logCloser.Close() })
	o := ContainerOptions{
		Logger:            logger,
		Endpoints:         Endpoints{OIDC: w.oidc.URL, Portal: w.por.URL, STS: w.aws.URL, IAM: w.aws.URL, CodeCommit: w.cc.URL, SignIn: w.aws.URL},
		DisableLoginShell: true,
		WatchInterval:     50 * time.Millisecond,
		TickInterval:      time.Hour,
		ScanDelay:         time.Hour,
	}
	for _, f := range opts {
		f(&o)
	}
	c, err := NewContainer(p, w.rt, o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)
	c.files.settle = 5 * time.Millisecond
	w.c = c
	w.acc, w.mig, w.repo, w.glob, w.sys = NewAccountService(c), NewMigrationService(c), NewRepoService(c), NewGitGlobalService(c), NewSystemService(c)
	return w
}

// serveCodeCommit answers BatchGetRepositories: every repository lives in the account named in repoOwners.
var repoOwners = map[string]string{"web-app": acctAcme, "infra-terraform": acctInitech}

func (w *world) serveCodeCommit(rw http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body struct {
		RepositoryNames []string `json:"repositoryNames"`
	}
	_ = json.Unmarshal(raw, &body)
	op := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "CodeCommit_20150413.")
	rw.Header().Set("Content-Type", "application/x-amz-json-1.1")
	if op != "BatchGetRepositories" {
		rw.WriteHeader(400)
		_, _ = rw.Write([]byte(`{"__type":"InvalidAction"}`))
		return
	}
	found := []map[string]any{}
	missing := []string{}
	for _, n := range body.RepositoryNames {
		if acct, ok := repoOwners[n]; ok {
			found = append(found, map[string]any{"accountId": acct, "repositoryId": "id-" + n, "repositoryName": n, "defaultBranch": "main",
				"cloneUrlHttp": ccURL(n), "lastModifiedDate": 1.7e9})
		} else {
			missing = append(missing, n)
		}
	}
	_ = json.NewEncoder(rw).Encode(map[string]any{"repositories": found, "repositoriesNotFound": missing})
}

// gitRepo creates <home>/Documents/code/<name> with an origin remote.
func (w *world) gitRepo(name, remote string) string {
	w.t.Helper()
	dir := filepath.Join(w.p.DefaultScanRoot, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		w.t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", remote}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), w.p.ChildEnv()...)
		if out, err := cmd.CombinedOutput(); err != nil {
			w.t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

// rec stores the JSON of a result (or error) for the secret scan.
func (w *world) rec(v any, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if b, merr := json.Marshal(v); merr == nil {
		w.recorded = append(w.recorded, string(b))
	}
	if err != nil {
		if b, merr := json.Marshal(api.AsAppError(err)); merr == nil {
			w.recorded = append(w.recorded, string(b))
		}
	}
}

// active is the world of the running test (tests in this package do not run in parallel).
var active *world

// ok records the result of a bound call and fails the test on error: ok(w.acc.GetState(bg)).
func ok[T any](v T, err error) T {
	w := active
	w.t.Helper()
	w.rec(v, err)
	if err != nil {
		w.t.Fatalf("unexpected error: %v", err)
	}
	return v
}

// fails records an expected failure and returns the AppError.
func fails[T any](v T, err error) *api.AppError {
	w := active
	w.t.Helper()
	w.rec(v, err)
	if err == nil {
		w.t.Fatal("expected an error")
	}
	var ae *api.AppError
	if !errors.As(err, &ae) {
		w.t.Fatalf("error is not an *api.AppError: %T %v", err, err)
	}
	return ae
}

func (w *world) read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		w.t.Fatal(err)
	}
	return string(b)
}

var bg = context.Background()

func applicationServiceOptions() application.ServiceOptions { return application.ServiceOptions{} }
