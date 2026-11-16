package awsapi

import (
	"errors"
	"testing"
)

func TestConsoleEndpoints(t *testing.T) {
	cases := []struct {
		ssoRegion, region     string
		federation, home      string
		unsupported, override bool
	}{
		{ssoRegion: "us-east-1", region: "us-west-2",
			federation: "https://signin.aws.amazon.com/federation", home: "https://console.aws.amazon.com/console/home?region=us-west-2"},
		{ssoRegion: "eu-central-1", region: "eu-central-1",
			federation: "https://eu-central-1.signin.aws.amazon.com/federation", home: "https://console.aws.amazon.com/console/home?region=eu-central-1"},
		{ssoRegion: "ap-southeast-2", region: "",
			federation: "https://ap-southeast-2.signin.aws.amazon.com/federation", home: "https://console.aws.amazon.com/console/home"},
		// a profile region of another partition, or not a region at all, is left out
		{ssoRegion: "us-west-2", region: "cn-north-1",
			federation: "https://us-west-2.signin.aws.amazon.com/federation", home: "https://console.aws.amazon.com/console/home"},
		{ssoRegion: "us-west-2", region: "us-west-2&x=1",
			federation: "https://us-west-2.signin.aws.amazon.com/federation", home: "https://console.aws.amazon.com/console/home"},
		{ssoRegion: "us-west-2", region: "us-iso-east-1",
			federation: "https://us-west-2.signin.aws.amazon.com/federation", home: "https://console.aws.amazon.com/console/home"},
		{ssoRegion: "us-gov-west-1", region: "us-gov-east-1",
			federation: "https://signin.amazonaws-us-gov.com/federation", home: "https://console.amazonaws-us-gov.com/console/home?region=us-gov-east-1"},
		{ssoRegion: "us-gov-east-1", region: "us-east-1",
			federation: "https://us-gov-east-1.signin.amazonaws-us-gov.com/federation", home: "https://console.amazonaws-us-gov.com/console/home"},
		{ssoRegion: "cn-north-1", region: "cn-northwest-1",
			federation: "https://cn-north-1.signin.amazonaws.cn/federation", home: "https://console.amazonaws.cn/console/home?region=cn-northwest-1"},
		{ssoRegion: "cn-northwest-1", region: "us-east-1",
			federation: "https://cn-northwest-1.signin.amazonaws.cn/federation", home: "https://console.amazonaws.cn/console/home"},
		// the override replaces the sign-in host only
		{ssoRegion: "us-east-1", region: "us-east-1", override: true,
			federation: "http://127.0.0.1:9/federation", home: "https://console.aws.amazon.com/console/home?region=us-east-1"},
		{ssoRegion: "us-iso-east-1", unsupported: true},
		{ssoRegion: "us-isob-east-1", unsupported: true},
		{ssoRegion: "eu-isoe-west-1", unsupported: true},
		{ssoRegion: "us-isof-south-1", unsupported: true},
		{ssoRegion: "us-iso-west-1", override: true, unsupported: true},
		{ssoRegion: "eusc-de-east-1", unsupported: true},
		{ssoRegion: "us-east-1.evil.example", unsupported: true},
		{ssoRegion: "", unsupported: true},
	}
	for _, c := range cases {
		var ep Endpoints
		if c.override {
			ep.SignIn = "http://127.0.0.1:9/"
		}
		fed, home, err := consoleEndpoints(c.ssoRegion, c.region, ep)
		if c.unsupported {
			if !errors.Is(err, ErrConsoleUnsupported) || fed != "" || home != "" {
				t.Errorf("%s: want ErrConsoleUnsupported, got %q %q %v", c.ssoRegion, fed, home, err)
			}
			continue
		}
		if err != nil || fed != c.federation || home != c.home {
			t.Errorf("%s/%s: got %q %q %v", c.ssoRegion, c.region, fed, home, err)
		}
	}
}
