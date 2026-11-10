package awsoidc

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/upfluence/errors"

	"github.com/upfluence/actions/pkg/toolkit"
)

const maxSessionNameLength = 64

var (
	ErrMissingRoleARN = errors.New("role arn is required")

	errNoCredentials = errors.New("sts returned no credentials")
)

// Config assumes an AWS role by trading the GitHub Actions OIDC token, it is
// meant to be embedded in the config of the actions needing AWS credentials.
type Config struct {
	RoleARN  string        `env:"ROLE_ARN" flag:"role-arn"`
	Audience string        `env:"AUDIENCE" flag:"audience"`
	Duration time.Duration `env:"DURATION" flag:"duration"`
}

var DefaultConfig = Config{Audience: "sts.amazonaws.com"}

type STSClient interface {
	AssumeRoleWithWebIdentity(context.Context, *sts.AssumeRoleWithWebIdentityInput, ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error)
}

func (c Config) AssumeRole(ctx context.Context, cl STSClient, otf toolkit.OIDCTokenFetcher, sessionName string) (aws.Credentials, error) {
	if c.RoleARN == "" {
		return aws.Credentials{}, ErrMissingRoleARN
	}

	t, err := otf.Fetch(ctx, c.Audience)

	if err != nil {
		return aws.Credentials{}, errors.Wrap(err, "fetch github oidc token")
	}

	in := sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(c.RoleARN),
		RoleSessionName:  aws.String(sessionName),
		WebIdentityToken: aws.String(t),
	}

	if c.Duration > 0 {
		in.DurationSeconds = aws.Int32(int32(c.Duration.Seconds()))
	}

	out, err := cl.AssumeRoleWithWebIdentity(ctx, &in)

	if err != nil {
		return aws.Credentials{}, errors.Wrap(err, "assume role with web identity")
	}

	if out.Credentials == nil {
		return aws.Credentials{}, errNoCredentials
	}

	return aws.Credentials{
		AccessKeyID:     aws.ToString(out.Credentials.AccessKeyId),
		SecretAccessKey: aws.ToString(out.Credentials.SecretAccessKey),
		SessionToken:    aws.ToString(out.Credentials.SessionToken),
		CanExpire:       out.Credentials.Expiration != nil,
		Expires:         aws.ToTime(out.Credentials.Expiration),
	}, nil
}

// SessionName builds a role session name identifying the runner.
func SessionName() string {
	parts := []string{"github"}

	if h, _ := os.Hostname(); h != "" {
		parts = append(parts, h)
	}

	n := strings.Join(
		append(parts, strconv.FormatInt(time.Now().Unix(), 10)),
		"-",
	)

	if len(n) > maxSessionNameLength {
		return n[:maxSessionNameLength]
	}

	return n
}

// StaticCredentials provides already retrieved credentials to AWS clients.
func StaticCredentials(creds aws.Credentials) aws.CredentialsProvider {
	return aws.CredentialsProviderFunc(
		func(context.Context) (aws.Credentials, error) { return creds, nil },
	)
}
