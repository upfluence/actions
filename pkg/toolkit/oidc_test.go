package toolkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/upfluence/pkg/testutil"
)

const testRequestToken = "request-token"

func tokenHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Header.Get("Authorization") != "bearer "+testRequestToken:
		w.WriteHeader(http.StatusUnauthorized)
	case r.URL.Query().Get("api-version") != "2.0":
		w.WriteHeader(http.StatusBadRequest)
	default:
		json.NewEncoder(w).Encode(
			map[string]string{"value": "jwt:" + r.URL.Query().Get("audience")},
		)
	}
}

func withRequestURL(token string) func(string) oidcConfig {
	return func(u string) oidcConfig {
		return oidcConfig{URL: u + "/idtoken?api-version=2.0", Token: token}
	}
}

func TestOIDCTokenFetcherFetch(t *testing.T) {
	for _, tt := range []struct {
		name         string
		haveHandler  http.HandlerFunc
		haveConfig   func(string) oidcConfig
		haveAudience string
		want         string
		errFn        testutil.ErrorAssertion
	}{
		{
			name:        "default audience",
			haveHandler: tokenHandler,
			haveConfig:  withRequestURL(testRequestToken),
			want:        "jwt:",
			errFn:       testutil.NoError(),
		},
		{
			name:         "custom audience",
			haveHandler:  tokenHandler,
			haveConfig:   withRequestURL(testRequestToken),
			haveAudience: "https://deploy.upfluence.co",
			want:         "jwt:https://deploy.upfluence.co",
			errFn:        testutil.NoError(),
		},
		{
			name:        "missing url",
			haveHandler: tokenHandler,
			haveConfig: func(string) oidcConfig {
				return oidcConfig{Token: testRequestToken}
			},
			errFn: testutil.ErrorCause(ErrOIDCTokenUnavailable),
		},
		{
			name:        "missing token",
			haveHandler: tokenHandler,
			haveConfig:  withRequestURL(""),
			errFn:       testutil.ErrorCause(ErrOIDCTokenUnavailable),
		},
		{
			name:        "unexpected status",
			haveHandler: tokenHandler,
			haveConfig:  withRequestURL("wrong-token"),
			errFn:       testutil.ErrorCause(errUnexpectedStatus),
		},
		{
			name: "empty value",
			haveHandler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`{"value": ""}`))
			},
			haveConfig: withRequestURL(testRequestToken),
			errFn:      testutil.ErrorCause(errEmptyOIDCToken),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.haveHandler)

			defer srv.Close()

			otf := newOIDCTokenFetcher(tt.haveConfig(srv.URL))

			got, err := otf.Fetch(context.Background(), tt.haveAudience)

			tt.errFn(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
