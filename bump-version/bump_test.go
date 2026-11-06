package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/upfluence/actions/pkg/toolkit"
)

func TestSchemeValueParse(t *testing.T) {
	for _, tt := range []struct {
		name      string
		haveValue string

		want    scheme
		wantErr bool
	}{
		{name: "empty keeps default", haveValue: "", want: semverScheme{}},
		{name: "true keeps default", haveValue: "true", want: semverScheme{}},
		{name: "semver", haveValue: "semver", want: semverScheme{}},
		{
			name:      "calver_month",
			haveValue: "calver_month",
			want:      calverMonthScheme{},
		},
		{
			name:      "unknown",
			haveValue: "calver_day",
			want:      semverScheme{},
			wantErr:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sv := schemeValue{scheme: semverScheme{}}

			err := sv.Parse(tt.haveValue)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tt.want, sv.scheme)
		})
	}
}

func TestConfigStrategy(t *testing.T) {
	for _, tt := range []struct {
		name       string
		haveConfig config
		haveCctx   toolkit.CommandContext

		want strategy
	}{
		{
			name:       "semver staging",
			haveConfig: config{Scheme: schemeValue{scheme: semverScheme{}}},
			haveCctx:   toolkit.CommandContext{RefType: "branch", RefName: "staging"},
			want:       bumpPre,
		},
		{
			name:       "calver_month staging",
			haveConfig: config{Scheme: schemeValue{scheme: calverMonthScheme{}}},
			haveCctx:   toolkit.CommandContext{RefType: "branch", RefName: "staging"},
			want:       bumpRC,
		},
		{
			name:       "calver_month qa",
			haveConfig: config{Scheme: schemeValue{scheme: calverMonthScheme{}}},
			haveCctx:   toolkit.CommandContext{RefType: "branch", RefName: "qa"},
			want:       bumpRC,
		},
		{
			name:       "calver_month master",
			haveConfig: config{Scheme: schemeValue{scheme: calverMonthScheme{}}},
			haveCctx:   toolkit.CommandContext{RefType: "branch", RefName: "master"},
			want:       bumpPatch,
		},
		{
			name:       "calver_month unknown branch",
			haveConfig: config{Scheme: schemeValue{scheme: calverMonthScheme{}}},
			haveCctx:   toolkit.CommandContext{RefType: "branch", RefName: "feature"},
			want:       bumpPatch,
		},
		{
			name: "branch override",
			haveConfig: config{
				Scheme:             schemeValue{scheme: calverMonthScheme{}},
				StrategiesByBranch: map[string]strategy{"staging": bumpPatch},
			},
			haveCctx: toolkit.CommandContext{RefType: "branch", RefName: "staging"},
			want:     bumpPatch,
		},
		{
			name: "explicit strategy",
			haveConfig: config{
				Scheme:   schemeValue{scheme: calverMonthScheme{}},
				Strategy: bumpRC,
			},
			haveCctx: toolkit.CommandContext{RefType: "branch", RefName: "master"},
			want:     bumpRC,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.haveConfig.strategy(tt.haveCctx))
		})
	}
}
