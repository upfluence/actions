package main

import "time"

var semverDefaultStrategies = map[string]strategy{
	"master":  bumpPatch,
	"main":    bumpPatch,
	"staging": bumpPre,
	"qa":      bumpRC,
}

type semverScheme struct{}

func (semverScheme) parse(t string) (*version, error) {
	return parse(t)
}

func (semverScheme) defaultStrategies() map[string]strategy {
	return semverDefaultStrategies
}

func (semverScheme) bump(v *version, s strategy, msgs []string, _ time.Time) error {
	if !incrementVersionFromCommits(v, msgs) {
		s.inc(v)
	}

	return nil
}
