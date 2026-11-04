// Package codecommit wraps the few CodeCommit API calls the app needs: finding which AWS account owns a
// repository and listing repositories for the clone wizard. Regions always come from the remote URL.
package codecommit

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cc "github.com/aws/aws-sdk-go-v2/service/codecommit"
	"github.com/aws/smithy-go"

	"github.com/rudian/aws_sso_hub/internal/api"
)

// RepoMeta is the subset of repository metadata the app shows.
type RepoMeta struct {
	Name, AccountID, RepositoryID, Description, DefaultBranch, CloneURLHTTP string
	LastModified                                                            time.Time
}

// BatchResult is the outcome of one BatchGetRepositories call.
type BatchResult struct {
	Found     []RepoMeta
	NotFound  []string
	KMSErrors []string // repositories that exist but whose KMS key is not accessible
}

// API is what the engine needs from CodeCommit.
type API interface {
	BatchGetRepositories(ctx context.Context, names []string) (BatchResult, error) // at most 100 names
	ListRepositories(ctx context.Context) ([]RepoMeta, error)                      // all pages with metadata (BatchGet in chunks of 25)
}

// Factory builds an API client for credentials and a region.
type Factory func(p aws.CredentialsProvider, region string) API

// NewFactory returns the production factory. endpoint "" means AWS; tests and sandbox mode pass a local URL.
func NewFactory(endpoint string) Factory {
	return func(p aws.CredentialsProvider, region string) API {
		opts := cc.Options{
			Region:           region,
			Credentials:      p,
			RetryMaxAttempts: 2,
			AppID:            "aws-sso-hub",
		}
		if endpoint != "" {
			opts.BaseEndpoint = aws.String(endpoint)
		}
		return &client{c: cc.New(opts)}
	}
}

type client struct{ c *cc.Client }

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (c *client) BatchGetRepositories(ctx context.Context, names []string) (BatchResult, error) {
	res := BatchResult{Found: []RepoMeta{}, NotFound: []string{}, KMSErrors: []string{}}
	if len(names) == 0 {
		return res, nil
	}
	out, err := c.c.BatchGetRepositories(ctx, &cc.BatchGetRepositoriesInput{RepositoryNames: names})
	if err != nil {
		return res, err
	}
	for _, r := range out.Repositories {
		m := RepoMeta{
			Name: str(r.RepositoryName), AccountID: str(r.AccountId), RepositoryID: str(r.RepositoryId),
			Description: str(r.RepositoryDescription), DefaultBranch: str(r.DefaultBranch), CloneURLHTTP: str(r.CloneUrlHttp),
		}
		if r.LastModifiedDate != nil {
			m.LastModified = r.LastModifiedDate.UTC()
		}
		res.Found = append(res.Found, m)
	}
	res.NotFound = append(res.NotFound, out.RepositoriesNotFound...)
	for _, e := range out.Errors {
		if strings.HasPrefix(string(e.ErrorCode), "Encryption") {
			res.KMSErrors = append(res.KMSErrors, str(e.RepositoryName))
		}
	}
	return res, nil
}

func (c *client) ListRepositories(ctx context.Context) ([]RepoMeta, error) {
	var names []string
	var token *string
	for {
		out, err := c.c.ListRepositories(ctx, &cc.ListRepositoriesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, r := range out.Repositories {
			names = append(names, str(r.RepositoryName))
		}
		if out.NextToken == nil || *out.NextToken == "" {
			break
		}
		token = out.NextToken
	}
	metas := make([]RepoMeta, 0, len(names))
	for i := 0; i < len(names); i += 25 {
		end := i + 25
		if end > len(names) {
			end = len(names)
		}
		res, err := c.BatchGetRepositories(ctx, names[i:end])
		if err != nil {
			return nil, err
		}
		metas = append(metas, res.Found...)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return metas, nil
}

// Query asks which accounts own a repository (Region comes from the remote URL).
type Query struct{ Key, Region, Name string }

// Account is one AWS account the app can query, with one representative credentials provider.
type Account struct {
	AccountID, Portal string
	Profiles          []string
	Preferred         string
	Provider          aws.CredentialsProvider
	Usable            bool
}

// Candidate is an account that owns (or, with KMSWarning, has) a repository of that name.
type Candidate struct {
	AccountID, RepositoryID string
	KMSWarning              bool
}

// Result is the detection outcome for one query.
type Result struct {
	Candidates       []Candidate
	Checked, Skipped int
	SkippedPortals   []string
}

const batchSize = 100

// Detect looks the queried repositories up in every usable account, one BatchGetRepositories call per
// (account, region, 100 names). Accounts that are not usable, signed out, denied or failing are skipped
// (never an error). The map key is Query.Key.
func Detect(ctx context.Context, qs []Query, accounts []Account, f Factory) map[string]Result {
	type slot struct {
		cands   map[string]Candidate
		checked int
		skipped int
		portals map[string]bool
	}
	slots := map[string]*slot{}
	for _, q := range qs {
		slots[q.Key] = &slot{cands: map[string]Candidate{}, portals: map[string]bool{}}
	}
	type group struct {
		region string
		names  []string
		keys   map[string][]string // name -> query keys
	}
	var groups []*group
	byRegion := map[string]*group{}
	for _, q := range qs {
		g := byRegion[q.Region]
		if g == nil {
			g = &group{region: q.Region, keys: map[string][]string{}}
			byRegion[q.Region] = g
			groups = append(groups, g)
		}
		if _, ok := g.keys[q.Name]; !ok {
			g.names = append(g.names, q.Name)
		}
		g.keys[q.Name] = append(g.keys[q.Name], q.Key)
	}
	skip := func(a Account, g *group, names []string) {
		for _, n := range names {
			for _, k := range g.keys[n] {
				s := slots[k]
				s.skipped++
				if a.Portal != "" {
					s.portals[a.Portal] = true
				}
			}
		}
	}
	for _, a := range accounts {
		for _, g := range groups {
			if ctx.Err() != nil {
				skip(a, g, g.names)
				continue
			}
			if !a.Usable || a.Provider == nil {
				skip(a, g, g.names)
				continue
			}
			client := f(a.Provider, g.region)
			for i := 0; i < len(g.names); i += batchSize {
				end := i + batchSize
				if end > len(g.names) {
					end = len(g.names)
				}
				chunk := g.names[i:end]
				res, err := client.BatchGetRepositories(ctx, chunk)
				if err != nil {
					skip(a, g, chunk)
					continue
				}
				add := func(name string, c Candidate) {
					if c.AccountID == "" {
						c.AccountID = a.AccountID
					}
					for _, k := range g.keys[name] {
						s := slots[k]
						if old, dup := s.cands[c.AccountID]; dup {
							c.KMSWarning = c.KMSWarning && old.KMSWarning
							if c.RepositoryID == "" {
								c.RepositoryID = old.RepositoryID
							}
						}
						s.cands[c.AccountID] = c
					}
				}
				for _, m := range res.Found {
					add(m.Name, Candidate{AccountID: m.AccountID, RepositoryID: m.RepositoryID})
				}
				for _, n := range res.KMSErrors {
					add(n, Candidate{AccountID: a.AccountID, KMSWarning: true})
				}
				for _, n := range chunk {
					for _, k := range g.keys[n] {
						slots[k].checked++
					}
				}
			}
		}
	}
	out := make(map[string]Result, len(slots))
	for k, s := range slots {
		r := Result{Candidates: make([]Candidate, 0, len(s.cands)), Checked: s.checked, Skipped: s.skipped, SkippedPortals: make([]string, 0, len(s.portals))}
		for _, c := range s.cands {
			r.Candidates = append(r.Candidates, c)
		}
		sort.Slice(r.Candidates, func(i, j int) bool { return r.Candidates[i].AccountID < r.Candidates[j].AccountID })
		for p := range s.portals {
			r.SkippedPortals = append(r.SkippedPortals, p)
		}
		sort.Strings(r.SkippedPortals)
		out[k] = r
	}
	return out
}

func apiCode(err error) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode()
	}
	return ""
}

// IsAccessDenied reports an IAM permission failure.
func IsAccessDenied(err error) bool {
	switch apiCode(err) {
	case "AccessDeniedException", "AccessDenied", "UnauthorizedOperation":
		return true
	}
	var ae *api.AppError
	return errors.As(err, &ae) && ae.Code == api.ErrAWSAccessDenied
}

// IsNotSignedIn reports missing, expired or rejected credentials.
func IsNotSignedIn(err error) bool {
	switch apiCode(err) {
	case "ExpiredTokenException", "ExpiredToken", "UnrecognizedClientException", "InvalidSignatureException",
		"InvalidClientTokenId", "UnauthorizedException", "RequestExpired":
		return true
	}
	var ae *api.AppError
	if errors.As(err, &ae) {
		return ae.Code == api.ErrNotSignedIn || ae.Code == api.ErrSessionExpired
	}
	return false
}
