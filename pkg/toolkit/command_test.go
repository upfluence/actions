package toolkit

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogging(t *testing.T) {
	var (
		buf bytes.Buffer

		l = newLogger(&buf)
	)

	l.Debug("foobar")
	l.WithField(Title("buf")).Error("buz")

	assert.Equal(
		t,
		`::debug file=command_test.go,line=20::foobar
::error file=command_test.go,line=21,title=buf::buz
`,
		buf.String(),
	)
}

type fakeConfig struct {
	Foo string
}

func TestAppIntegration(t *testing.T) {
	var (
		foo string

		fp, errp = os.CreateTemp(t.TempDir(), "")
		fs, errs = os.CreateTemp(t.TempDir(), "")
	)

	require.NoError(t, errp)
	require.NoError(t, errs)

	fp.Close()
	fs.Close()

	os.Setenv("GITHUB_PATH", fp.Name())
	os.Setenv("GITHUB_STATE", fs.Name())
	os.Setenv("FOO", "bar")

	NewApp(
		"fiz",
		func(_ context.Context, cctx CommandContext, fc fakeConfig) error {
			foo = fc.Foo
			cctx.Path.WriteLine("line")
			cctx.State.WriteKeyValue("key", "value")
			return nil
		},
	).Execute(context.Background())

	assert.Equal(t, "bar", foo)

	assertFileContent(t, fp, "line\n")
	assertFileContent(t, fs, "key=value\n")
}

func assertFileContent(t *testing.T, f *os.File, expect string) {
	line, err := os.ReadFile(f.Name())
	assert.NoError(t, err)
	assert.Equal(t, expect, string(line))
}
