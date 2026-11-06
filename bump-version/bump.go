package main

import (
	"context"
	"fmt"
	"time"

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

	schemeByNames = map[string]scheme{
		"semver":       semverScheme{},
		"calver_month": calverMonthScheme{},
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

func (s strategy) String() string {
	for n, ts := range strategyByNames {
		if ts == s {
			return n
		}
	}

	return "noop"
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

type schemeValue struct {
	scheme
}

func (sv *schemeValue) Parse(v string) error {
	if v == "" || v == "true" {
		return nil
	}

	if s, ok := schemeByNames[v]; ok {
		sv.scheme = s

		return nil
	}

	return fmt.Errorf("%q is not a valid scheme", v)
}

func (sv schemeValue) String() string {
	for n, s := range schemeByNames {
		if s == sv.scheme {
			return n
		}
	}

	return ""
}

type config struct {
	Scheme             schemeValue
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
			c.Scheme.defaultStrategies(),
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
			tag, msgs, err := fetchContext(ctx, cctx, c.Scheme)

			if err != nil {
				return err
			}

			if err := c.Scheme.bump(
				tag,
				c.strategy(cctx),
				msgs,
				time.Now().UTC(),
			); err != nil {
				return err
			}

			cctx.Logger.Noticef(
				"Version computed: %s",
				tag.String(),
			)

			return cctx.Output.WriteKeyValue("version", tag.String())
		},
		toolkit.WithDefaultConfig(
			config{Scheme: schemeValue{scheme: semverScheme{}}},
		),
	).Run(context.Background())
}
