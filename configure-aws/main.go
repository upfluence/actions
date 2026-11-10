package main

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/upfluence/errors"

	"github.com/upfluence/actions/pkg/awsoidc"
	"github.com/upfluence/actions/pkg/toolkit"
)

var defaultConfig = config{
	Config: awsoidc.DefaultConfig,
	Region: "us-east-1",
}

type config struct {
	awsoidc.Config

	Region string `env:"REGION" flag:"region"`
}

func (c *config) environ(ctx context.Context, cl awsoidc.STSClient, otf toolkit.OIDCTokenFetcher, sessionName string) (toolkit.Environment, error) {
	creds, err := c.AssumeRole(ctx, cl, otf, sessionName)

	if err != nil {
		return toolkit.Environment{}, errors.WithStack(err)
	}

	return toolkit.Environment{
		Variables: []toolkit.Variable{
			{Key: "AWS_ACCESS_KEY_ID", Value: creds.AccessKeyID},
			{Key: "AWS_SECRET_ACCESS_KEY", Value: creds.SecretAccessKey},
			{Key: "AWS_SESSION_TOKEN", Value: creds.SessionToken},
			{Key: "AWS_DEFAULT_REGION", Value: c.Region},
			{Key: "AWS_REGION", Value: c.Region},
		},
		Secrets: []string{
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		},
	}, nil
}

func main() {
	toolkit.NewApp(
		"configure-aws",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			env, err := c.environ(
				ctx,
				sts.New(sts.Options{Region: c.Region}),
				cctx.OIDCToken,
				awsoidc.SessionName(),
			)

			if err != nil {
				return err
			}

			return env.Export(cctx.Mask, cctx.Env)
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
