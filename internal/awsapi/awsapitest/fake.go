// Package awsapitest provides a fake AWS query-protocol server (STS + IAM on one URL) for tests.
package awsapitest

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"sync"
	"testing"
)

// Request is one recorded call.
type Request struct {
	Action        string
	AccessKeyID   string // from the SigV4 Credential= scope
	Authorization string
	Form          url.Values
}

// Identity is what GetCallerIdentity returns for a signing access key ID.
type Identity struct{ Account, Arn, UserID string }

// Key is an IAM access key owned by a user.
type Key struct{ ID, Status string }

// Fake serves STS and IAM. Configure the exported maps before use; read Requests after.
type Fake struct {
	URL string

	mu         sync.Mutex
	Identities map[string]Identity // signing access key id -> identity; missing -> InvalidClientTokenId
	Keys       map[string][]Key    // IAM user name -> keys
	Deny       map[string]bool     // "<signing akid>|<Action>" or "*|<Action>" -> AccessDenied
	reqs       []Request
}

var credRE = regexp.MustCompile(`Credential=([A-Z0-9]+)/`)

func New(t *testing.T) *Fake {
	f := &Fake{Identities: map[string]Identity{}, Keys: map[string][]Key{}, Deny: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.reqs...)
}

func xmlErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<ErrorResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/"><Error><Type>Sender</Type><Code>%s</Code><Message>%s</Message></Error><RequestId>req-1</RequestId></ErrorResponse>`, code, msg)
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	auth := r.Header.Get("Authorization")
	akid := ""
	if m := credRE.FindStringSubmatch(auth); m != nil {
		akid = m[1]
	}
	action := r.Form.Get("Action")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, Request{Action: action, AccessKeyID: akid, Authorization: auth, Form: r.Form})
	id, known := f.Identities[akid]
	if !known {
		xmlErr(w, 403, "InvalidClientTokenId", "The security token included in the request is invalid.")
		return
	}
	if f.Deny[akid+"|"+action] || f.Deny["*|"+action] {
		xmlErr(w, 403, "AccessDenied", "User: "+id.Arn+" is not authorized to perform: iam:"+action)
		return
	}
	w.Header().Set("Content-Type", "text/xml")
	switch action {
	case "GetCallerIdentity":
		fmt.Fprintf(w, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/"><GetCallerIdentityResult><Arn>%s</Arn><UserId>%s</UserId><Account>%s</Account></GetCallerIdentityResult><ResponseMetadata><RequestId>req-2</RequestId></ResponseMetadata></GetCallerIdentityResponse>`, id.Arn, id.UserID, id.Account)
	case "ListAccessKeys":
		user := r.Form.Get("UserName")
		fmt.Fprintf(w, `<ListAccessKeysResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/"><ListAccessKeysResult><UserName>%s</UserName><AccessKeyMetadata>`, user)
		for _, k := range f.Keys[user] {
			fmt.Fprintf(w, `<member><UserName>%s</UserName><AccessKeyId>%s</AccessKeyId><Status>%s</Status><CreateDate>2026-01-01T00:00:00Z</CreateDate></member>`, user, k.ID, k.Status)
		}
		fmt.Fprint(w, `</AccessKeyMetadata><IsTruncated>false</IsTruncated></ListAccessKeysResult><ResponseMetadata><RequestId>req-3</RequestId></ResponseMetadata></ListAccessKeysResponse>`)
	case "UpdateAccessKey":
		user := r.Form.Get("UserName")
		if user == "" { // the caller's own user: derive from its identity ARN (".../user/<name>")
			if i := lastSlash(id.Arn); i >= 0 {
				user = id.Arn[i+1:]
			}
		}
		for i, k := range f.Keys[user] {
			if k.ID == r.Form.Get("AccessKeyId") {
				f.Keys[user][i].Status = r.Form.Get("Status")
			}
		}
		fmt.Fprint(w, `<UpdateAccessKeyResponse xmlns="https://iam.amazonaws.com/doc/2010-05-08/"><ResponseMetadata><RequestId>req-4</RequestId></ResponseMetadata></UpdateAccessKeyResponse>`)
	default:
		xmlErr(w, 400, "InvalidAction", "unknown action "+action)
	}
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

// KeyStatus returns the current status of a key ("" if unknown).
func (f *Fake) KeyStatus(user, id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, k := range f.Keys[user] {
		if k.ID == id {
			return k.Status
		}
	}
	return ""
}
