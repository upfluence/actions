package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/upfluence/actions/pkg/toolkit"
)

func TestConfigTags(t *testing.T) {
	for _, tt := range []struct {
		name       string
		haveConfig Config
		haveRef    string
		want       []string
	}{
		{
			name: "app tags on main",
			haveConfig: Config{
				Version: "v1.2.3",
				TagMode: App,
			},
			haveRef: "main",
			want:    []string{"v1.2.3", "latest", "0123456"},
		},
		{
			name: "app tags on branch with additional tags",
			haveConfig: Config{
				Version:        "v1.2.3",
				TagMode:        App,
				AdditionalTags: []string{"stable"},
			},
			haveRef: "staging",
			want:    []string{"v1.2.3", "staging", "0123456", "stable"},
		},
		{
			name: "none mode",
			haveConfig: Config{
				TagMode: None,
			},
			want: []string{"0123456"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.haveConfig.Tags(
				toolkit.CommandContext{
					RefName: tt.haveRef,
					Sha:     "0123456789",
				},
			)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfigRepository(t *testing.T) {
	for _, tt := range []struct {
		name       string
		haveConfig Config
		want       string
	}{
		{
			name: "original repository",
			want: "upfluence/example",
		},
		{
			name: "overridden repository",
			haveConfig: Config{
				OverrideRepositories: map[string]string{
					"upfluence/example": "upfluence/renamed",
				},
			},
			want: "upfluence/renamed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.haveConfig.Repository("upfluence/example"))
		})
	}
}
