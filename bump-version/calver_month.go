package main

import (
	"fmt"
	"time"

	"github.com/Masterminds/semver"
	"github.com/upfluence/errors"
)

var (
	errUnsupportedStrategy = errors.New("unsupported strategy")

	calverMonthDefaultStrategies = map[string]strategy{
		"master":  bumpPatch,
		"main":    bumpPatch,
		"staging": bumpRC,
		"qa":      bumpRC,
	}
)

type calverMonthScheme struct{}

func (calverMonthScheme) parse(t string) (*version, error) {
	v, err := parse(t)

	if err != nil {
		return nil, err
	}

	if v.String() != t || v.Major() < 2000 || v.Minor() < 1 ||
		v.Minor() > 12 || v.Metadata() != "" ||
		(v.Prerelease() != "" && v.RC() == 0) {
		return nil, fmt.Errorf("%q is not a valid calver_month version", t)
	}

	return v, nil
}

func (calverMonthScheme) defaultStrategies() map[string]strategy {
	return calverMonthDefaultStrategies
}

func (calverMonthScheme) bump(v *version, s strategy, _ []string, now time.Time) error {
	if s != bumpPatch && s != bumpRC {
		return errors.Wrapf(
			errUnsupportedStrategy,
			"calver_month does not support %s",
			s,
		)
	}

	if v.Major() == int64(now.Year()) && v.Minor() == int64(now.Month()) {
		s.inc(v)

		return nil
	}

	v.Version = *semver.MustParse(
		fmt.Sprintf("%d.%d.0", now.Year(), now.Month()),
	)

	if s == bumpRC {
		v.Version, _ = v.SetPrerelease("rc1")
	}

	return nil
}
