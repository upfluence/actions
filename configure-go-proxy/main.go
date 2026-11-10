package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/upfluence/errors"

	"github.com/upfluence/actions/pkg/toolkit"
)

var (
	errMissingHost           = errors.New("go proxy host is required")
	errMissingUser           = errors.New("go proxy user is required")
	errMissingPrivateModules = errors.New("private modules are required")
)

type config struct {
	Host           string `env:"GOPROXY_HOST"    flag:"host"`
	User           string `env:"GOPROXY_USER"    flag:"user"`
	PrivateModules string `env:"PRIVATE_MODULES" flag:"private-modules"`
	Audience       string `env:"AUDIENCE"        flag:"audience"`

	SkipGolangProxy bool `env:"SKIP_GOLANG_PROXY" flag:"skip-golang-proxy"`
}

func (c *config) validate() error {
	switch {
	case c.Host == "":
		return errMissingHost
	case c.User == "":
		return errMissingUser
	case c.PrivateModules == "":
		return errMissingPrivateModules
	}

	return nil
}

func (c *config) environ(ctx context.Context, otf toolkit.OIDCTokenFetcher) (toolkit.Environment, error) {
	if err := c.validate(); err != nil {
		return toolkit.Environment{}, err
	}

	t, err := otf.Fetch(ctx, c.Audience)

	if err != nil {
		return toolkit.Environment{}, errors.Wrap(err, "fetch github oidc token")
	}

	urls := []string{fmt.Sprintf("https://%s:%s@%s", c.User, t, c.Host)}

	if !c.SkipGolangProxy {
		urls = append(urls, "https://proxy.golang.org")
	}

	urls = append(urls, "direct")

	return toolkit.Environment{
		Variables: []toolkit.Variable{
			{Key: "GOPRIVATE", Value: c.PrivateModules},
			{Key: "GONOPROXY", Value: "none"},
			{Key: "GOPROXY", Value: strings.Join(urls, ",")},
		},
		Secrets: []string{t},
	}, nil
}

func main() {
	toolkit.NewApp(
		"configure-go-proxy",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			env, err := c.environ(ctx, cctx.OIDCToken)

			if err != nil {
				return err
			}

			return env.Export(cctx.Mask, cctx.Env)
		},
	).Run(context.Background())
}
