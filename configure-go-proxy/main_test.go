package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/upfluence/pkg/testutil"

	"github.com/upfluence/actions/pkg/toolkit"
)

const token = "tok"

type oidcTokenFetcher struct {
	err error

	audience string
}

func (otf *oidcTokenFetcher) Fetch(_ context.Context, audience string) (string, error) {
	otf.audience = audience

	return token, otf.err
}

func newConfig(fn func(*config)) config {
	c := config{
		Host:           "proxy.example.com/proxy",
		User:           "foo",
		PrivateModules: "github.com/example",
	}

	fn(&c)

	return c
}

func TestConfigEnviron(t *testing.T) {
	for _, tt := range []struct {
		name         string
		haveConfig   config
		haveFetchErr error

		want         toolkit.Environment
		wantAudience string
		assertErr    testutil.ErrorAssertion
	}{
		{
			name:       "with golang proxy",
			haveConfig: newConfig(func(*config) {}),
			want: toolkit.Environment{
				Variables: []toolkit.Variable{
					{Key: "GOPRIVATE", Value: "github.com/example"},
					{Key: "GONOPROXY", Value: "none"},
					{
						Key: "GOPROXY",
						Value: "https://foo:tok@proxy.example.com/proxy," +
							"https://proxy.golang.org,direct",
					},
				},
				Secrets: []string{token},
			},
			assertErr: testutil.NoError(),
		},
		{
			name: "skip golang proxy with custom audience",
			haveConfig: newConfig(func(c *config) {
				c.PrivateModules = "github.com/example,github.com/other"
				c.Audience = "custom"
				c.SkipGolangProxy = true
			}),
			want: toolkit.Environment{
				Variables: []toolkit.Variable{
					{
						Key:   "GOPRIVATE",
						Value: "github.com/example,github.com/other",
					},
					{Key: "GONOPROXY", Value: "none"},
					{
						Key:   "GOPROXY",
						Value: "https://foo:tok@proxy.example.com/proxy,direct",
					},
				},
				Secrets: []string{token},
			},
			wantAudience: "custom",
			assertErr:    testutil.NoError(),
		},
		{
			name:       "missing host",
			haveConfig: newConfig(func(c *config) { c.Host = "" }),
			assertErr:  testutil.ErrorCause(errMissingHost),
		},
		{
			name:       "missing user",
			haveConfig: newConfig(func(c *config) { c.User = "" }),
			assertErr:  testutil.ErrorCause(errMissingUser),
		},
		{
			name:       "missing private modules",
			haveConfig: newConfig(func(c *config) { c.PrivateModules = "" }),
			assertErr:  testutil.ErrorCause(errMissingPrivateModules),
		},
		{
			name:         "oidc token unavailable",
			haveConfig:   newConfig(func(*config) {}),
			haveFetchErr: toolkit.ErrOIDCTokenUnavailable,
			assertErr:    testutil.ErrorCause(toolkit.ErrOIDCTokenUnavailable),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			otf := oidcTokenFetcher{err: tt.haveFetchErr}

			got, err := tt.haveConfig.environ(t.Context(), &otf)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantAudience, otf.audience)
		})
	}
}
