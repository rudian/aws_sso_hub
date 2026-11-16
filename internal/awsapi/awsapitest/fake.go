// Package awsapitest provides a fake AWS query-protocol server (STS + IAM on one URL) for tests. The same URL
// serves the console sign-in federation endpoint at /federation.
package awsapitest

import (
	"encoding/json"
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

// SignIn is one recorded call of the federation endpoint.
type SignIn struct {
	Method, ContentType, RawQuery       string
	Form                                url.Values // the request body
	SessionID, SessionKey, SessionToken string     // decoded from the Session JSON
}

// Fake serves STS, IAM and the federation endpoint. Configure the exported maps before use; read Requests
// and SignIns after.
type Fake struct {
	URL string

	mu         sync.Mutex
	Identities map[string]Identity // signing access key id -> identity; missing -> InvalidClientTokenId
	Keys       map[string][]Key    // IAM user name -> keys
	Deny       map[string]bool     // "<signing akid>|<Action>" or "*|<Action>" -> AccessDenied
	reqs       []Request
	signIns    []SignIn
	signInFail int
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

// SignIns returns the recorded federation calls.
func (f *Fake) SignIns() []SignIn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]SignIn(nil), f.signIns...)
}

// FailSignIn makes the federation endpoint answer with this HTTP status and an HTML page, the way AWS reports
// failures. 0 restores normal answers.
func (f *Fake) FailSignIn(status int) {
	f.mu.Lock()
	f.signInFail = status
	f.mu.Unlock()
}

// serveFederation answers a POST Action=getSigninToken with {"SigninToken":"SIGNIN-<sessionId>"}.
func (f *Fake) serveFederation(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var s struct {
		ID    string `json:"sessionId"`
		Key   string `json:"sessionKey"`
		Token string `json:"sessionToken"`
	}
	_ = json.Unmarshal([]byte(r.PostForm.Get("Session")), &s)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signIns = append(f.signIns, SignIn{Method: r.Method, ContentType: r.Header.Get("Content-Type"), RawQuery: r.URL.RawQuery,
		Form: r.PostForm, SessionID: s.ID, SessionKey: s.Key, SessionToken: s.Token})
	status := f.signInFail
	if status == 0 && (r.Method != http.MethodPost || r.PostForm.Get("Action") != "getSigninToken" || s.ID == "") {
		status = http.StatusBadRequest
	}
	if status != 0 {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(status)
		fmt.Fprint(w, `<html><body><h1>Bad Request</h1></body></html>`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"SigninToken": "SIGNIN-" + s.ID})
}

func (f *Fake) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/federation" {
		f.serveFederation(w, r)
		return
	}
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
