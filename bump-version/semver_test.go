package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSemverBump(t *testing.T) {
	for _, tt := range []struct {
		name         string
		haveVersion  string
		haveStrategy strategy
		haveMessages []string

		want string
	}{
		{
			name:         "rc from release",
			haveVersion:  "v0.0.0",
			haveStrategy: bumpRC,
			want:         "v0.0.1-rc1",
		},
		{
			name:         "pre from release",
			haveVersion:  "v0.0.0",
			haveStrategy: bumpPre,
			want:         "v0.0.1-rc1_pre1",
		},
		{
			name:         "pre from pre",
			haveVersion:  "v0.0.1-rc1_pre1",
			haveStrategy: bumpPre,
			want:         "v0.0.1-rc1_pre2",
		},
		{
			name:         "rc from pre",
			haveVersion:  "v0.0.1-rc1_pre1",
			haveStrategy: bumpRC,
			want:         "v0.0.1-rc2",
		},
		{
			name:         "patch promotes rc",
			haveVersion:  "v0.0.1-rc2",
			haveStrategy: bumpPatch,
			want:         "v0.0.1",
		},
		{
			name:         "patch from release",
			haveVersion:  "v1.2.3",
			haveStrategy: bumpPatch,
			want:         "v1.2.4",
		},
		{
			name:         "minor from release",
			haveVersion:  "v1.2.3",
			haveStrategy: bumpMinor,
			want:         "v1.3.0",
		},
		{
			name:         "major from pre",
			haveVersion:  "v0.0.1-rc1_pre1",
			haveStrategy: bumpMajor,
			want:         "v1.0.0",
		},
		{
			name:         "bump-major commit overrides strategy",
			haveVersion:  "v1.2.3",
			haveStrategy: bumpPatch,
			haveMessages: []string{"foo: bar\n\nbump-major"},
			want:         "v2.0.0",
		},
		{
			name:         "bump-minor commit overrides strategy",
			haveVersion:  "v1.2.3",
			haveStrategy: bumpRC,
			haveMessages: []string{"foo: bar", "bump-minor"},
			want:         "v1.3.0",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parse(tt.haveVersion)
			require.NoError(t, err)

			err = semverScheme{}.bump(
				v,
				tt.haveStrategy,
				tt.haveMessages,
				time.Now(),
			)

			require.NoError(t, err)
			assert.Equal(t, tt.want, v.String())
		})
	}
}
