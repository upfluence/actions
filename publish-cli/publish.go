package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/go-github/v53/github"
	"github.com/upfluence/errors"

	"github.com/upfluence/actions/pkg/toolkit"
)

var defaultConfig = config{
	HomebrewTap:      "upfluence/tap",
	WorkflowFilename: "homebrew-formula-update.yml",
	Template:         "template/formula.rb.template",
	TargetRef:        "main",
}

type definitions map[string]map[string]any

type config struct {
	Version          string `flag:"release-version"`
	HomebrewTap      string `flag:"homebrew-tap"`
	WorkflowFilename string `flag:"workflow-filename"`
	TargetRef        string `flag:"target-ref"`
	Template         string `flag:"template"`
	Definitions      string `env:"DEFINITIONS"`
}

func (c config) targetRepo() (string, string) {
	sr := strings.Split(c.HomebrewTap, "/")

	if len(sr) != 2 {
		panic(fmt.Sprintf("Invalid homebrew-tap format: %q", c.HomebrewTap))
	}

	return sr[0], "homebrew-" + sr[1]
}

func main() {
	toolkit.NewApp(
		"publish-cli",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			org, repo := c.targetRepo()

			var defs definitions

			json.Unmarshal([]byte(c.Definitions), &defs)

			for k, def := range defs {
				buf, err := json.Marshal(def)

				if err != nil {
					return errors.Wrapf(err, "cant marshal def of %q", k)
				}

				if _, err := cctx.Client.Actions.CreateWorkflowDispatchEventByFileName(
					ctx,
					org,
					repo,
					c.WorkflowFilename,
					github.CreateWorkflowDispatchEventRequest{
						Ref: c.TargetRef,
						Inputs: map[string]interface{}{
							"cli_name":   k,
							"version":    c.Version,
							"repository": cctx.Repository,
							"binaries":   string(buf),
							"template":   c.Template,
						},
					},
				); err != nil {
					return errors.Wrapf(err, "cant dispatch event for %q", k)
				}
			}

			return nil
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
