package main

import "time"

type scheme interface {
	parse(string) (*version, error)
	defaultStrategies() map[string]strategy
	bump(*version, strategy, []string, time.Time) error
}
