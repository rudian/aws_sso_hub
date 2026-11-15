// Package awsapi holds the small set of AWS API calls the app makes with explicit credentials:
// STS identification, IAM access-key deactivation, and the credential providers used for them.
package awsapi

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/smithy-go"

	"github.com/rudian/codepass/internal/api"
	"github.com/rudian/codepass/internal/awsconfig"
	"github.com/rudian/codepass/internal/sso"
)

// Endpoints override the AWS defaults (sandbox mode, tests). STS and IAM always use region us-east-1.
type Endpoints struct{ STS, IAM string }

const globalRegion = "us-east-1"

// StaticProvider wraps a static key. The key stays in memory.
func StaticProvider(k awsconfig.StaticKey) aws.CredentialsProvider {
	return credentials.NewStaticCredentialsProvider(k.AccessKeyID, k.SecretAccessKey, k.SessionToken)
}

// SSOProvider returns role credentials from the SSO portal for (account, role); it never consults static keys.
func SSOProvider(m *sso.Manager, s sso.Session, accountID, role string) aws.CredentialsProvider {
	return aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		return m.RoleCredentials(ctx, s, accountID, role)
	}))
}

// CallerIdentity is the result of sts:GetCallerIdentity.
type CallerIdentity struct {
	AccountID, Arn, UserID, IAMUser string
	IsRoot, IsAssumedRole           bool
}

func stsClient(p aws.CredentialsProvider, ep Endpoints) *sts.Client {
	return sts.NewFromConfig(aws.Config{Region: globalRegion, Credentials: p}, func(o *sts.Options) {
		if ep.STS != "" {
			o.BaseEndpoint = aws.String(ep.STS)
		}
	})
}

func iamClient(p aws.CredentialsProvider, ep Endpoints) *iam.Client {
	return iam.NewFromConfig(aws.Config{Region: globalRegion, Credentials: p}, func(o *iam.Options) {
		if ep.IAM != "" {
			o.BaseEndpoint = aws.String(ep.IAM)
		}
	})
}

// GetCallerIdentity calls STS with exactly the given provider.
func GetCallerIdentity(ctx context.Context, p aws.CredentialsProvider, ep Endpoints) (CallerIdentity, error) {
	out, err := stsClient(p, ep).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return CallerIdentity{}, err
	}
	id := CallerIdentity{AccountID: aws.ToString(out.Account), Arn: aws.ToString(out.Arn), UserID: aws.ToString(out.UserId)}
	acct, user, root, assumed := ParseARN(id.Arn)
	if id.AccountID == "" {
		id.AccountID = acct
	}
	id.IAMUser, id.IsRoot, id.IsAssumedRole = user, root, assumed
	return id, nil
}

// ParseARN extracts the account id, the IAM user name (last path segment of user/...), and whether the ARN
// is a root or assumed-role principal.
func ParseARN(arn string) (accountID, iamUser string, isRoot, isAssumedRole bool) {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) < 6 || parts[0] != "arn" {
		return "", "", false, false
	}
	accountID = parts[4]
	res := parts[5]
	switch {
	case res == "root":
		isRoot = true
	case strings.HasPrefix(res, "user/"):
		segs := strings.Split(res, "/")
		iamUser = segs[len(segs)-1]
	case strings.HasPrefix(res, "assumed-role/"):
		isAssumedRole = true
	}
	return accountID, iamUser, isRoot, isAssumedRole
}

// ErrorCode returns the AWS error code of err ("" if err carries none).
func ErrorCode(err error) string {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		return ae.ErrorCode()
	}
	return ""
}

func isAccessDenied(code string) bool {
	return code == "AccessDenied" || code == "AccessDeniedException" || code == "UnauthorizedOperation"
}

func isInvalidKey(code string) bool {
	switch code {
	case "InvalidClientTokenId", "SignatureDoesNotMatch", "ExpiredToken", "UnrecognizedClientException":
		return true
	}
	return false
}

// IsInvalidKeyCode reports whether an AWS error code means the access key itself was rejected.
func IsInvalidKeyCode(code string) bool { return isInvalidKey(code) }

func operationName(err error) string {
	var oe *smithy.OperationError
	if errors.As(err, &oe) {
		return strings.ToLower(oe.Service()) + ":" + oe.Operation()
	}
	return ""
}

// ToAppError maps AWS and network errors to contract errors.
func ToAppError(err error) *api.AppError {
	if err == nil {
		return nil
	}
	var ae *api.AppError
	if errors.As(err, &ae) {
		return ae
	}
	if errors.Is(err, context.Canceled) {
		return api.NewError(api.ErrCancelled, "cancelled")
	}
	code := ErrorCode(err)
	switch {
	case isAccessDenied(code):
		return api.NewError(api.ErrAWSAccessDenied, "access denied").WithDetail(operationName(err))
	case isInvalidKey(code):
		return api.NewError(api.ErrAWSInvalidKey, "AWS rejected the access key").WithDetail(code)
	case code != "":
		return api.NewError(api.ErrAWSAPI, "AWS returned an error").WithDetail(code)
	}
	if host, ok := sso.NetworkHost(err); ok {
		return api.NewError(api.ErrNetwork, "network error").WithDetail(host)
	}
	return api.AsAppError(err)
}

// DeactivateWithRole sets the access key Inactive using the given (SSO role) credentials. Access denied is
// returned as an *api.AppError with code E_AWS_ACCESS_DENIED so the caller can fall back.
func DeactivateWithRole(ctx context.Context, p aws.CredentialsProvider, userName, accessKeyID string, ep Endpoints) (api.DeactivationStatus, error) {
	cli := iamClient(p, ep)
	var marker *string
	for {
		out, err := cli.ListAccessKeys(ctx, &iam.ListAccessKeysInput{UserName: aws.String(userName), Marker: marker})
		if err != nil {
			return "", ToAppError(err)
		}
		for _, k := range out.AccessKeyMetadata {
			if aws.ToString(k.AccessKeyId) != accessKeyID {
				continue
			}
			if k.Status == iamtypes.StatusTypeInactive {
				return api.DeactivationAlready, nil
			}
			if _, err := cli.UpdateAccessKey(ctx, &iam.UpdateAccessKeyInput{
				UserName: aws.String(userName), AccessKeyId: aws.String(accessKeyID), Status: iamtypes.StatusTypeInactive}); err != nil {
				return "", ToAppError(err)
			}
			return api.DeactivationDone, nil
		}
		if !out.IsTruncated || out.Marker == nil {
			return api.DeactivationKeyNotFound, nil
		}
		marker = out.Marker
	}
}

// DeactivateSelf sets the key Inactive signed with the key itself (UserName omitted = the caller).
func DeactivateSelf(ctx context.Context, oldKey awsconfig.StaticKey, ep Endpoints) (api.DeactivationStatus, error) {
	cli := iamClient(StaticProvider(oldKey), ep)
	if _, err := cli.UpdateAccessKey(ctx, &iam.UpdateAccessKeyInput{
		AccessKeyId: aws.String(oldKey.AccessKeyID), Status: iamtypes.StatusTypeInactive}); err != nil {
		return "", ToAppError(err)
	}
	return api.DeactivationDone, nil
}

// IAMUserDestination is the console deep link for a user's security credentials.
func IAMUserDestination(userName string) string {
	return "https://console.aws.amazon.com/iam/home#/users/details/" + url.PathEscape(userName) + "?section=security_credentials"
}
