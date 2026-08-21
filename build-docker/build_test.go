package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/upfluence/pkg/testutil"

	dockerconfig "github.com/upfluence/actions/pkg/docker"
	"github.com/upfluence/actions/pkg/toolkit"
)

func TestConfigBuildArgs(t *testing.T) {
	for _, tt := range []struct {
		name       string
		haveConfig config
		want       []string
	}{
		{
			name: "none tag and arg modes with tag push",
			haveConfig: config{
				Config: dockerconfig.Config{
					TagMode:    dockerconfig.None,
					Registries: []string{"registry.example.com"},
				},
				OS:       "linux",
				Archs:    []string{"amd64"},
				PushMode: pushTags,
			},
			want: []string{
				"buildx", "build", "--pull", "--file", "Dockerfile",
				"--platform", "linux/amd64", "--metadata-file", "metadata.json",
				"--tag", "registry.example.com/upfluence/example:0123456",
				"--output", "type=registry", ".",
			},
		},
		{
			name: "app tag and arg modes with tag push",
			haveConfig: config{
				Config: dockerconfig.Config{
					Version:    "v1.2.3",
					TagMode:    dockerconfig.App,
					Registries: []string{"registry.example.com"},
				},
				OS:       "linux",
				Archs:    []string{"amd64", "arm64"},
				ArgMode:  argApp,
				PushMode: pushTags,
			},
			want: []string{
				"buildx", "build", "--pull", "--file", "Dockerfile",
				"--platform", "linux/amd64,linux/arm64", "--metadata-file", "metadata.json",
				"--tag", "registry.example.com/upfluence/example:v1.2.3",
				"--tag", "registry.example.com/upfluence/example:latest",
				"--tag", "registry.example.com/upfluence/example:0123456",
				"--output", "type=registry",
				"--build-arg", "GITHUB_TOKEN=token",
				"--build-arg", "GIT_BRANCH=main",
				"--build-arg", "GIT_COMMIT=0123456789",
				"--build-arg", "GIT_REMOTE=https://github.com/upfluence/example",
				"--build-arg", "SEMVER_VERSION=v1.2.3",
				".",
			},
		},
		{
			name: "additional tags and args with cache",
			haveConfig: config{
				Config: dockerconfig.Config{
					TagMode:        dockerconfig.None,
					AdditionalTags: []string{"stable", "canary"},
					Registries:     []string{"registry.example.com"},
				},
				OS:    "linux",
				Archs: []string{"amd64"},
				AdditionalArgs: map[string]string{
					"FOO": "bar",
					"ZED": "last",
				},
				PushMode:    pushTags,
				UseGHACache: true,
			},
			want: []string{
				"buildx", "build", "--pull", "--file", "Dockerfile",
				"--platform", "linux/amd64", "--metadata-file", "metadata.json",
				"--cache-from", "type=gha", "--cache-to", "type=gha,mode=max",
				"--tag", "registry.example.com/upfluence/example:0123456",
				"--tag", "registry.example.com/upfluence/example:stable",
				"--tag", "registry.example.com/upfluence/example:canary",
				"--output", "type=registry",
				"--build-arg", "FOO=bar", "--build-arg", "ZED=last", ".",
			},
		},
		{
			name: "digest push to multiple registries",
			haveConfig: config{
				Config: dockerconfig.Config{
					TagMode:    dockerconfig.App,
					Registries: []string{"registry.example.com", "backup.example.com"},
				},
				OS:       "linux",
				Archs:    []string{"amd64"},
				PushMode: pushDigest,
			},
			want: []string{
				"buildx", "build", "--pull", "--file", "Dockerfile",
				"--platform", "linux/amd64", "--metadata-file", "metadata.json",
				"--output", "type=image,name=registry.example.com/upfluence/example,push=true,push-by-digest=true,name-canonical=true",
				"--output", "type=image,name=backup.example.com/upfluence/example,push=true,push-by-digest=true,name-canonical=true",
				".",
			},
		},
		{
			name: "tag push to multiple registries",
			haveConfig: config{
				Config: dockerconfig.Config{
					TagMode:    dockerconfig.None,
					Registries: []string{"registry.example.com", "backup.example.com"},
				},
				OS:       "linux",
				Archs:    []string{"amd64"},
				PushMode: pushTags,
			},
			want: []string{
				"buildx", "build", "--pull", "--file", "Dockerfile",
				"--platform", "linux/amd64", "--metadata-file", "metadata.json",
				"--tag", "registry.example.com/upfluence/example:0123456",
				"--tag", "backup.example.com/upfluence/example:0123456",
				"--output", "type=registry",
				".",
			},
		},
		{
			name: "no push",
			haveConfig: config{
				OS:       "linux",
				Archs:    []string{"amd64"},
				PushMode: pushNone,
			},
			want: []string{
				"buildx", "build", "--pull", "--file", "Dockerfile",
				"--platform", "linux/amd64", "--metadata-file", "metadata.json",
				".",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.haveConfig.build(
				toolkit.CommandContext{
					Repository: "upfluence/example",
					Sha:        "0123456789",
					RefName:    "main",
					Token:      "token",
				},
				"Dockerfile",
				"Dockerfile",
			)

			assert.Equal(t, tt.want, b.buildArgs("metadata.json"))
		})
	}
}

func TestPlatformDigests(t *testing.T) {
	for _, tt := range []struct {
		name         string
		haveMetadata string
		haveManifest string
		havePlatform string
		want         map[string]string
		assertErr    testutil.ErrorAssertion
	}{
		{
			name: "single platform manifest",
			haveMetadata: `{
				"containerimage.descriptor": {
					"digest": "sha256:amd64",
					"mediaType": "application/vnd.oci.image.manifest.v1+json"
				}
			}`,
			havePlatform: "linux/amd64",
			want: map[string]string{
				"linux/amd64": "sha256:amd64",
			},
			assertErr: testutil.NoError(),
		},
		{
			name: "multi-platform index ignores attestation",
			haveMetadata: `{
				"containerimage.descriptor": {
					"digest": "sha256:index",
					"mediaType": "application/vnd.oci.image.index.v1+json"
				}
			}`,
			haveManifest: `{
				"manifests": [
					{"digest":"sha256:amd64","platform":{"architecture":"amd64","os":"linux"}},
					{"digest":"sha256:arm","platform":{"architecture":"arm","os":"linux","variant":"v7"}},
					{"digest":"sha256:attestation","platform":{"architecture":"unknown","os":"unknown"}}
				]
			}`,
			havePlatform: "linux/amd64,linux/arm",
			want: map[string]string{
				"linux/amd64":  "sha256:amd64",
				"linux/arm/v7": "sha256:arm",
			},
			assertErr: testutil.NoError(),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := platformDigests(
				[]byte(tt.haveMetadata),
				[]byte(tt.haveManifest),
				tt.havePlatform,
			)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
