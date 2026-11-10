package toolkit

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/upfluence/errors"
	"github.com/upfluence/pkg/testutil"
)

var errWrite = errors.New("write failure")

type failingKeyValueWriter struct {
	err error
}

func (fkvw failingKeyValueWriter) WriteKeyValue(string, string) error {
	return fkvw.err
}

func TestEnvironmentExport(t *testing.T) {
	for _, tt := range []struct {
		name         string
		haveEnv      Environment
		haveWriteErr error

		wantOut   string
		wantEnv   string
		assertErr testutil.ErrorAssertion
	}{
		{
			name:      "empty",
			assertErr: testutil.NoError(),
		},
		{
			name: "masks secrets before exporting variables",
			haveEnv: Environment{
				Variables: []Variable{
					{Key: "FOO", Value: "bar"},
					{Key: "TOKEN", Value: "s3cr3t"},
				},
				Secrets: []string{"s3cr3t"},
			},
			wantOut:   "::add-mask::s3cr3t\n",
			wantEnv:   "FOO=bar\nTOKEN=s3cr3t\n",
			assertErr: testutil.NoError(),
		},
		{
			name: "env write failure",
			haveEnv: Environment{
				Variables: []Variable{{Key: "FOO", Value: "bar"}},
			},
			haveWriteErr: errWrite,
			assertErr:    testutil.ErrorCause(errWrite),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var (
				out bytes.Buffer
				env bytes.Buffer

				kvw KeyValueWriter = keyValueWriter{w: &env}
			)

			if tt.haveWriteErr != nil {
				kvw = failingKeyValueWriter{err: tt.haveWriteErr}
			}

			err := tt.haveEnv.Export(masker{w: &out}, kvw)

			tt.assertErr(t, err)
			assert.Equal(t, tt.wantOut, out.String())
			assert.Equal(t, tt.wantEnv, env.String())
		})
	}
}
