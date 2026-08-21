package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/upfluence/errors"
	"github.com/upfluence/log"
	"github.com/upfluence/log/record"

	dockerconfig "github.com/upfluence/actions/pkg/docker"
	"github.com/upfluence/actions/pkg/executil"
	"github.com/upfluence/actions/pkg/toolkit"
)

var defaultConfig = config{
	Config:          dockerconfig.DefaultConfig,
	DockerfilePaths: []string{"Dockerfile"},
	OS:              "linux",
	Archs:           []string{"amd64"},
}

const (
	argNone argMode = iota
	argApp
)

const noneValue = "none"

const (
	pushTags pushMode = iota
	pushDigest
	pushNone
)

type argMode int

func (am *argMode) Parse(v string) error {
	switch v {
	case "app":
		*am = argApp
	case noneValue:
		*am = argNone
	default:
		return fmt.Errorf("invalid arg-mode %q", v)
	}

	return nil
}

func (am argMode) args(cctx toolkit.CommandContext, v string) map[string]string {
	switch am {
	case argApp:
		return map[string]string{
			"GIT_BRANCH":     cctx.RefName,
			"GIT_COMMIT":     cctx.Sha,
			"GIT_REMOTE":     "https://github.com/" + cctx.Repository,
			"SEMVER_VERSION": v,
			"GITHUB_TOKEN":   cctx.Token,
		}
	default:
		return make(map[string]string)
	}
}

type pushMode int

func (pm *pushMode) Parse(v string) error {
	switch v {
	case "tags":
		*pm = pushTags
	case "digest":
		*pm = pushDigest
	case noneValue:
		*pm = pushNone
	default:
		return fmt.Errorf("invalid push-mode %q", v)
	}

	return nil
}

type config struct {
	dockerconfig.Config

	DockerfilePaths []string `flag:"dockerfile-paths"`

	ArgMode        argMode           `flag:"arg-mode"`
	AdditionalArgs map[string]string `flag:"additional-args"`

	OS    string   `flag:"os"`
	Archs []string `flag:"archs"`

	PushMode pushMode `flag:"push-mode"`

	UseGHACache bool `flag:"gha-cache"`
}

func (c *config) platform() string {
	var ps []string

	for _, arch := range c.Archs {
		ps = append(ps, fmt.Sprintf("%s/%s", c.OS, arch))
	}

	return strings.Join(ps, ",")
}

func (c *config) args(cctx toolkit.CommandContext) map[string]string {
	vs := c.ArgMode.args(cctx, c.Version)

	maps.Copy(vs, c.AdditionalArgs)

	return vs
}

func (c *config) outputs(name string, tags []string) []string {
	var vs []string

	switch c.PushMode {
	case pushTags:
		for _, r := range c.Registries {
			for _, t := range tags {
				vs = append(vs, "--tag", fmt.Sprintf("%s/%s:%s", r, name, t))
			}
		}

		vs = append(vs, "--output", "type=registry")
	case pushDigest:
		for _, r := range c.Registries {
			vs = append(
				vs,
				"--output",
				fmt.Sprintf(
					"type=image,name=%s/%s,push=true,push-by-digest=true,name-canonical=true",
					r,
					name,
				),
			)
		}
	}

	return vs
}

func (c *config) executor(l log.Logger) executil.Executor {
	return executil.VerboseExecutor{
		Next:   executil.StdExecutor{PropagateEnviron: true},
		Logger: l,
		Level:  record.Debug,
	}
}

func (c *config) build(cctx toolkit.CommandContext, path, fname string) build {
	name := cctx.Repository

	if path != "Dockerfile" {
		name, _ = cctx.SplittedRepository()
		name += "/" + filepath.Base(filepath.Dir(fname))
	}

	name = c.Repository(name)

	return build{
		name:       name,
		dockerfile: fname,
		platform:   c.platform(),
		ghaCache:   c.UseGHACache,
		registries: c.Registries,
		args:       c.args(cctx),
		outputs:    c.outputs(name, c.Tags(cctx)),
	}
}

func (c *config) builds(cctx toolkit.CommandContext) ([]build, error) {
	var bs []build

	for _, p := range c.DockerfilePaths {
		fnames, err := filepath.Glob(filepath.Join(".", p))

		if err != nil {
			return nil, errors.Wrapf(err, "invalid glob %q", p)
		}

		for _, fname := range fnames {
			bs = append(bs, c.build(cctx, p, fname))
		}
	}

	return bs, nil
}

type build struct {
	name       string
	dockerfile string
	args       map[string]string
	platform   string
	ghaCache   bool
	outputs    []string

	registries []string
}

func (b build) buildArgs(metadataFile string) []string {
	vs := []string{
		"buildx",
		"build",
		"--pull",
		"--file",
		b.dockerfile,
		"--platform",
		b.platform,
		"--metadata-file",
		metadataFile,
	}

	if b.ghaCache {
		vs = append(vs, "--cache-from", "type=gha", "--cache-to", "type=gha,mode=max")
	}

	vs = append(vs, b.outputs...)

	for _, k := range slices.Sorted(maps.Keys(b.args)) {
		v := b.args[k]

		vs = append(vs, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}

	return append(vs, ".")
}

func (b build) imageReference(imageDigest string) (string, error) {
	if len(b.registries) == 0 {
		return "", errors.New("build has no image reference")
	}

	return fmt.Sprintf("%s/%s@%s", b.registries[0], b.name, imageDigest), nil
}

type buildMetadata struct {
	Descriptor struct {
		Digest    string `json:"digest"`
		MediaType string `json:"mediaType"`
	} `json:"containerimage.descriptor"`
}

type imageIndex struct {
	Manifests []struct {
		Digest   string `json:"digest"`
		Platform struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
			Variant      string `json:"variant"`
		} `json:"platform"`
	} `json:"manifests"`
}

func platformDigests(metadata, manifest []byte, platform string) (map[string]string, error) {
	md, err := decodeBuildMetadata(metadata)

	if err != nil {
		return nil, err
	}

	if !strings.Contains(md.Descriptor.MediaType, "index") &&
		!strings.Contains(md.Descriptor.MediaType, "manifest.list") {
		if strings.Contains(platform, ",") {
			return nil, errors.New("multi-platform build did not produce an image index")
		}

		return map[string]string{platform: md.Descriptor.Digest}, nil
	}

	var idx imageIndex

	if err := json.Unmarshal(manifest, &idx); err != nil {
		return nil, errors.Wrap(err, "cannot decode image index")
	}

	digests := make(map[string]string)

	for _, m := range idx.Manifests {
		if m.Platform.OS == "" || m.Platform.OS == "unknown" ||
			m.Platform.Architecture == "" || m.Platform.Architecture == "unknown" {
			continue
		}

		p := m.Platform.OS + "/" + m.Platform.Architecture

		if m.Platform.Variant != "" {
			p += "/" + m.Platform.Variant
		}

		digests[p] = m.Digest
	}

	if len(digests) == 0 {
		return nil, errors.New("image index has no platform manifests")
	}

	return digests, nil
}

func decodeBuildMetadata(metadata []byte) (buildMetadata, error) {
	var md buildMetadata

	if err := json.Unmarshal(metadata, &md); err != nil {
		return buildMetadata{}, errors.Wrap(err, "cannot decode build metadata")
	}

	if md.Descriptor.Digest == "" {
		return buildMetadata{}, errors.New("build metadata has no image digest")
	}

	return md, nil
}

func main() {
	toolkit.NewApp(
		"build-docker",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			bs, err := c.builds(cctx)

			if err != nil {
				return err
			}

			exc := c.executor(cctx.Logger)

			exec := func(args []string, stdout io.Writer) error {
				return errors.Wrap(
					exc.Exec(
						ctx,
						executil.Command{
							Cmd:    "docker",
							Args:   args,
							Stdout: stdout,
							Stderr: cctx.CommandContext.Stderr,
						},
					),
					"cant exec docker command",
				)
			}

			digests := make(map[string]string)

			for i, b := range bs {
				metadataFile, err := os.CreateTemp("", "build-docker-metadata-*.json")

				if err != nil {
					return errors.Wrap(err, "cannot create build metadata file")
				}

				metadataPath := metadataFile.Name()

				if err := metadataFile.Close(); err != nil {
					return errors.Wrap(err, "cannot close build metadata file")
				}

				defer os.Remove(metadataPath)

				if err := exec(b.buildArgs(metadataPath), cctx.CommandContext.Stdout); err != nil {
					return err
				}

				if i != 0 {
					continue
				}

				if len(b.outputs) == 0 {
					continue
				}

				metadata, err := os.ReadFile(metadataPath)

				if err != nil {
					return errors.Wrap(err, "cannot read build metadata")
				}

				md, err := decodeBuildMetadata(metadata)

				if err != nil {
					return err
				}

				ref, err := b.imageReference(md.Descriptor.Digest)

				if err != nil {
					return err
				}

				var manifest bytes.Buffer

				if err := exec(
					[]string{"buildx", "imagetools", "inspect", "--raw", ref},
					&manifest,
				); err != nil {
					return err
				}

				digests, err = platformDigests(metadata, manifest.Bytes(), b.platform)

				if err != nil {
					return err
				}
			}

			buf, err := json.Marshal(digests)

			if err != nil {
				return errors.Wrap(err, "cannot encode platform digests")
			}

			return errors.Wrap(
				cctx.Output.WriteKeyValue("digests", string(buf)),
				"cannot write platform digests output",
			)
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
