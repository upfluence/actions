package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/upfluence/errors"
	"github.com/upfluence/log"
	"github.com/upfluence/log/record"

	dockerconfig "github.com/upfluence/actions/pkg/docker"
	"github.com/upfluence/actions/pkg/executil"
	"github.com/upfluence/actions/pkg/toolkit"
)

var defaultConfig = config{
	Config: dockerconfig.DefaultConfig,
}

var errDigestsEmpty = errors.New("digests cannot be empty")

type digests struct {
	values map[string]string
}

func (ds *digests) Parse(v string) error {
	return errors.Wrap(json.Unmarshal([]byte(v), &ds.values), "invalid digests")
}

type config struct {
	dockerconfig.Config

	Digests digests `flag:"digests"`
}

func (c config) commands(cctx toolkit.CommandContext) ([][]string, error) {
	if len(c.Digests.values) == 0 {
		return nil, errDigestsEmpty
	}

	name := c.Repository(cctx.Repository)
	tags := c.Tags(cctx)
	platforms := make([]string, 0, len(c.Digests.values))

	for platform := range c.Digests.values {
		platforms = append(platforms, platform)
	}

	slices.Sort(platforms)

	commands := make([][]string, 0, len(c.Registries))

	for _, registry := range c.Registries {
		args := []string{"buildx", "imagetools", "create"}

		for _, tag := range tags {
			args = append(args, "--tag", fmt.Sprintf("%s/%s:%s", registry, name, tag))
		}

		for _, platform := range platforms {
			digest := c.Digests.values[platform]

			if digest == "" {
				return nil, fmt.Errorf("digest for platform %q cannot be empty", platform)
			}

			args = append(args, fmt.Sprintf("%s/%s@%s", registry, name, digest))
		}

		commands = append(commands, args)
	}

	return commands, nil
}

func (c config) executor(l log.Logger) executil.Executor {
	return executil.VerboseExecutor{
		Next:   executil.StdExecutor{PropagateEnviron: true},
		Logger: l,
		Level:  record.Debug,
	}
}

func main() {
	toolkit.NewApp(
		"stitch-docker",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			commands, err := c.commands(cctx)

			if err != nil {
				return err
			}

			exc := c.executor(cctx.Logger)

			for _, args := range commands {
				err := exc.Exec(
					ctx,
					executil.Command{
						Cmd:    "docker",
						Args:   args,
						Stdout: cctx.CommandContext.Stdout,
						Stderr: cctx.CommandContext.Stderr,
					},
				)

				if err != nil {
					return errors.Wrap(err, "cannot create docker image")
				}
			}

			return nil
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
