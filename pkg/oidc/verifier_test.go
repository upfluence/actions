package oidc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/upfluence/errors"
	"github.com/upfluence/pkg/testutil"
)

const (
	testClientID = "test-client"
	testKeyID    = "test-key"

	testJTI        = "example-id"
	testRef        = "refs/heads/main"
	testRepository = "octo-org/octo-repo"
	testOwner      = "octo-org"
	testVisibility = "private"
)

func generateKey(t *testing.T) *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)

	require.NoError(t, err)

	return k
}

func newIssuer(t *testing.T, k *rsa.PrivateKey) *httptest.Server {
	var (
		mux = http.NewServeMux()
		srv = httptest.NewServer(mux)
	)

	mux.HandleFunc(
		"/.well-known/openid-configuration",
		func(w http.ResponseWriter, _ *http.Request) {
			json.NewEncoder(w).Encode(
				map[string]any{
					"issuer":                                srv.URL,
					"jwks_uri":                              srv.URL + "/.well-known/jwks",
					"id_token_signing_alg_values_supported": []string{"RS256"},
				},
			)
		},
	)

	mux.HandleFunc(
		"/.well-known/jwks",
		func(w http.ResponseWriter, _ *http.Request) {
			json.NewEncoder(w).Encode(
				jose.JSONWebKeySet{
					Keys: []jose.JSONWebKey{
						{
							Key:       &k.PublicKey,
							KeyID:     testKeyID,
							Algorithm: string(jose.RS256),
							Use:       "sig",
						},
					},
				},
			)
		},
	)

	t.Cleanup(srv.Close)

	return srv
}

func signToken(t *testing.T, k *rsa.PrivateKey, claims map[string]any) string {
	s, err := jose.NewSigner(
		jose.SigningKey{
			Algorithm: jose.RS256,
			Key:       jose.JSONWebKey{Key: k, KeyID: testKeyID},
		},
		(&jose.SignerOptions{}).WithType("JWT"),
	)

	require.NoError(t, err)

	payload, err := json.Marshal(claims)

	require.NoError(t, err)

	jws, err := s.Sign(payload)

	require.NoError(t, err)

	raw, err := jws.CompactSerialize()

	require.NoError(t, err)

	return raw
}

func withClaims(base map[string]any, overrides map[string]any) map[string]any {
	res := maps.Clone(base)

	maps.Copy(res, overrides)

	return res
}

func tokenClaims(tok *IDToken) *Claims {
	if tok == nil {
		return nil
	}

	return &tok.Claims
}

func TestVerifierVerify(t *testing.T) {
	var (
		ctx = context.Background()
		now = time.Now().Unix()

		key      = generateKey(t)
		otherKey = generateKey(t)
		srv      = newIssuer(t, key)

		errAssertion = errors.New("assertion failed")

		baseClaims = map[string]any{
			"iss":                   srv.URL,
			"aud":                   testClientID,
			"sub":                   "repo:octo-org/octo-repo:ref:refs/heads/main",
			"jti":                   testJTI,
			"iat":                   now,
			"nbf":                   now,
			"exp":                   now + 300,
			"repository":            testRepository,
			"repository_id":         "74",
			"repository_owner":      testOwner,
			"repository_owner_id":   "65",
			"ref":                   testRef,
			"repo_property_team":    "platform",
			"repository_visibility": testVisibility,
		}

		baseWant = Claims{
			Issuer:               srv.URL,
			Subject:              "repo:octo-org/octo-repo:ref:refs/heads/main",
			Audience:             Audience{testClientID},
			ID:                   testJTI,
			ExpiresAt:            now + 300,
			IssuedAt:             now,
			NotBefore:            now,
			Ref:                  testRef,
			Repository:           testRepository,
			RepositoryID:         74,
			RepositoryOwner:      testOwner,
			RepositoryOwnerID:    65,
			RepositoryVisibility: testVisibility,
			RepoProperties:       map[string]string{"team": "platform"},
		}
	)

	for _, tt := range []struct {
		name          string
		haveKey       *rsa.PrivateKey
		haveClaims    map[string]any
		haveAssertion ClaimsAssertion
		want          *Claims
		errFn         testutil.ErrorAssertion
	}{
		{
			name:       "valid token without assertion",
			haveKey:    key,
			haveClaims: baseClaims,
			want:       &baseWant,
			errFn:      testutil.NoError(),
		},
		{
			name:       "valid token with passing assertion",
			haveKey:    key,
			haveClaims: baseClaims,
			haveAssertion: func(c Claims) error {
				if c.RepositoryOwner != testOwner {
					return errAssertion
				}

				return nil
			},
			want:  &baseWant,
			errFn: testutil.NoError(),
		},
		{
			name:          "claims assertion error is propagated",
			haveKey:       key,
			haveClaims:    baseClaims,
			haveAssertion: func(Claims) error { return errAssertion },
			errFn:         testutil.ErrorCause(errAssertion),
		},
		{
			name:       "wrong audience",
			haveKey:    key,
			haveClaims: withClaims(baseClaims, map[string]any{"aud": "other"}),
			errFn:      errorContains("expected audience"),
		},
		{
			name:    "wrong issuer",
			haveKey: key,
			haveClaims: withClaims(
				baseClaims,
				map[string]any{"iss": "https://token.actions.githubusercontent.com"},
			),
			errFn: errorContains("issued by a different provider"),
		},
		{
			name:       "expired token",
			haveKey:    key,
			haveClaims: withClaims(baseClaims, map[string]any{"exp": now - 600}),
			errFn:      errorContains("token is expired"),
		},
		{
			name:       "bad signature",
			haveKey:    otherKey,
			haveClaims: baseClaims,
			errFn:      errorContains("failed to verify signature"),
		},
		{
			name:       "invalid github claim",
			haveKey:    key,
			haveClaims: withClaims(baseClaims, map[string]any{"run_id": "abc"}),
			errFn:      errorContains("decode claims"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewVerifier(
				ctx,
				VerifierConfig{
					IssuerURL:       srv.URL,
					ClientID:        testClientID,
					ClaimsAssertion: tt.haveAssertion,
				},
			)

			require.NoError(t, err)

			tok, err := v.Verify(ctx, signToken(t, tt.haveKey, tt.haveClaims))

			tt.errFn(t, err)
			assert.Equal(t, tt.want, tokenClaims(tok))
		})
	}
}

func TestNewVerifierDiscoveryError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())

	defer srv.Close()

	v, err := NewVerifier(
		context.Background(),
		VerifierConfig{IssuerURL: srv.URL, ClientID: testClientID},
	)

	assert.Nil(t, v)
	assert.ErrorContains(t, err, "discover oidc provider")
}
