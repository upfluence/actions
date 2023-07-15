package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/go-github/v53/github"
	"github.com/upfluence/actions/pkg/toolkit"
	"github.com/upfluence/errors"
)

type config struct {
	Version     string   `flag:"release-version"`
	Attachments []string `flag:"attachments"`
	Prerelease  bool     `flag:"prerelease"`
}

func (c config) attachments(cctx toolkit.CommandContext) ([]string, error) {
	var paths []string
	for _, att := range c.Attachments {
		fnames, err := filepath.Glob(filepath.Join(cctx.Workspace, att))

		if err != nil {
			return nil, errors.Wrapf(err, "invalid glob %q", att)
		}

		paths = append(paths, fnames...)
	}

	return paths, nil
}

func main() {
	toolkit.NewApp(
		"create-github-release",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			if c.Version == "" {
				return errors.New("non empty --version argument must be given")
			}

			org, repo := cctx.SplittedRepository()

			obj := &github.GitObject{
				SHA:  &cctx.Sha,
				Type: github.String("commit"),
			}

			if _, _, err := cctx.Client.Git.CreateTag(
				ctx,
				org,
				repo,
				&github.Tag{
					Tag:     &c.Version,
					Message: &c.Version,
					Object:  obj,
				},
			); err != nil {
				return err
			}

			if _, _, err := cctx.Client.Git.CreateRef(
				ctx,
				org,
				repo,
				&github.Reference{
					Ref: github.String(
						fmt.Sprintf("refs/tags/%s", c.Version),
					),
					Object: obj,
				},
			); err != nil {
				return err
			}

			release, _, err := cctx.Client.Repositories.CreateRelease(
				ctx,
				org,
				repo,
				&github.RepositoryRelease{
					TagName:              &c.Version,
					Prerelease:           &c.Prerelease,
					GenerateReleaseNotes: github.Bool(true),
				},
			)

			if err != nil {
				return err
			}

			fnames, err := c.attachments(cctx)

			if err != nil {
				return err
			}

			for _, fname := range fnames {
				f, err := os.Open(fname)

				if err != nil {
					return err
				}

				if _, _, err := cctx.Client.Repositories.UploadReleaseAsset(
					ctx,
					org,
					repo,
					release.GetID(),
					&github.UploadOptions{
						Name: filepath.Base(fname),
					},
					f,
				); err != nil {
					return err
				}

				f.Close()
			}

			return nil
		},
	).Run(context.Background())
}
