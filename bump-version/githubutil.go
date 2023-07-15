package main

import (
	"context"

	"github.com/google/go-github/v53/github"
	"github.com/upfluence/actions/pkg/toolkit"
)

func fetchLatestTag(ctx context.Context, cctx toolkit.CommandContext) (*version, error) {
	org, repo := cctx.SplittedRepository()

	tags, _, err := cctx.Client.Repositories.ListTags(
		ctx,
		org,
		repo,
		&github.ListOptions{PerPage: 100},
	)

	if err != nil {
		return nil, err
	}

	if len(tags) == 0 {
		return parse("v0.0.0")
	}

	var v *version

	for _, tag := range tags {
		lv, err := parse(tag.GetName())

		if err != nil {
			continue
		}

		if v == nil {
			v = lv
			continue
		}

		if v.Compare(lv) < 0 {
			v = lv
		}
	}

	return parse(tags[0].GetName())
}

func fetchContext(ctx context.Context, cctx toolkit.CommandContext) (*version, []string, error) {
	tag, err := fetchLatestTag(ctx, cctx)

	if err != nil {
		return nil, nil, err
	}

	org, repo := cctx.SplittedRepository()

	commits, _, err := cctx.Client.Repositories.CompareCommits(
		ctx,
		org,
		repo,
		tag.String(),
		cctx.Sha,
		&github.ListOptions{PerPage: 100},
	)

	if err != nil {
		return nil, nil, err
	}

	var msgs []string

	for _, c := range commits.Commits {
		msgs = append(msgs, c.GetCommit().GetMessage())
	}

	return tag, msgs, nil
}
