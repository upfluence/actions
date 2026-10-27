package toolkit

import (
	"bytes"
	"context"
	"net/http"
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
		`::debug file=command_test.go,line=21::foobar
::error file=command_test.go,line=22,title=buf::buz
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
		otf OIDCTokenFetcher

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
	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_URL", "https://example.com/idtoken")
	os.Setenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN", "request-token")

	NewApp(
		"fiz",
		func(_ context.Context, cctx CommandContext, fc fakeConfig) error {
			foo = fc.Foo
			otf = cctx.OIDCToken
			cctx.Path.WriteLine("line")
			cctx.State.WriteKeyValue("key", "value")
			return nil
		},
	).Execute(context.Background())

	assert.Equal(t, "bar", foo)
	assert.Equal(
		t,
		&oidcTokenFetcher{
			url:    "https://example.com/idtoken",
			token:  "request-token",
			client: http.DefaultClient,
		},
		otf,
	)

	assertFileContent(t, fp, "line\n")
	assertFileContent(t, fs, "key=value\n")
}

func assertFileContent(t *testing.T, f *os.File, expect string) {
	line, err := os.ReadFile(f.Name())
	assert.NoError(t, err)
	assert.Equal(t, expect, string(line))
}
