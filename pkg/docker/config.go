package docker

import (
	"fmt"

	"github.com/upfluence/actions/pkg/toolkit"
)

const (
	None TagMode = iota
	App
)

var DefaultConfig = Config{
	Registries: []string{"index.docker.io"},
}

type TagMode int

func (tm *TagMode) Parse(v string) error {
	switch v {
	case "app":
		*tm = App
	case "none":
		*tm = None
	default:
		return fmt.Errorf("invalid tag-mode %q", v)
	}

	return nil
}

func (tm TagMode) Tags(cctx toolkit.CommandContext, version string) []string {
	if tm == None {
		return []string{cctx.Sha[:7]}
	}

	upstream := cctx.RefName

	if upstream == "master" || upstream == "main" {
		upstream = "latest"
	}

	return []string{version, upstream, cctx.Sha[:7]}
}

type Config struct {
	Version string `flag:"release-version"`

	TagMode        TagMode  `flag:"tag-mode"`
	AdditionalTags []string `flag:"additional-tags"`

	Registries           []string          `flag:"registries"`
	OverrideRepositories map[string]string `flag:"override-repositories"`
}

func (c Config) Tags(cctx toolkit.CommandContext) []string {
	return append(c.TagMode.Tags(cctx, c.Version), c.AdditionalTags...)
}

func (c Config) Repository(name string) string {
	if r, ok := c.OverrideRepositories[name]; ok {
		return r
	}

	return name
}
