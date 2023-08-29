package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/upfluence/errors"
	"github.com/upfluence/log"
	"github.com/upfluence/log/record"

	"github.com/upfluence/actions/pkg/executil"
	"github.com/upfluence/actions/pkg/toolkit"
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

func (b build) archKey() string { return fmt.Sprintf("%s/%s", b.OS, b.Arch) }

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

func (c config) executor(l log.Logger) executil.Executor {
	return executil.VerboseExecutor{
		Next:   executil.StdExecutor{PropagateEnviron: true},
		Logger: l,
		Level:  record.Debug,
	}
}

type compiler struct {
	path string

	executor executil.Executor

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
		path:     p,
		executor: c.executor(cctx.Logger),
		distDir:  c.DistDir,
		cgo:      c.CGo,
		links:    c.links(cctx),
		nt:       c.NameTemplate,
		repo:     cctx.Repository,
	}, nil
}

func (c *compiler) execute(ctx context.Context, b build, cctx toolkit.CommandContext) (string, string, error) {
	t, err := c.nt.render(b)

	if err != nil {
		return "", "", err
	}

	ldFlags := []string{"-s"}

	for k, v := range c.links {
		ldFlags = append(ldFlags, fmt.Sprintf("-X %s=%s", k, v))
	}

	cgoStr := "0"

	if c.cgo {
		cgoStr = "1"
	}

	filename := filepath.Join(c.distDir, t)

	err = c.executor.Exec(
		ctx,
		executil.Command{
			Cmd: c.path,
			Args: []string{
				"build",
				"-ldflags",
				strings.Join(ldFlags, " "),
				"-o",
				filename,
				"./" + b.Path,
			},

			Stdout: cctx.CommandContext.Stdout,
			Stderr: cctx.CommandContext.Stderr,
			Env: map[string]string{
				"GOOS":        b.OS,
				"GOARCH":      b.Arch,
				"CGO_ENABLED": cgoStr,
			},
		},
	)

	if err != nil {
		return "", "", err
	}

	cctx.Logger.Noticef("Finished compiling %s", filepath.Join(c.distDir, t))

	f, err := os.Open(filename)

	if err != nil {
		return "", "", errors.Wrapf(err, "cant open %q", filename)
	}

	h := sha256.New()
	_, err = io.Copy(h, f)

	return t, hex.EncodeToString(h.Sum(nil)), errors.Wrap(err, "cant hash the file")
}

type definition struct {
	Filename string `json:"filename"`
	Sha256   string `json:"sha256"`
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

			defs := make(map[string]map[string]definition)

			for _, b := range bs {
				fname, sha256Sum, err := cp.execute(ctx, b, cctx)

				if err != nil {
					return err
				}
				n := b.Name()

				if defs[n] == nil {
					defs[n] = make(map[string]definition, 1)
				}

				defs[n][b.archKey()] = definition{
					Filename: fname,
					Sha256:   sha256Sum,
				}
			}

			buf, err := json.Marshal(defs)

			if err != nil {
				return err
			}

			return cctx.Output.WriteKeyValue("definitions", string(buf))
		},
		toolkit.WithDefaultConfig(defaultConfig),
	).Run(context.Background())
}
