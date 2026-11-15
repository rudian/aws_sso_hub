package codecommit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"

	"github.com/rudian/codepass/internal/api"
)

type fakeCC struct {
	mu      sync.Mutex
	targets []string
	regions []string
	bodies  []map[string]any
	handler func(op string, body map[string]any) (int, any)
}

func newFakeCC(t *testing.T, h func(op string, body map[string]any) (int, any)) (*fakeCC, string) {
	f := &fakeCC{handler: h}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		op := strings.TrimPrefix(r.Header.Get("X-Amz-Target"), "CodeCommit_20150413.")
		auth := r.Header.Get("Authorization")
		region := ""
		if i := strings.Index(auth, "Credential="); i >= 0 {
			parts := strings.Split(strings.Fields(auth[i+11:])[0], "/")
			if len(parts) > 2 {
				region = parts[2]
			}
		}
		f.mu.Lock()
		f.targets = append(f.targets, op)
		f.regions = append(f.regions, region)
		f.bodies = append(f.bodies, body)
		f.mu.Unlock()
		code, resp := f.handler(op, body)
		w.Header().Set("Content-Type", "application/x-amz-json-1.1")
		if code != 200 {
			w.Header().Set("X-Amzn-ErrorType", resp.(map[string]string)["__type"])
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return f, srv.URL
}

func staticProv(id string) aws.CredentialsProvider {
	return credentials.NewStaticCredentialsProvider(id, "fakeSecret", "")
}

func TestBatchGetRepositoriesParses(t *testing.T) {
	_, url := newFakeCC(t, func(op string, body map[string]any) (int, any) {
		return 200, map[string]any{
			"repositories": []map[string]any{{
				"accountId": "777788889999", "repositoryId": "id-1", "repositoryName": "found_repo",
				"repositoryDescription": "d", "defaultBranch": "main", "cloneUrlHttp": "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/found_repo",
				"lastModifiedDate": 1.7e9,
			}},
			"repositoriesNotFound": []string{"missing_repo"},
			"errors":               []map[string]any{{"errorCode": "EncryptionKeyAccessDeniedException", "repositoryName": "kms_repo", "errorMessage": "x"}},
		}
	})
	res, err := NewFactory(url)(staticProv("AKIAIOSFODNN7EXAMPLE"), "us-east-1").BatchGetRepositories(context.Background(), []string{"found_repo", "missing_repo", "kms_repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Found) != 1 || res.Found[0].AccountID != "777788889999" || res.Found[0].RepositoryID != "id-1" || res.Found[0].DefaultBranch != "main" || res.Found[0].LastModified.IsZero() {
		t.Errorf("found: %+v", res.Found)
	}
	if len(res.NotFound) != 1 || res.NotFound[0] != "missing_repo" || len(res.KMSErrors) != 1 || res.KMSErrors[0] != "kms_repo" {
		t.Errorf("notFound %v kms %v", res.NotFound, res.KMSErrors)
	}
}

func TestListRepositoriesPaginates(t *testing.T) {
	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("repo_%02d", i)
	}
	f, url := newFakeCC(t, func(op string, body map[string]any) (int, any) {
		switch op {
		case "ListRepositories":
			if body["nextToken"] == nil {
				var page []map[string]string
				for _, n := range names[:20] {
					page = append(page, map[string]string{"repositoryName": n, "repositoryId": "id-" + n})
				}
				return 200, map[string]any{"repositories": page, "nextToken": "t2"}
			}
			var page []map[string]string
			for _, n := range names[20:] {
				page = append(page, map[string]string{"repositoryName": n, "repositoryId": "id-" + n})
			}
			return 200, map[string]any{"repositories": page}
		case "BatchGetRepositories":
			var repos []map[string]string
			for _, n := range body["repositoryNames"].([]any) {
				repos = append(repos, map[string]string{"repositoryName": n.(string), "repositoryId": "id-" + n.(string), "accountId": "111122223333"})
			}
			return 200, map[string]any{"repositories": repos}
		}
		return 400, map[string]string{"__type": "UnknownOperationException"}
	})
	metas, err := NewFactory(url)(staticProv("AKIAIOSFODNN7EXAMPLE"), "us-east-1").ListRepositories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 30 || metas[0].Name != "repo_00" || metas[29].Name != "repo_29" || metas[5].AccountID != "111122223333" {
		t.Errorf("metas: %d %+v", len(metas), metas[0])
	}
	counts := map[string]int{}
	for _, o := range f.targets {
		counts[o]++
	}
	if counts["ListRepositories"] != 2 || counts["BatchGetRepositories"] != 2 {
		t.Errorf("calls: %v (30 names = 2 pages + 2 batches of 25)", counts)
	}
}

type fakeAPI struct {
	account string
	repos   map[string]string // name -> repository id owned by this account
	kms     map[string]bool
	fail    error
	calls   *[]int
}

func (f *fakeAPI) BatchGetRepositories(ctx context.Context, names []string) (BatchResult, error) {
	if f.calls != nil {
		*f.calls = append(*f.calls, len(names))
	}
	if f.fail != nil {
		return BatchResult{}, f.fail
	}
	res := BatchResult{Found: []RepoMeta{}, NotFound: []string{}, KMSErrors: []string{}}
	for _, n := range names {
		switch {
		case f.kms[n]:
			res.KMSErrors = append(res.KMSErrors, n)
		case f.repos[n] != "":
			res.Found = append(res.Found, RepoMeta{Name: n, AccountID: f.account, RepositoryID: f.repos[n]})
		default:
			res.NotFound = append(res.NotFound, n)
		}
	}
	return res, nil
}
func (f *fakeAPI) ListRepositories(ctx context.Context) ([]RepoMeta, error) { return nil, nil }

func factoryFor(apis map[string]*fakeAPI, regions *[]string) Factory {
	return func(p aws.CredentialsProvider, region string) API {
		cr, _ := p.Retrieve(context.Background())
		if regions != nil {
			*regions = append(*regions, region)
		}
		return apis[cr.AccessKeyID]
	}
}

func acct(id, portal string, usable bool) Account {
	return Account{AccountID: id, Portal: portal, Profiles: []string{"p-" + id}, Preferred: "p-" + id, Provider: staticProv(id), Usable: usable}
}

func TestDetectSingleAmbiguousPartial(t *testing.T) {
	apis := map[string]*fakeAPI{
		"A": {account: "111111111111", repos: map[string]string{"only_a": "ida", "both": "ida2"}},
		"B": {account: "222222222222", repos: map[string]string{"both": "idb2"}, kms: map[string]bool{"kms_b": true}},
		"D": {account: "444444444444", fail: errors.New("boom")},
	}
	accts := []Account{acct("A", "pa", true), acct("B", "pb", true), acct("C", "pc", false), acct("D", "pd", true)}
	qs := []Query{
		{"k1", "us-east-1", "only_a"}, {"k2", "us-east-1", "both"}, {"k3", "us-east-1", "nowhere"}, {"k4", "us-east-1", "kms_b"},
	}
	res := Detect(context.Background(), qs, accts, factoryFor(apis, nil))
	if r := res["k1"]; len(r.Candidates) != 1 || r.Candidates[0].AccountID != "111111111111" || r.Candidates[0].RepositoryID != "ida" || r.Checked != 2 || r.Skipped != 2 {
		t.Errorf("single: %+v", r)
	}
	if r := res["k1"]; strings.Join(r.SkippedPortals, ",") != "pc,pd" {
		t.Errorf("skipped portals: %v", r.SkippedPortals)
	}
	if r := res["k2"]; len(r.Candidates) != 2 {
		t.Errorf("ambiguous: %+v", r)
	}
	if r := res["k3"]; len(r.Candidates) != 0 || r.Skipped != 2 {
		t.Errorf("partial: %+v", r)
	}
	if r := res["k4"]; len(r.Candidates) != 1 || !r.Candidates[0].KMSWarning || r.Candidates[0].AccountID != "B" {
		t.Errorf("kms: %+v", r)
	}
}

func TestDetectAmbiguousAcrossAccounts(t *testing.T) {
	// Two local clones (two query keys) of acme_cs_portal; two accounts own a repository of that name.
	apis := map[string]*fakeAPI{
		"A": {account: "111111111111", repos: map[string]string{"acme_cs_portal": "ida"}},
		"B": {account: "222222222222", repos: map[string]string{"acme_cs_portal": "idb"}},
	}
	qs := []Query{{"clone1", "us-east-1", "acme_cs_portal"}, {"clone2", "us-east-1", "acme_cs_portal"}}
	res := Detect(context.Background(), qs, []Account{acct("A", "", true), acct("B", "", true)}, factoryFor(apis, nil))
	for _, k := range []string{"clone1", "clone2"} {
		r := res[k]
		if len(r.Candidates) != 2 || r.Candidates[0].AccountID != "111111111111" || r.Candidates[1].AccountID != "222222222222" {
			t.Errorf("%s: %+v", k, r)
		}
	}
}

func TestDetectChunksOf100(t *testing.T) {
	repos := map[string]string{}
	var qs []Query
	for i := 0; i < 150; i++ {
		n := fmt.Sprintf("repo_%03d", i)
		repos[n] = "id" + n
		qs = append(qs, Query{Key: n, Region: "us-east-1", Name: n})
	}
	var calls []int
	apis := map[string]*fakeAPI{"A": {account: "111111111111", repos: repos, calls: &calls}}
	res := Detect(context.Background(), qs, []Account{acct("A", "", true)}, factoryFor(apis, nil))
	if len(calls) != 2 || calls[0] != 100 || calls[1] != 50 {
		t.Errorf("calls: %v", calls)
	}
	if len(res) != 150 || len(res["repo_149"].Candidates) != 1 {
		t.Errorf("results: %d", len(res))
	}
}

func TestDetectUsesURLRegion(t *testing.T) {
	// The account's profile region is eu-west-1; the repository URL says us-east-1.
	var regions []string
	apis := map[string]*fakeAPI{"A": {account: "111111111111", repos: map[string]string{"r": "id"}}}
	a := acct("A", "", true)
	a.Profiles = []string{"globex(region=eu-west-1)"}
	res := Detect(context.Background(), []Query{{"k", "us-east-1", "r"}, {"k2", "us-east-2", "r"}}, []Account{a}, factoryFor(apis, &regions))
	if len(regions) != 2 || regions[0] != "us-east-1" || regions[1] != "us-east-2" {
		t.Errorf("factory regions: %v", regions)
	}
	if len(res["k"].Candidates) != 1 {
		t.Errorf("%+v", res["k"])
	}
}

func TestDetectRealClientSignsWithURLRegion(t *testing.T) {
	f, url := newFakeCC(t, func(op string, body map[string]any) (int, any) {
		return 200, map[string]any{"repositories": []map[string]any{{"accountId": "777788889999", "repositoryId": "rid", "repositoryName": "r"}}}
	})
	res := Detect(context.Background(), []Query{{"k", "us-east-1", "r"}}, []Account{acct("AKIAIOSFODNN7EXAMPLE", "", true)}, NewFactory(url))
	if len(res["k"].Candidates) != 1 || res["k"].Candidates[0].AccountID != "777788889999" {
		t.Errorf("%+v", res["k"])
	}
	if len(f.regions) != 1 || f.regions[0] != "us-east-1" {
		t.Errorf("request signed for %v", f.regions)
	}
}

func TestErrorClassification(t *testing.T) {
	_, url := newFakeCC(t, func(op string, body map[string]any) (int, any) {
		return 400, map[string]string{"__type": "AccessDeniedException", "message": "nope"}
	})
	_, err := NewFactory(url)(staticProv("AKIAIOSFODNN7EXAMPLE"), "us-east-1").BatchGetRepositories(context.Background(), []string{"r"})
	if err == nil || !IsAccessDenied(err) || IsNotSignedIn(err) {
		t.Errorf("access denied: %v", err)
	}
	_, url2 := newFakeCC(t, func(op string, body map[string]any) (int, any) {
		return 400, map[string]string{"__type": "ExpiredTokenException", "message": "expired"}
	})
	_, err = NewFactory(url2)(staticProv("AKIAIOSFODNN7EXAMPLE"), "us-east-1").BatchGetRepositories(context.Background(), []string{"r"})
	if err == nil || !IsNotSignedIn(err) || IsAccessDenied(err) {
		t.Errorf("expired: %v", err)
	}
	// credential provider failures surface the provider's AppError
	failing := aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		return aws.Credentials{}, api.NewError(api.ErrNotSignedIn, "not signed in").WithParam("portal", "initech")
	})
	_, err = NewFactory(url)(failing, "us-east-1").BatchGetRepositories(context.Background(), []string{"r"})
	if err == nil || !IsNotSignedIn(err) {
		t.Errorf("provider failure: %v", err)
	}
	if IsAccessDenied(nil) || IsNotSignedIn(nil) {
		t.Error("nil must not match")
	}
}
