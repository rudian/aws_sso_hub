package accounts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsapi"
	"github.com/rudian/aws_sso_hub/internal/awsapi/awsapitest"
	"github.com/rudian/aws_sso_hub/internal/backup"
	"github.com/rudian/aws_sso_hub/internal/paths"
	"github.com/rudian/aws_sso_hub/internal/sso"
	"github.com/rudian/aws_sso_hub/internal/sso/ssotest"
)

// fakeStore is an in-memory Store.
type fakeStore struct {
	mu         sync.Mutex
	colors     map[string]int
	monograms  map[string]string
	names      map[string]string
	identities map[string]api.StaticIdentity
	sessions   map[string]api.SessionRecord
	autoRenew  bool
	lead       time.Duration
	notice     bool
	colorSets  int
}

func newFakeStore() *fakeStore {
	return &fakeStore{colors: map[string]int{}, monograms: map[string]string{}, names: map[string]string{},
		identities: map[string]api.StaticIdentity{}, sessions: map[string]api.SessionRecord{}, autoRenew: true, lead: 15 * time.Minute, notice: true}
}

func (s *fakeStore) AccountColors() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{}
	for k, v := range s.colors {
		out[k] = v
	}
	return out
}
func (s *fakeStore) SetAccountColors(c map[string]int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.colors = map[string]int{}
	for k, v := range c {
		s.colors[k] = v
	}
	s.colorSets++
	return nil
}
func (s *fakeStore) Monograms() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.monograms {
		out[k] = v
	}
	return out
}
func (s *fakeStore) SetMonogram(p, m string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m == "" {
		delete(s.monograms, p)
	} else {
		s.monograms[p] = m
	}
	return nil
}
func (s *fakeStore) AccountNames() map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, v := range s.names {
		out[k] = v
	}
	return out
}
func (s *fakeStore) MergeAccountNames(n map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range n {
		s.names[k] = v
	}
	return nil
}
func (s *fakeStore) StaticIdentity(p string) (api.StaticIdentity, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, ok := s.identities[p]
	return i, ok
}
func (s *fakeStore) SetStaticIdentity(i api.StaticIdentity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identities[i.Profile] = i
	return nil
}
func (s *fakeStore) SessionRecord(p string) (api.SessionRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.sessions[p]
	return r, ok
}
func (s *fakeStore) SetSessionRecord(p string, r api.SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[p] = r
	return nil
}
func (s *fakeStore) DeleteSessionRecord(p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, p)
	return nil
}
func (s *fakeStore) AutoRenew() bool             { return s.autoRenew }
func (s *fakeStore) ReminderLead() time.Duration { return s.lead }
func (s *fakeStore) ExpiryNotice() bool          { return s.notice }

type event struct {
	name string
	data any
}

type harness struct {
	t     *testing.T
	p     paths.Paths
	store *fakeStore
	oidc  *ssotest.OIDC
	por   *ssotest.Portal
	aws   *awsapitest.Fake
	sso   *sso.Manager
	eng   *Engine
	bk    *backup.Store

	mu     sync.Mutex
	now    time.Time
	events []event
	writes []string
}

func (h *harness) clock() time.Time { h.mu.Lock(); defer h.mu.Unlock(); return h.now }
func (h *harness) advance(d time.Duration) {
	h.mu.Lock()
	h.now = h.now.Add(d)
	h.mu.Unlock()
}

func (h *harness) emitted(name string) []any {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []any
	for _, e := range h.events {
		if e.name == name {
			out = append(out, e.data)
		}
	}
	return out
}

func (h *harness) names() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, e := range h.events {
		out = append(out, e.name)
	}
	return out
}

func readTestdata(t *testing.T, name string) []byte {
	b, err := os.ReadFile(filepath.Join("..", "awsconfig", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// newHarness builds an engine over a sandbox home with the fixture files.
func newHarness(t *testing.T, withFixtures bool) *harness {
	t.Helper()
	root := t.TempDir()
	p := paths.ForRoot(root)
	if err := p.EnsureAppDirs(); err != nil {
		t.Fatal(err)
	}
	if withFixtures {
		os.MkdirAll(filepath.Dir(p.AWSConfigFile), 0o700)
		os.WriteFile(p.AWSConfigFile, readTestdata(t, "config.fixture"), 0o600)
		os.WriteFile(p.AWSCredentialsFile, readTestdata(t, "credentials.fixture"), 0o600)
	}
	h := &harness{t: t, p: p, store: newFakeStore(), oidc: ssotest.NewOIDC(t), por: ssotest.NewPortal(t), aws: awsapitest.New(t),
		now: time.Now().UTC().Truncate(time.Second)}
	h.sso = sso.NewManager(sso.Config{
		Home: p.Home, RegistrationsDir: p.RegistrationsDir, Endpoints: sso.Endpoints{OIDC: h.oidc.URL, Portal: h.por.URL},
		OpenBrowser: ssotest.Browser(), Lang: func() string { return "en" }, Now: h.clock,
	})
	bk, err := backup.Open(p.BackupDir, func() int { return 20 }, h.clock)
	if err != nil {
		t.Fatal(err)
	}
	h.bk = bk
	h.eng = New(Deps{
		Paths: p, Sessions: h.sso, Backup: bk, Store: h.store, Endpoints: awsapi.Endpoints{STS: h.aws.URL, IAM: h.aws.URL},
		Emit: func(name string, data any) {
			h.mu.Lock()
			h.events = append(h.events, event{name, data})
			h.mu.Unlock()
		},
		Now: h.clock,
		OnWrite: func(path string, content []byte) {
			h.mu.Lock()
			h.writes = append(h.writes, path)
			h.mu.Unlock()
		},
	})
	return h
}

var portalInitech = sso.Session{Name: "initech", StartURL: "https://d-1234567890.awsapps.com/start", Region: "us-west-2", Scopes: []string{"sso:account:access"}, ScopesSet: true}

// writeToken writes a CLI-format token for initech relative to the harness clock.
func (h *harness) writeToken(left time.Duration, clientID string) {
	tok := &sso.CachedToken{StartURL: portalInitech.StartURL, Region: portalInitech.Region, AccessToken: "AT-seed", ExpiresAt: h.clock().Add(left),
		ClientID: clientID, ClientSecret: "CS-seed", RegistrationExpiresAt: h.clock().Add(80 * 24 * time.Hour), RefreshToken: "RT-seed"}
	if err := sso.WriteToken(sso.TokenCacheDir(h.p.Home), sso.TokenCacheKey("initech", ""), tok); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) setRecord(loginAgo time.Duration, clientID string, minutes int) {
	h.store.SetSessionRecord("initech", api.SessionRecord{
		LoginAt: h.clock().Add(-loginAgo).Format(time.RFC3339), ClientIDHash: sso.ClientIDHash(clientID),
		TokenIssuedAt: h.clock().Format(time.RFC3339), SessionMinutes: minutes})
}

func (h *harness) status(portal string) api.SessionStatus {
	h.t.Helper()
	st, err := h.eng.SessionStatus(portal)
	if err != nil {
		h.t.Fatal(err)
	}
	return st
}

func jsonOf(v any) string { b, _ := json.Marshal(v); return string(b) }

var bg = context.Background()

func awsapiIdentity(account, arn string) awsapitest.Identity {
	return awsapitest.Identity{Account: account, Arn: arn, UserID: "AIDAFAKE"}
}

func time_hours(n int) time.Duration { return time.Duration(n) * time.Hour }
