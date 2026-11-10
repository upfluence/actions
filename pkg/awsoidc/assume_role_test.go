package awsoidc

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/upfluence/errors"
	"github.com/upfluence/pkg/testutil"

	"github.com/upfluence/actions/pkg/toolkit"
)

const roleARN = "arn:aws:iam::123:role/foo"

var errSTS = errors.New("sts failure")

type oidcTokenFetcher struct {
	token string
	err   error

	audience string
}

func (otf *oidcTokenFetcher) Fetch(_ context.Context, audience string) (string, error) {
	otf.audience = audience

	return otf.token, otf.err
}

type fakeSTSClient struct {
	out *sts.AssumeRoleWithWebIdentityOutput
	err error

	in *sts.AssumeRoleWithWebIdentityInput
}

func (c *fakeSTSClient) AssumeRoleWithWebIdentity(_ context.Context, in *sts.AssumeRoleWithWebIdentityInput, _ ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error) {
	c.in = in

	return c.out, c.err
}

func TestConfigAssumeRole(t *testing.T) {
	var (
		expiration = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

		output = sts.AssumeRoleWithWebIdentityOutput{
			Credentials: &types.Credentials{
				AccessKeyId:     aws.String("key-id"),
				SecretAccessKey: aws.String("secret"),
				SessionToken:    aws.String("session"),
				Expiration:      aws.Time(expiration),
			},
		}

		defaultInput = sts.AssumeRoleWithWebIdentityInput{
			RoleArn:          aws.String(roleARN),
			RoleSessionName:  aws.String("session-name"),
			WebIdentityToken: aws.String("tok"),
		}

		config = Config{RoleARN: roleARN, Audience: DefaultConfig.Audience}
	)

	for _, tt := range []struct {
		name         string
		haveConfig   Config
		haveFetchErr error
		haveOutput   *sts.AssumeRoleWithWebIdentityOutput
		haveSTSErr   error

		want         aws.Credentials
		wantInput    *sts.AssumeRoleWithWebIdentityInput
		wantAudience string
		assertErr    testutil.ErrorAssertion
	}{
		{
			name:       "success",
			haveConfig: config,
			haveOutput: &output,
			want: aws.Credentials{
				AccessKeyID:     "key-id",
				SecretAccessKey: "secret",
				SessionToken:    "session",
				CanExpire:       true,
				Expires:         expiration,
			},
			wantInput:    &defaultInput,
			wantAudience: DefaultConfig.Audience,
			assertErr:    testutil.NoError(),
		},
		{
			name: "custom audience and duration",
			haveConfig: Config{
				RoleARN:  roleARN,
				Audience: "custom",
				Duration: 2 * time.Hour,
			},
			haveOutput: &output,
			want: aws.Credentials{
				AccessKeyID:     "key-id",
				SecretAccessKey: "secret",
				SessionToken:    "session",
				CanExpire:       true,
				Expires:         expiration,
			},
			wantInput: &sts.AssumeRoleWithWebIdentityInput{
				RoleArn:          aws.String(roleARN),
				RoleSessionName:  aws.String("session-name"),
				WebIdentityToken: aws.String("tok"),
				DurationSeconds:  aws.Int32(7200),
			},
			wantAudience: "custom",
			assertErr:    testutil.NoError(),
		},
		{
			name:       "missing role arn",
			haveConfig: DefaultConfig,
			assertErr:  testutil.ErrorCause(ErrMissingRoleARN),
		},
		{
			name:         "oidc token unavailable",
			haveConfig:   config,
			haveFetchErr: toolkit.ErrOIDCTokenUnavailable,
			wantAudience: DefaultConfig.Audience,
			assertErr:    testutil.ErrorCause(toolkit.ErrOIDCTokenUnavailable),
		},
		{
			name:         "sts failure",
			haveConfig:   config,
			haveSTSErr:   errSTS,
			wantInput:    &defaultInput,
			wantAudience: DefaultConfig.Audience,
			assertErr:    testutil.ErrorCause(errSTS),
		},
		{
			name:         "no credentials",
			haveConfig:   config,
			haveOutput:   &sts.AssumeRoleWithWebIdentityOutput{},
			wantInput:    &defaultInput,
			wantAudience: DefaultConfig.Audience,
			assertErr:    testutil.ErrorCause(errNoCredentials),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var (
				otf = oidcTokenFetcher{token: "tok", err: tt.haveFetchErr}
				cl  = fakeSTSClient{out: tt.haveOutput, err: tt.haveSTSErr}
			)

			got, err := tt.haveConfig.AssumeRole(
				t.Context(),
				&cl,
				&otf,
				"session-name",
			)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantInput, cl.in)
			assert.Equal(t, tt.wantAudience, otf.audience)
		})
	}
}

func TestSessionName(t *testing.T) {
	n := SessionName()

	assert.Regexp(t, `^github-[\w.-]+$`, n)
	assert.LessOrEqual(t, len(n), maxSessionNameLength)
}
