package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalverMonthParse(t *testing.T) {
	for _, tt := range []struct {
		name        string
		haveVersion string

		wantErr bool
	}{
		{name: "release", haveVersion: "v2026.11.0"},
		{name: "rc", haveVersion: "v2026.11.3-rc2"},
		{name: "semver", haveVersion: "v1.2.3", wantErr: true},
		{name: "month zero", haveVersion: "v2026.0.1", wantErr: true},
		{name: "month overflow", haveVersion: "v2026.13.1", wantErr: true},
		{name: "zero padded", haveVersion: "v2026.01.00", wantErr: true},
		{name: "missing prefix", haveVersion: "2026.11.0", wantErr: true},
		{name: "pre", haveVersion: "v2026.11.3-rc1_pre1", wantErr: true},
		{name: "non rc prerelease", haveVersion: "v2026.11.3-beta", wantErr: true},
		{name: "invalid", haveVersion: "foo", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, err := calverMonthScheme{}.parse(tt.haveVersion)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.haveVersion, v.String())
		})
	}
}

func TestCalverMonthBump(t *testing.T) {
	now := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)

	for _, tt := range []struct {
		name         string
		haveVersion  string
		haveStrategy strategy
		haveNow      time.Time

		want    string
		wantErr error
	}{
		{
			name:         "patch from no tag",
			haveVersion:  "v0.0.0",
			haveStrategy: bumpPatch,
			haveNow:      now,
			want:         "v2026.11.0",
		},
		{
			name:         "patch from semver tag",
			haveVersion:  "v1.4.2",
			haveStrategy: bumpPatch,
			haveNow:      now,
			want:         "v2026.11.0",
		},
		{
			name:         "patch from previous month",
			haveVersion:  "v2026.10.7",
			haveStrategy: bumpPatch,
			haveNow:      now,
			want:         "v2026.11.0",
		},
		{
			name:         "patch from previous year",
			haveVersion:  "v2025.12.4",
			haveStrategy: bumpPatch,
			haveNow:      time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC),
			want:         "v2026.1.0",
		},
		{
			name:         "patch from current month",
			haveVersion:  "v2026.11.2",
			haveStrategy: bumpPatch,
			haveNow:      now,
			want:         "v2026.11.3",
		},
		{
			name:         "patch promotes current month rc",
			haveVersion:  "v2026.11.3-rc2",
			haveStrategy: bumpPatch,
			haveNow:      now,
			want:         "v2026.11.3",
		},
		{
			name:         "patch from previous month rc",
			haveVersion:  "v2026.10.3-rc2",
			haveStrategy: bumpPatch,
			haveNow:      now,
			want:         "v2026.11.0",
		},
		{
			name:         "rc from previous month",
			haveVersion:  "v2026.10.7",
			haveStrategy: bumpRC,
			haveNow:      now,
			want:         "v2026.11.0-rc1",
		},
		{
			name:         "rc from previous month rc",
			haveVersion:  "v2026.10.7-rc3",
			haveStrategy: bumpRC,
			haveNow:      now,
			want:         "v2026.11.0-rc1",
		},
		{
			name:         "rc from current month release",
			haveVersion:  "v2026.11.2",
			haveStrategy: bumpRC,
			haveNow:      now,
			want:         "v2026.11.3-rc1",
		},
		{
			name:         "rc from current month rc",
			haveVersion:  "v2026.11.3-rc1",
			haveStrategy: bumpRC,
			haveNow:      now,
			want:         "v2026.11.3-rc2",
		},
		{
			name:         "minor is not supported",
			haveVersion:  "v2026.11.2",
			haveStrategy: bumpMinor,
			haveNow:      now,
			want:         "v2026.11.2",
			wantErr:      errUnsupportedStrategy,
		},
		{
			name:         "pre is not supported",
			haveVersion:  "v2026.11.2",
			haveStrategy: bumpPre,
			haveNow:      now,
			want:         "v2026.11.2",
			wantErr:      errUnsupportedStrategy,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parse(tt.haveVersion)
			require.NoError(t, err)

			err = calverMonthScheme{}.bump(
				v,
				tt.haveStrategy,
				[]string{"bump-major"},
				tt.haveNow,
			)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, v.String())
		})
	}
}
