package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/upfluence/pkg/testutil"

	dockerconfig "github.com/upfluence/actions/pkg/docker"
	"github.com/upfluence/actions/pkg/toolkit"
)

func TestConfigCommands(t *testing.T) {
	for _, tt := range []struct {
		name       string
		haveConfig config
		want       [][]string
		assertErr  testutil.ErrorAssertion
	}{
		{
			name: "app tags",
			haveConfig: config{
				Config: dockerconfig.Config{
					Version:    "v1.2.3",
					TagMode:    dockerconfig.App,
					Registries: []string{"index.docker.io"},
				},
				Digests: digests{
					"linux/arm64": "sha256:arm64",
					"linux/amd64": "sha256:amd64",
				},
			},
			want: [][]string{
				{
					"buildx", "imagetools", "create",
					"--tag", "index.docker.io/upfluence/example:v1.2.3",
					"--tag", "index.docker.io/upfluence/example:latest",
					"--tag", "index.docker.io/upfluence/example:0123456",
					"index.docker.io/upfluence/example@sha256:amd64",
					"index.docker.io/upfluence/example@sha256:arm64",
				},
			},
			assertErr: testutil.NoError(),
		},
		{
			name: "override additional tags and registries",
			haveConfig: config{
				Config: dockerconfig.Config{
					TagMode:        dockerconfig.None,
					AdditionalTags: []string{"stable"},
					Registries:     []string{"registry.example.com", "backup.example.com"},
					OverrideRepositories: map[string]string{
						"upfluence/example": "upfluence/renamed",
					},
				},
				Digests: digests{
					"linux/amd64": "sha256:amd64",
				},
			},
			want: [][]string{
				{
					"buildx", "imagetools", "create",
					"--tag", "registry.example.com/upfluence/renamed:0123456",
					"--tag", "registry.example.com/upfluence/renamed:stable",
					"registry.example.com/upfluence/renamed@sha256:amd64",
				},
				{
					"buildx", "imagetools", "create",
					"--tag", "backup.example.com/upfluence/renamed:0123456",
					"--tag", "backup.example.com/upfluence/renamed:stable",
					"backup.example.com/upfluence/renamed@sha256:amd64",
				},
			},
			assertErr: testutil.NoError(),
		},
		{
			name:      "empty digests",
			assertErr: testutil.ErrorEqual(errDigestsEmpty),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.haveConfig.commands(
				toolkit.CommandContext{
					Repository: "upfluence/example",
					RefName:    "main",
					Sha:        "0123456789",
				},
			)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
