package transfer

import (
	"strings"

	"github.com/rudian/aws_sso_hub/internal/api"
	"github.com/rudian/aws_sso_hub/internal/awsconfig"
	"github.com/rudian/aws_sso_hub/internal/ini"
)

func parseIni(b []byte) (*ini.Doc, error) {
	d, err := ini.Parse(b)
	if err != nil {
		return nil, api.NewError(api.ErrConfigParse, "config parse error")
	}
	return d, nil
}

func sessionKVs(s awsconfig.Session) []ini.KV {
	kvs := []ini.KV{{Key: "sso_start_url", Value: s.StartURL}, {Key: "sso_region", Value: s.Region}}
	if len(s.Scopes) > 0 {
		kvs = append(kvs, ini.KV{Key: "sso_registration_scopes", Value: strings.Join(s.Scopes, ",")})
	}
	return kvs
}

// addSession appends a new [sso-session] section.
func addSession(s awsconfig.Session) edit {
	return func(d *ini.Doc) {
		if !d.Has(awsconfig.SessionSection(s.Name)) {
			d.AddSection(awsconfig.SessionSection(s.Name), sessionKVs(s))
		}
	}
}

// replaceSession rewrites the three portal keys of an existing section and leaves every other line alone.
func replaceSession(s awsconfig.Session) edit {
	return func(d *ini.Doc) {
		d.Set(awsconfig.SessionSection(s.Name), sessionKVs(s), ini.InsertBottom)
	}
}

// addProfile appends a new SSO profile with the same keys, in the same order, as creating one by hand.
func addProfile(e profileEntry, name, portal, portalRegion string) edit {
	return func(d *ini.Doc) {
		sec := awsconfig.ConfigSection(name)
		if d.Has(sec) {
			return
		}
		region := e.Region
		if region == "" {
			region = portalRegion
		}
		output := e.Output
		if output == "" {
			output = "json"
		}
		d.AddSection(sec, []ini.KV{
			{Key: "sso_session", Value: portal},
			{Key: "sso_account_id", Value: e.AccountID},
			{Key: "sso_role_name", Value: e.Role},
			{Key: "region", Value: region},
			{Key: "output", Value: output},
		})
	}
}

// replaceProfile points an existing SSO profile at the entry's portal, account and role. region and output
// change only when the entry carries them; other keys stay.
func replaceProfile(e profileEntry, portal string) edit {
	return func(d *ini.Doc) {
		sec := awsconfig.ConfigSection(e.Name)
		d.Set(sec, []ini.KV{{Key: "sso_session", Value: portal}, {Key: "sso_account_id", Value: e.AccountID}, {Key: "sso_role_name", Value: e.Role}}, ini.InsertTop)
		var rest []ini.KV
		if e.Region != "" {
			rest = append(rest, ini.KV{Key: "region", Value: e.Region})
		}
		if e.Output != "" {
			rest = append(rest, ini.KV{Key: "output", Value: e.Output})
		}
		if len(rest) > 0 {
			d.Set(sec, rest, ini.InsertBottom)
		}
	}
}
