package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/upfluence/actions/pkg/toolkit"
	"github.com/upfluence/errors"
)

type linkerMode int

const (
	none linkerMode = iota
	pkg
	cli
)

func (lm *linkerMode) Parse(v string) error {
	switch v {
	case "pkg":
		*lm = pkg
	case "cli":
		*lm = cli
	case "none":
		*lm = none
	default:
		return fmt.Errorf("Invalid linker-mode %q", v)
	}

	return nil
}

func (lm linkerMode) links(cctx toolkit.CommandContext, v string) map[string]string {
	switch lm {
	case pkg:
		return map[string]string{
			"github.com/upfluence/pkg/peer.Version":   v,
			"github.com/upfluence/pkg/peer.GitCommit": cctx.Sha,
			"github.com/upfluence/pkg/peer.GitBranch": cctx.RefName,
			"github.com/upfluence/pkg/peer.GitRemote": "https://github.com/" + cctx.Repository,
		}
	case cli:
		return map[string]string{
			"github.com/upfluence/cfg/x/cli.Version": v,
		}
	}

	return make(map[string]string)
}

type nameTemplate struct {
	t *template.Template
}

func (nt *nameTemplate) Parse(v string) error {
	var err error

	nt.t, err = template.New("").Parse(v)

	return err
}

func (nt nameTemplate) render(b build) (string, error) {
	if nt.t == nil {
		return b.Name(), nil
	}

	var buf bytes.Buffer

	err := nt.t.Execute(&buf, b)

	return buf.String(), err
}

type build struct {
	Path string

	Version string
	OS      string
	Arch    string
}

func (b build) Name() string {
	return filepath.Base(b.Path)
}

var defaultConfig = config{
	ExecutablePaths: []string{"."},
	DistDir:         "dist/",
	OSs:             []string{"linux"},
	Archs:           []string{"amd64"},
	LinkerMode:      none,
}

type config struct {
	Version         string            `flag:"release-version"`
	ExecutablePaths []string          `flag:"executable-paths"`
	DistDir         string            `flag:"dist-dir"`
	OSs             []string          `flag:"oss"`
	Archs           []string          `flag:"archs"`
	CGo             bool              `flag:"cgo"`
	LinkerMode      linkerMode        `flag:"linker-mode"`
	AdditionalLinks map[string]string `flag:"additional-links"`
	CompilerPath    string            `flag:"compiler-path"`
	NameTemplate    nameTemplate      `flag:"name-template"`
}

func (c config) executablePaths(cctx toolkit.CommandContext) ([]string, error) {
	var paths []string

	for _, exc := range c.ExecutablePaths {
		fnames, err := filepath.Glob(filepath.Join(".", exc))

		if err != nil {
			return nil, errors.Wrapf(err, "invalid glob %q", exc)
		}

		paths = append(paths, fnames...)
	}

	return paths, nil
}

func (c config) builds(cctx toolkit.CommandContext) ([]build, error) {
	ps, err := c.executablePaths(cctx)

	if err != nil {
		return nil, err
	}

	var bs []build

	for _, p := range ps {
		for _, os := range c.OSs {
			for _, arch := range c.Archs {
				bs = append(
					bs,
					build{
						Path:    p,
						Version: c.Version,
						OS:      os,
						Arch:    arch,
					},
				)
			}
		}
	}

	return bs, nil
}

func (c config) links(cctx toolkit.CommandContext) map[string]string {
	ls := c.LinkerMode.links(cctx, c.Version)

	for k, v := range c.AdditionalLinks {
		ls[k] = v
	}

	return ls
}

func (c config) compilerPath() (string, error) {
	if c.CompilerPath != "" {
		return c.CompilerPath, nil
	}

	return exec.LookPath("go")
}

type compiler struct {
	path string

	distDir string
	cgo     bool

	links map[string]string

	nt nameTemplate

	repo string
}

func newCompiler(c config, cctx toolkit.CommandContext) (*compiler, error) {
	p, err := c.compilerPath()

	if err != nil {
		return nil, err
	}

	return &compiler{
		path:    p,
		distDir: c.DistDir,
		cgo:     c.CGo,
		links:   c.links(cctx),
		nt:      c.NameTemplate,
		repo:    cctx.Repository,
	}, nil
}

func (c *compiler) execute(ctx context.Context, b build, cctx toolkit.CommandContext) error {
	t, err := c.nt.render(b)

	if err != nil {
		return err
	}

	ldFlags := []string{"-s"}

	for k, v := range c.links {
		ldFlags = append(ldFlags, fmt.Sprintf("-X %s=%s", k, v))
	}

	cmd := exec.CommandContext(
		ctx,
		c.path,
		"build",
		"-ldflags",
		strings.Join(ldFlags, " "),
		"-o",
		filepath.Join(c.distDir, t),
		"./"+b.Path,
	)

	cmd.Stdout = cctx.CommandContext.Stdout
	cmd.Stderr = cctx.CommandContext.Stderr

	cgoInt := 0

	if c.cgo {
		cgoInt = 1
	}

	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("GOOS=%s", b.OS),
		fmt.Sprintf("GOARCH=%s", b.Arch),
		fmt.Sprintf("CGO_ENABLED=%d", cgoInt),
	)

	if err == nil {
		cctx.Logger.Noticef("Finished compiling %s", filepath.Join(c.distDir, t))
	}

	return cmd.Run()
}

func main() {
	toolkit.NewApp(
		"compile-go",
		func(ctx context.Context, cctx toolkit.CommandContext, c config) error {
			bs, err := c.builds(cctx)

			if err != nil {
				return err
			}

			cp, err := newCompiler(c, cctx)

			if err != nil {
				return err
			}

			for _, b := range bs {
				if err := cp.execute(ctx, b, cctx); err != nil {
					return err
				}
			}

			return nil
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
