package main

import (
	"context"
	"fmt"

	"github.com/upfluence/actions/pkg/toolkit"
)

type strategy int

const (
	noop strategy = iota
	bumpRC
	bumpPatch
	bumpMinor
	bumpMajor
	bumpPre
)

var (
	strategyByNames = map[string]strategy{
		"bump_rc":    bumpRC,
		"bump_patch": bumpPatch,
		"bump_minor": bumpMinor,
		"bump_major": bumpMajor,
		"bump_pre":   bumpPre,
	}

	defaultStrategyByBranch = map[string]strategy{
		"master":  bumpPatch,
		"main":    bumpPatch,
		"staging": bumpPre,
		"qa":      bumpRC,
	}
)

func (s *strategy) Parse(v string) error {
	if v == "" || v == "true" {
		return nil
	}

	if ts, ok := strategyByNames[v]; ok {
		*s = ts
		return nil
	}

	return fmt.Errorf("%q is not a valid strategy", v)
}

func (s strategy) inc(v *version) {
	switch s {
	case bumpRC:
		v.IncRC()
	case bumpPatch:
		v.IncPatch()
	case bumpMinor:
		v.IncMinor()
	case bumpMajor:
		v.IncMajor()
	case bumpPre:
		v.IncPre()
	}
}

type config struct {
	Strategy           strategy
	StrategiesByBranch map[string]strategy `flag:"strategies-by-branch"`
}

func (c config) strategy(cctx toolkit.CommandContext) strategy {
	if c.Strategy != noop {
		return c.Strategy
	}

	if cctx.RefType == "branch" {
		for _, ss := range []map[string]strategy{
			c.StrategiesByBranch,
			defaultStrategyByBranch,
		} {
			if s := ss[cctx.RefName]; s != noop {
				return s
			}
		}
	}

	return bumpPatch
}

func main() {
	toolkit.NewApp(
		"bump-version",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			tag, msgs, err := fetchContext(ctx, cctx)

			if err != nil {
				return err
			}

			if !incrementVersionFromCommits(tag, msgs) {
				c.strategy(cctx).inc(tag)
			}

			cctx.Logger.Noticef(
				"Version computed: %s",
				tag.String(),
			)

			return cctx.Output.WriteKeyValue("version", tag.String())
		},
	).Run(context.Background())
}
