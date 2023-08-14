package executil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/upfluence/log"
	"github.com/upfluence/log/record"
)

type Command struct {
	Cmd  string
	Args []string

	Env map[string]string
	Dir string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Executor interface {
	Exec(context.Context, Command) error
}

type StdExecutor struct {
	PropagateEnviron bool
}

func (se StdExecutor) Exec(ctx context.Context, cmd Command) error {
	c := exec.CommandContext(ctx, cmd.Cmd, cmd.Args...)

	if se.PropagateEnviron {
		c.Env = os.Environ()
	}

	for k, v := range cmd.Env {
		c.Env = append(c.Env, fmt.Sprintf("%s=%s", k, v))
	}

	c.Dir = cmd.Dir
	c.Stdin = cmd.Stdin
	c.Stdout = cmd.Stdout
	c.Stderr = cmd.Stderr

	return c.Run()
}

type VerboseExecutor struct {
	Next   Executor
	Logger log.Logger
	Level  record.Level
}

func (ve VerboseExecutor) Exec(ctx context.Context, cmd Command) error {
	t0 := time.Now()
	err := ve.Next.Exec(ctx, cmd)

	var status int

	if err != nil {
		var exitErr *exec.ExitError

		if errors.As(err, &exitErr) {
			status = exitErr.ExitCode()
		} else {
			status = -1
		}
	}

	ve.Logger.WithFields(
		log.Field("status", status),
		log.Field("duration", time.Since(t0)),
	).Logf(
		ve.Level,
		"executing: %s", strings.Join(append([]string{cmd.Cmd}, cmd.Args...), " "),
	)

	return err
}
