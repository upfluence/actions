package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/upfluence/errors"

	"github.com/upfluence/actions/pkg/awsoidc"
	"github.com/upfluence/actions/pkg/toolkit"
)

var (
	errMissingAppID          = errors.New("app id is required")
	errMissingInstallationID = errors.New("app installation id is required")
	errMissingKMSKeyARN      = errors.New("kms key arn is required")

	defaultConfig = config{
		Config:      awsoidc.DefaultConfig,
		Permissions: map[string]string{"contents": "read"},
		EnvNames:    []string{"GH_TOKEN"},
		APIURL:      "https://api.github.com",
	}
)

type config struct {
	awsoidc.Config

	AppID          string `env:"APP_ID"              flag:"app-id"`
	InstallationID int64  `env:"APP_INSTALLATION_ID" flag:"app-installation-id"`
	KMSKeyARN      string `env:"KMS_KEY_ARN"         flag:"kms-key-arn"`

	Repositories []string          `env:"REPOSITORIES" flag:"repositories"`
	Permissions  map[string]string `env:"PERMISSIONS"  flag:"permissions"`

	EnvNames []string `env:"ENV_NAMES"      flag:"env-names"`
	APIURL   string   `env:"GITHUB_API_URL" flag:"api-url"`
}

// region validates the config and returns the region of the KMS key, used
// for both STS and KMS.
func (c *config) region() (string, error) {
	switch {
	case c.AppID == "":
		return "", errMissingAppID
	case c.InstallationID == 0:
		return "", errMissingInstallationID
	case c.KMSKeyARN == "":
		return "", errMissingKMSKeyARN
	}

	a, err := arn.Parse(c.KMSKeyARN)

	if err != nil {
		return "", errors.Wrap(err, "parse kms key arn")
	}

	return a.Region, nil
}

func (c *config) installationToken(ctx context.Context, owner string, kcl kmsClient, hcl *http.Client, now time.Time) (installationToken, error) {
	rs, err := normalizeRepositories(owner, c.Repositories)

	if err != nil {
		return installationToken{}, err
	}

	ps, err := normalizePermissions(c.Permissions)

	if err != nil {
		return installationToken{}, err
	}

	jwt, err := signAppJWT(ctx, kcl, c.KMSKeyARN, c.AppID, now)

	if err != nil {
		return installationToken{}, err
	}

	return createInstallationToken(
		ctx,
		hcl,
		c.APIURL,
		c.InstallationID,
		jwt,
		installationTokenRequest{Repositories: rs, Permissions: ps},
	)
}

func (c *config) environment(t installationToken) toolkit.Environment {
	env := toolkit.Environment{Secrets: []string{t.Token}}

	for _, n := range c.EnvNames {
		env.Variables = append(env.Variables, toolkit.Variable{Key: n, Value: t.Token})
	}

	return env
}

func main() {
	toolkit.NewApp(
		"configure-github",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			region, err := c.region()

			if err != nil {
				return err
			}

			creds, err := c.AssumeRole(
				ctx,
				sts.New(sts.Options{Region: region}),
				cctx.OIDCToken,
				awsoidc.SessionName(),
			)

			if err != nil {
				return errors.WithStack(err)
			}

			owner, _, _ := strings.Cut(cctx.Repository, "/")

			t, err := c.installationToken(
				ctx,
				owner,
				kms.New(
					kms.Options{
						Region:      region,
						Credentials: awsoidc.StaticCredentials(creds),
					},
				),
				http.DefaultClient,
				time.Now(),
			)

			if err != nil {
				return err
			}

			if err := c.environment(t).Export(cctx.Mask, cctx.Env); err != nil {
				return errors.WithStack(err)
			}

			if err := cctx.Output.WriteKeyValue("token", t.Token); err != nil {
				return errors.Wrap(err, "write token output")
			}

			return errors.Wrap(
				cctx.Output.WriteKeyValue(
					"expires-at",
					t.ExpiresAt.Format(time.RFC3339),
				),
				"write expires-at output",
			)
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
