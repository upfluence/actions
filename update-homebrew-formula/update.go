package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/google/go-github/v53/github"
	"github.com/upfluence/errors"
	"github.com/upfluence/pkg/backoff"
	"github.com/upfluence/pkg/backoff/exponential"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/upfluence/actions/pkg/toolkit"
)

var backoffStrategy = backoff.LimitStrategy(exponential.NewDefaultBackoff(time.Second, 15*time.Second), 5)

type config struct {
	Version    string `flag:"release-version"`
	Template   string `flag:"template"`
	CLIName    string `flag:"cli-name"`
	Binaries   string `env:"BINARIES"`
	Repository string `flag:"repository"`
}

func camelCase(v string) string {
	var buf strings.Builder

	for _, k := range strings.Split(v, "-") {
		buf.WriteString(cases.Title(language.English).String(k))
	}

	return buf.String()
}

func main() {
	toolkit.NewApp(
		"update-homebrew-formula",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			c.Version = strings.TrimPrefix(c.Version, "v")

			t := template.New("").Funcs(template.FuncMap{"camelCase": camelCase})

			t, err := t.ParseFiles(c.Template)

			if err != nil {
				return errors.Wrapf(err, "cant parse template %q", c.Template)
			}

			var buf bytes.Buffer

			if err := t.ExecuteTemplate(&buf, filepath.Base(c.Template), c); err != nil {
				return errors.Wrap(err, "cant template file")
			}

			org, repo := cctx.SplittedRepository()

			fname := fmt.Sprintf("Formula/%s.rb", c.CLIName)

			i := 0

			for {
				commits, _, err := cctx.Client.Repositories.ListCommits(
					ctx,
					org,
					repo,
					&github.CommitsListOptions{Path: fname},
				)

				if err != nil {
					return err
				}

				var sha *string

				if len(commits) > 0 {
					commit := commits[0]

					t, _, err := cctx.Client.Git.GetTree(
						ctx,
						org,
						repo,
						commit.GetSHA(),
						true,
					)

					if err != nil {
						return err
					}

					for _, entry := range t.Entries {
						if *entry.Path == fname {
							sha = entry.SHA
							break
						}
					}
				}
				_, _, err = cctx.Client.Repositories.UpdateFile(
					ctx,
					org,
					repo,
					fname,
					&github.RepositoryContentFileOptions{
						Message: github.String(fmt.Sprintf("Update %s to v%s", c.CLIName, c.Version)),
						Content: buf.Bytes(),
						Branch:  &cctx.RefName,
						SHA:     sha,
					},
				)

				var ghErr *github.ErrorResponse

				if err == nil || !errors.As(err, &ghErr) || ghErr.Response.StatusCode != http.StatusConflict {
					return err
				}

				d, err := backoffStrategy.Backoff(i)

				if err != nil {
					return errors.Wrap(err, "backoff failed")
				}

				if d == backoff.Canceled {
					return ghErr
				}

				select {
				case <-ctx.Done():
				case <-time.After(d):
				}

				i++
			}
		},
	).Run(context.Background())
}
