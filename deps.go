//go:build tools

package main

import (
	_ "github.com/aws/aws-sdk-go-v2/aws"
	_ "github.com/aws/aws-sdk-go-v2/config"
	_ "github.com/aws/aws-sdk-go-v2/credentials"
	_ "github.com/aws/aws-sdk-go-v2/credentials/ssocreds"
	_ "github.com/aws/aws-sdk-go-v2/service/codecommit"
	_ "github.com/aws/aws-sdk-go-v2/service/iam"
	_ "github.com/aws/aws-sdk-go-v2/service/sso"
	_ "github.com/aws/aws-sdk-go-v2/service/ssooidc"
	_ "github.com/aws/aws-sdk-go-v2/service/sts"
	_ "github.com/aws/smithy-go"
	_ "github.com/wailsapp/wails/v3/pkg/application"
	_ "github.com/wailsapp/wails/v3/pkg/events"
)
