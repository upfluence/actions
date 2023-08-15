package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/upfluence/errors"
	"github.com/upfluence/log"
	"github.com/upfluence/log/record"

	"github.com/upfluence/actions/pkg/executil"
	"github.com/upfluence/actions/pkg/toolkit"
)

var defaultConfig = config{
	DockerfilePaths: []string{"Dockerfile"},
	OS:              "linux",
	Arch:            "amd64",
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
		return nil
	}
}

type config struct {
	Version string `flag:"release-version"`

	DockerfilePaths []string `flag:"dockerfile-paths"`
	Registries      []string `flag:"registries"`

	ArgMode        argMode           `flag:"arg-mode"`
	AdditionalArgs map[string]string `flag:"additional-args"`

	OS   string `flag:"os"`
	Arch string `flag:"arch"`

	TagMode        tagMode  `flag:"tag-mode"`
	AdditionalTags []string `flag:"additional-tags"`

	SkipPush bool `flag:"skip-push"`
}

func (c *config) platform() string {
	return fmt.Sprintf("%s/%s", c.OS, c.Arch)
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
					name:       name,
					dockerfile: fname,
					platform:   platform,
					commit:     cctx.Sha[:7],
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
	commit     string

	registries []string
	tags       []string
}

func (b build) intermediateTag() string {
	return fmt.Sprintf("%s:%s", b.name, b.commit)
}

func (b build) buildArgs() []string {
	vs := []string{
		"build",
		"--pull",
		"--file",
		b.dockerfile,
		"--tag",
		b.intermediateTag(),
		"--platform",
		b.platform,
	}

	for k, v := range b.args {
		vs = append(vs, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}

	return append(vs, ".")
}

func (b build) tagArgs() [][]string {
	var as [][]string

	for _, r := range b.registries {
		for _, t := range b.tags {
			as = append(
				as,
				[]string{
					"tag",
					b.intermediateTag(),
					fmt.Sprintf("%s/%s:%s", r, b.name, t),
				},
			)
		}
	}

	return as
}

func (b build) pushArgs() [][]string {
	var as [][]string

	for _, r := range b.registries {
		for _, t := range b.tags {
			as = append(
				as,
				[]string{
					"push",
					fmt.Sprintf("%s/%s:%s", r, b.name, t),
				},
			)
		}
	}

	return as
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

				for _, args := range b.tagArgs() {
					if err := exec(args); err != nil {
						return err
					}
				}

				if !c.SkipPush {
					for _, args := range b.pushArgs() {
						if err := exec(args); err != nil {
							return err
						}
					}
				}
			}

			return nil
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
