package gitx

import (
	"testing"

	"github.com/rudian/aws_sso_hub/internal/api"
)

func TestParseRemote(t *testing.T) {
	cases := []struct {
		name, raw string
		want      RemoteInfo
		ok        bool
	}{
		{"https regional", "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/web-app",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit.us-east-1.amazonaws.com", Region: "us-east-1", Repo: "web-app",
				HTTPSURL: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/web-app"}, true},
		{"https userinfo", "https://someone-at-123456789012@git-codecommit.us-west-2.amazonaws.com/v1/repos/infra-terraform",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit.us-west-2.amazonaws.com", Region: "us-west-2", Repo: "infra-terraform",
				User: "someone-at-123456789012", HTTPSURL: "https://git-codecommit.us-west-2.amazonaws.com/v1/repos/infra-terraform"}, true},
		{"https userinfo with password drops password", "https://u:secretpw@git-codecommit.us-east-2.amazonaws.com/v1/repos/x",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit.us-east-2.amazonaws.com", Region: "us-east-2", Repo: "x", User: "u",
				HTTPSURL: "https://git-codecommit.us-east-2.amazonaws.com/v1/repos/x"}, true},
		{"fips", "https://git-codecommit-fips.us-east-2.amazonaws.com/v1/repos/MyDemoRepo",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit-fips.us-east-2.amazonaws.com", Region: "us-east-2", Repo: "MyDemoRepo", FIPS: true,
				HTTPSURL: "https://git-codecommit-fips.us-east-2.amazonaws.com/v1/repos/MyDemoRepo"}, true},
		{"china", "https://git-codecommit.cn-north-1.amazonaws.com.cn/v1/repos/r1",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit.cn-north-1.amazonaws.com.cn", Region: "cn-north-1", Repo: "r1", China: true,
				HTTPSURL: "https://git-codecommit.cn-north-1.amazonaws.com.cn/v1/repos/r1"}, true},
		{"vpce", "https://vpce-0b7d2939f4example-abcdefgh.git-codecommit.us-east-2.vpce.amazonaws.com/v1/repos/MyDemoRepo",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "vpce-0b7d2939f4example-abcdefgh.git-codecommit.us-east-2.vpce.amazonaws.com", Region: "us-east-2", Repo: "MyDemoRepo", VPCE: true,
				HTTPSURL: "https://vpce-0b7d2939f4example-abcdefgh.git-codecommit.us-east-2.vpce.amazonaws.com/v1/repos/MyDemoRepo"}, true},
		{"port 443 gov", "https://git-codecommit.us-gov-west-1.amazonaws.com:443/v1/repos/x.y-z",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit.us-gov-west-1.amazonaws.com", Region: "us-gov-west-1", Repo: "x.y-z",
				HTTPSURL: "https://git-codecommit.us-gov-west-1.amazonaws.com/v1/repos/x.y-z"}, true},
		{"uppercase host", "https://GIT-CODECOMMIT.US-EAST-1.AMAZONAWS.COM/v1/repos/Repo",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit.us-east-1.amazonaws.com", Region: "us-east-1", Repo: "Repo",
				HTTPSURL: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/Repo"}, true},
		{"trailing slash", "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/a/",
			RemoteInfo{Kind: api.RemoteHTTPS, Host: "git-codecommit.us-east-1.amazonaws.com", Region: "us-east-1", Repo: "a",
				HTTPSURL: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/a"}, true},
		{"github", "https://github.com/org/repo.git", RemoteInfo{}, false},
		{"ssh url", "ssh://git-codecommit.us-east-1.amazonaws.com/v1/repos/design-system",
			RemoteInfo{Kind: api.RemoteSSH, Host: "git-codecommit.us-east-1.amazonaws.com", Region: "us-east-1", Repo: "design-system",
				HTTPSURL: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/design-system"}, true},
		{"ssh url with key id", "ssh://APKAEXAMPLE@git-codecommit.us-east-1.amazonaws.com/v1/repos/design-system",
			RemoteInfo{Kind: api.RemoteSSH, Host: "git-codecommit.us-east-1.amazonaws.com", Region: "us-east-1", Repo: "design-system", User: "APKAEXAMPLE",
				HTTPSURL: "https://git-codecommit.us-east-1.amazonaws.com/v1/repos/design-system"}, true},
		{"ssh scp-like", "APKAEIBAERJR2EXAMPLE@git-codecommit.us-east-2.amazonaws.com:v1/repos/MyDemoRepo",
			RemoteInfo{Kind: api.RemoteSSH, Host: "git-codecommit.us-east-2.amazonaws.com", Region: "us-east-2", Repo: "MyDemoRepo", User: "APKAEIBAERJR2EXAMPLE",
				HTTPSURL: "https://git-codecommit.us-east-2.amazonaws.com/v1/repos/MyDemoRepo"}, true},
		{"grc region+profile", "codecommit::us-east-1://acme@web-app",
			RemoteInfo{Kind: api.RemoteGRC, Region: "us-east-1", Profile: "acme", Repo: "web-app"}, true},
		{"grc profile only", "codecommit://globex@data-pipeline",
			RemoteInfo{Kind: api.RemoteGRC, Profile: "globex", Repo: "data-pipeline"}, true},
		{"grc bare", "codecommit://analytics-dashboard", RemoteInfo{Kind: api.RemoteGRC, Repo: "analytics-dashboard"}, true},
		{"local path", "/srv/git/x.git", RemoteInfo{}, false},
		{"file url", "file:///tmp/x.git", RemoteInfo{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseRemote(c.raw)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, c.ok, got)
			}
			if got != c.want {
				t.Errorf("got  %+v\nwant %+v", got, c.want)
			}
		})
	}
}
