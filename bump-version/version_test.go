package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrategy(t *testing.T) {
	for _, tt := range []struct {
		have     string
		strategy strategy

		want string
	}{
		{
			have:     "v0.0.0",
			strategy: bumpRC,
			want:     "v0.0.1-rc1",
		},
		{
			have:     "v0.0.0",
			strategy: bumpPre,
			want:     "v0.0.1-rc1_pre1",
		},
		{
			have:     "v0.0.1-rc1_pre1",
			strategy: bumpRC,
			want:     "v0.0.1-rc2",
		},
		{
			have:     "v0.0.1-rc1_pre1",
			strategy: bumpMajor,
			want:     "v1.0.0",
		},
	} {
		v, err := parse(tt.have)
		require.NoError(t, err)

		tt.strategy.inc(v)

		assert.Equal(t, tt.want, v.String())
	}
}
