package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/upfluence/errors"
	"github.com/upfluence/log"
	"github.com/upfluence/log/record"

	"github.com/upfluence/actions/pkg/executil"
	"github.com/upfluence/actions/pkg/toolkit"
)

var defaultConfig = config{
	DockerfilePaths: []string{"Dockerfile"},
	OS:              "linux",
	Archs:           []string{"amd64"},
	Registries:      []string{"index.docker.io"},
}

const (
	none = iota
	app
)

type argMode int

func (am *argMode) Parse(v string) error {
	switch v {
	case "app":
		*am = app
	case "none":
		*am = none
	default:
		return fmt.Errorf("Invalid arg-mode %q", v)
	}

	return nil
}

func (am argMode) args(cctx toolkit.CommandContext, v string) map[string]string {
	switch am {
	case app:
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

type tagMode int

func (tm *tagMode) Parse(v string) error {
	switch v {
	case "app":
		*tm = app
	case "none":
		*tm = none
	default:
		return fmt.Errorf("Invalid tag-mode %q", v)
	}

	return nil
}

func (tm tagMode) tags(cctx toolkit.CommandContext, v string) []string {
	switch tm {
	case app:
		upstream := cctx.RefName

		if upstream == "master" || upstream == "main" {
			upstream = "latest"
		}

		return []string{v, upstream, cctx.Sha[:7]}
	default:
		return []string{cctx.Sha[:7]}
	}
}

type config struct {
	Version string `flag:"release-version"`

	DockerfilePaths []string `flag:"dockerfile-paths"`
	Registries      []string `flag:"registries"`

	ArgMode        argMode           `flag:"arg-mode"`
	AdditionalArgs map[string]string `flag:"additional-args"`

	OS    string   `flag:"os"`
	Archs []string `flag:"archs"`

	TagMode        tagMode  `flag:"tag-mode"`
	AdditionalTags []string `flag:"additional-tags"`

	SkipPush    bool `flag:"skip-push"`
	UseGHACache bool `flag:"gha-cache"`

	OverrideRepositories map[string]string `flag:"override-repositories"`
}

func (c *config) repository(n string) string {
	if r, ok := c.OverrideRepositories[n]; ok {
		return r
	}

	return n
}

func (c *config) platform() string {
	var ps []string

	for _, arch := range c.Archs {
		ps = append(ps, fmt.Sprintf("%s/%s", c.OS, arch))
	}

	return strings.Join(ps, ",")
}

func (c *config) tags(cctx toolkit.CommandContext) []string {
	return append(
		c.TagMode.tags(cctx, c.Version),
		c.AdditionalTags...,
	)
}

func (c *config) args(cctx toolkit.CommandContext) map[string]string {
	vs := c.ArgMode.args(cctx, c.Version)

	for k, v := range c.AdditionalArgs {
		vs[k] = v
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

func (c *config) builds(cctx toolkit.CommandContext) ([]build, error) {
	var (
		bs []build

		tags     = c.tags(cctx)
		platform = c.platform()
		args     = c.args(cctx)
	)

	for _, p := range c.DockerfilePaths {
		fnames, err := filepath.Glob(filepath.Join(".", p))

		if err != nil {
			return nil, errors.Wrapf(err, "invalid glob %q", p)
		}

		for _, fname := range fnames {
			name := cctx.Repository

			if p != "Dockerfile" {
				name, _ = cctx.SplittedRepository()
				name += "/" + filepath.Base(filepath.Dir(fname))
			}

			bs = append(
				bs,
				build{
					name:       c.repository(name),
					dockerfile: fname,
					platform:   platform,
					skipPush:   c.SkipPush,
					ghaCache:   c.UseGHACache,
					registries: c.Registries,
					tags:       tags,
					args:       args,
				},
			)
		}
	}

	return bs, nil
}

type build struct {
	name       string
	dockerfile string
	args       map[string]string
	platform   string
	skipPush   bool
	ghaCache   bool

	registries []string
	tags       []string
}

func (b build) buildArgs() []string {
	vs := []string{
		"buildx",
		"build",
		"--pull",
		"--file",
		b.dockerfile,
		"--platform",
		b.platform,
	}

	if !b.skipPush {
		vs = append(vs, "--push")
	}

	if b.ghaCache {
		vs = append(vs, "--cache-from", "type=gha", "--cache-to", "type=gha,mode=max")
	}

	for _, r := range b.registries {
		for _, t := range b.tags {
			vs = append(vs, "--tag", fmt.Sprintf("%s/%s:%s", r, b.name, t))
		}
	}

	for k, v := range b.args {
		vs = append(vs, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}

	return append(vs, ".")
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

			exec := func(args []string) error {
				return errors.Wrap(
					exc.Exec(
						ctx,
						executil.Command{
							Cmd:    "docker",
							Args:   args,
							Stdout: cctx.CommandContext.Stdout,
							Stderr: cctx.CommandContext.Stderr,
						},
					),
					"cant exec docker command",
				)
			}

			for _, b := range bs {
				if err := exec(b.buildArgs()); err != nil {
					return err
				}
			}

			return nil
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
