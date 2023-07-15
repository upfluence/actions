package toolkit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/go-github/v53/github"
	"github.com/upfluence/cfg/x/cli"
	"github.com/upfluence/log"
	"github.com/upfluence/log/pkg/stacktrace"
	"github.com/upfluence/log/record"
	"golang.org/x/oauth2"
)

type Title string

func (t Title) GetKey() string   { return "title" }
func (t Title) GetValue() string { return string(t) }

type sink struct {
	w io.Writer
}

func formalLevel(lvl record.Level) string {
	switch {
	case lvl >= record.Error:
		return "error"
	case lvl == record.Warning:
		return "warning"
	case lvl >= record.Notice:
		return "notice"
	default:
		return "debug"
	}
}

// ::notice file={name},line={line},endLine={endLine},title={title}::{message}
func (s *sink) Log(r record.Record) error {
	io.WriteString(s.w, "::")
	io.WriteString(s.w, formalLevel(r.Level()))

	initial := true

	if frame := stacktrace.FindCaller(2, []string{"github.com/upfluence/intenral/toolkit"}); frame != nil {
		fmt.Fprintf(s.w, " file=%s,line=%d", filepath.Base(frame.File), frame.Line)
		initial = false
	}

	for _, f := range r.Fields() {
		if f.GetKey() != "title" {
			continue
		}

		r := ','

		if initial {
			r = ' '
		}

		fmt.Fprintf(s.w, "%ctitle=%s", r, f.GetValue())

		break
	}

	io.WriteString(s.w, "::")
	r.WriteFormatted(s.w)
	io.WriteString(s.w, "\n")

	return nil
}

func newLogger(w io.Writer) log.Logger {
	return log.NewLogger(
		log.WithSink(&sink{w: w}),
	)
}

type lazyFile struct {
	fname string
	once  sync.Once

	err error
	f   *os.File
}

func (lf *lazyFile) Parse(v string) error {
	lf.fname = v
	return nil
}

func (lf *lazyFile) Write(buf []byte) (int, error) {
	lf.once.Do(func() {
		if lf.fname == "" {
			lf.f = os.Stdout
			return
		}

		lf.f, lf.err = os.OpenFile(lf.fname, os.O_APPEND|os.O_WRONLY, 0644)
	})

	if lf.err != nil {
		return 0, lf.err
	}

	defer lf.f.Sync()

	return lf.f.Write(buf)
}

type lineWriter struct {
	w io.Writer
}

func (lw lineWriter) WriteLine(v string) error {
	_, err := fmt.Fprintf(lw.w, "%s\n", v)
	return err
}

type LineWriter interface {
	WriteLine(string) error
}

type keyValueWriter struct {
	w io.Writer
}

func (kvw keyValueWriter) WriteKeyValue(k, v string) error {
	_, err := fmt.Fprintf(kvw.w, "%s=%s\n", k, v)
	return err
}

type KeyValueWriter interface {
	WriteKeyValue(string, string) error
}

type localConfig struct {
	Env         string
	Path        string
	Output      string
	StepSummary string `env:"STEP_SUMMARY"`
	State       string

	Token string

	EventName string `env:"EVENT_NAME"`

	Ref     string `env:"REF"`
	BaseRef string `env:"BASE_REF"`
	HeadRef string `env:"HEAD_REF"`
	Sha     string `env:"SHA"`
	RefName string `env:"REF_NAME"`
	RefType string `env:"REF_TYPE"`

	Repository string `env:"REPOSITORY"`
	Workspace  string `env:"WORKSPACE"`
}

type CommandContext struct {
	CommandContext cli.CommandContext

	Logger log.Logger

	StepSummary LineWriter
	Path        LineWriter

	Env    KeyValueWriter
	State  KeyValueWriter
	Output KeyValueWriter

	EventName string

	Ref     string
	BaseRef string
	HeadRef string
	Sha     string
	RefName string
	RefType string

	Workspace  string
	Repository string

	Client *github.Client
}

func (cc CommandContext) SplittedRepository() (string, string) {
	sr := strings.Split(cc.Repository, "/")

	if len(sr) != 2 {
		panic(fmt.Sprintf("Invalid GITHUB_REPOSITORY format: %q", cc.Repository))
	}

	return sr[0], sr[1]
}

func newCommandContext(cctx cli.CommandContext, lc localConfig) CommandContext {
	return CommandContext{
		CommandContext: cctx,
		Logger:         newLogger(os.Stdout),
		StepSummary:    &lineWriter{w: &lazyFile{fname: lc.StepSummary}},
		Path:           &lineWriter{w: &lazyFile{fname: lc.Path}},
		Env:            &keyValueWriter{w: &lazyFile{fname: lc.Env}},
		State:          &keyValueWriter{w: &lazyFile{fname: lc.State}},
		Output:         &keyValueWriter{w: &lazyFile{fname: lc.Output}},
		EventName:      lc.EventName,
		Ref:            lc.Ref,
		BaseRef:        lc.BaseRef,
		HeadRef:        lc.HeadRef,
		Sha:            lc.Sha,
		RefName:        lc.RefName,
		RefType:        lc.RefType,
		Repository:     lc.Repository,
		Workspace:      lc.Workspace,
		Client: github.NewClient(
			oauth2.NewClient(
				context.Background(),
				oauth2.StaticTokenSource(
					&oauth2.Token{AccessToken: lc.Token},
				),
			),
		),
	}
}

type configWrapper[T any] struct {
	Args   T           `env:"" flag:""`
	Github localConfig `env:"GITHUB" flag:"-"`
}

func WithDefaultConfig[T any](v T) Option[T] {
	return option[T]{
		co: cli.WithDefaultConfig(configWrapper[T]{Args: v}),
	}
}

func WrapCommand[T any](fn func(context.Context, CommandContext, T) error, opts ...cli.DefaultStaticCommandOption[configWrapper[T]]) cli.StaticCommand {
	return cli.DefaultStaticCommand(
		func(ctx context.Context, cctx cli.CommandContext, cw configWrapper[T]) error {
			return fn(ctx, newCommandContext(cctx, cw.Github), cw.Args)
		},
		opts...,
	)
}

type option[T any] struct {
	ao cli.Option
	co cli.DefaultStaticCommandOption[configWrapper[T]]
}

func (o option[T]) appOption() cli.Option                                           { return o.ao }
func (o option[T]) commandOption() cli.DefaultStaticCommandOption[configWrapper[T]] { return o.co }

type Option[T any] interface {
	appOption() cli.Option
	commandOption() cli.DefaultStaticCommandOption[configWrapper[T]]
}

func NewApp[T any](name string, fn func(context.Context, CommandContext, T) error, opts ...Option[T]) *cli.App {
	var (
		cos []cli.DefaultStaticCommandOption[configWrapper[T]]
		aos []cli.Option
	)

	for _, opt := range opts {
		if ao := opt.appOption(); ao != nil {
			aos = append(aos, ao)
		}

		if co := opt.commandOption(); co != nil {
			cos = append(cos, co)
		}
	}

	return cli.NewApp(
		append(
			[]cli.Option{
				cli.WithName(name),
				cli.WithCommand(WrapCommand(fn, cos...)),
			},
			aos...,
		)...,
	)
}
