package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/stretchr/testify/assert"
	"github.com/upfluence/errors"
	"github.com/upfluence/pkg/testutil"

	"github.com/upfluence/actions/pkg/awsoidc"
	"github.com/upfluence/actions/pkg/toolkit"
)

const region = "eu-west-1"

var errSTS = errors.New("sts failure")

type oidcTokenFetcher struct{}

func (oidcTokenFetcher) Fetch(context.Context, string) (string, error) {
	return "tok", nil
}

type fakeSTSClient struct {
	err error
}

func (c fakeSTSClient) AssumeRoleWithWebIdentity(context.Context, *sts.AssumeRoleWithWebIdentityInput, ...func(*sts.Options)) (*sts.AssumeRoleWithWebIdentityOutput, error) {
	if c.err != nil {
		return nil, c.err
	}

	return &sts.AssumeRoleWithWebIdentityOutput{
		Credentials: &types.Credentials{
			AccessKeyId:     aws.String("key-id"),
			SecretAccessKey: aws.String("secret"),
			SessionToken:    aws.String("session"),
		},
	}, nil
}

func TestConfigEnviron(t *testing.T) {
	for _, tt := range []struct {
		name       string
		haveConfig config
		haveSTSErr error

		want      toolkit.Environment
		assertErr testutil.ErrorAssertion
	}{
		{
			name: "success",
			haveConfig: config{
				Config: awsoidc.Config{RoleARN: "arn:aws:iam::123:role/foo"},
				Region: region,
			},
			want: toolkit.Environment{
				Variables: []toolkit.Variable{
					{Key: "AWS_ACCESS_KEY_ID", Value: "key-id"},
					{Key: "AWS_SECRET_ACCESS_KEY", Value: "secret"},
					{Key: "AWS_SESSION_TOKEN", Value: "session"},
					{Key: "AWS_DEFAULT_REGION", Value: region},
					{Key: "AWS_REGION", Value: region},
				},
				Secrets: []string{"key-id", "secret", "session"},
			},
			assertErr: testutil.NoError(),
		},
		{
			name:       "missing role arn",
			haveConfig: defaultConfig,
			assertErr:  testutil.ErrorCause(awsoidc.ErrMissingRoleARN),
		},
		{
			name: "sts failure",
			haveConfig: config{
				Config: awsoidc.Config{RoleARN: "arn:aws:iam::123:role/foo"},
			},
			haveSTSErr: errSTS,
			assertErr:  testutil.ErrorCause(errSTS),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.haveConfig.environ(
				t.Context(),
				fakeSTSClient{err: tt.haveSTSErr},
				oidcTokenFetcher{},
				"session-name",
			)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
