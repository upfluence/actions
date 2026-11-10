package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/upfluence/errors"
	"github.com/upfluence/pkg/testutil"

	"github.com/upfluence/actions/pkg/toolkit"
)

const (
	keyARN = "arn:aws:kms:eu-west-1:123:key/abc"
	owner  = "upfluence"
	token  = "ghs_xxx"
)

var (
	errKMS = errors.New("kms failure")

	errMalformedJWT = errors.New("malformed jwt")
	errJWTHeader    = errors.New("unexpected jwt header")
)

func newConfig(fn func(*config)) config {
	c := config{AppID: "42", InstallationID: 7, KMSKeyARN: keyARN}

	fn(&c)

	return c
}

type fakeKMSClient struct {
	key *rsa.PrivateKey
	err error

	in *kms.SignInput
}

func (c *fakeKMSClient) Sign(_ context.Context, in *kms.SignInput, _ ...func(*kms.Options)) (*kms.SignOutput, error) {
	c.in = in

	if c.err != nil {
		return nil, c.err
	}

	sig, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, in.Message)

	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &kms.SignOutput{Signature: sig}, nil
}

func verifyAppJWT(pub *rsa.PublicKey, raw string) (jwtClaims, error) {
	var (
		parts = strings.Split(raw, ".")
		enc   = base64.RawURLEncoding

		claims jwtClaims
	)

	if len(parts) != 3 {
		return claims, errMalformedJWT
	}

	sig, err := enc.DecodeString(parts[2])

	if err != nil {
		return claims, errors.WithStack(err)
	}

	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))

	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return claims, errors.WithStack(err)
	}

	if header, _ := enc.DecodeString(parts[0]); string(header) != jwtHeader {
		return claims, errJWTHeader
	}

	buf, err := enc.DecodeString(parts[1])

	if err != nil {
		return claims, errors.WithStack(err)
	}

	return claims, json.Unmarshal(buf, &claims)
}

type recordedRequest struct {
	Path    string
	Version string
	Claims  jwtClaims
	Body    installationTokenRequest
}

// newGitHubServer verifies the app JWT against the public key before
// recording the installation token request.
func newGitHubServer(t *testing.T, pub *rsa.PublicKey, status int, body string) (*httptest.Server, *recordedRequest) {
	t.Helper()

	var rr recordedRequest

	srv := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := verifyAppJWT(
				pub,
				strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
			)

			if !assert.NoError(t, err) {
				w.WriteHeader(http.StatusUnauthorized)

				return
			}

			assert.NoError(t, json.NewDecoder(r.Body).Decode(&rr.Body))

			rr.Path = r.URL.Path
			rr.Version = r.Header.Get("X-GitHub-Api-Version")
			rr.Claims = claims

			w.WriteHeader(status)
			io.WriteString(w, body)
		}),
	)

	t.Cleanup(srv.Close)

	return srv, &rr
}

func TestConfigInstallationToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	var (
		now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

		created = installationToken{Token: token, ExpiresAt: now.Add(time.Hour)}

		request = func(itr installationTokenRequest) *recordedRequest {
			return &recordedRequest{
				Path:    "/app/installations/7/access_tokens",
				Version: "2022-11-28",
				Claims: jwtClaims{
					IssuedAt:  now.Add(-time.Minute).Unix(),
					ExpiresAt: now.Add(9 * time.Minute).Unix(),
					Issuer:    "42",
				},
				Body: itr,
			}
		}
	)

	createdBody, err := json.Marshal(created)
	require.NoError(t, err)

	for _, tt := range []struct {
		name       string
		haveConfig config
		haveKMSErr error
		haveStatus int
		haveBody   string

		want        installationToken
		wantRequest *recordedRequest
		assertErr   testutil.ErrorAssertion
	}{
		{
			name: "scoped token",
			haveConfig: newConfig(func(c *config) {
				c.Repositories = []string{"upfluence/api", "web"}
				c.Permissions = map[string]string{
					"issues":        permissionRead,
					"pull-requests": "Write",
				}
			}),
			haveStatus: http.StatusCreated,
			haveBody:   string(createdBody),
			want:       created,
			wantRequest: request(
				installationTokenRequest{
					Repositories: []string{"api", "web"},
					Permissions: map[string]string{
						"issues":        permissionRead,
						"pull_requests": permissionWrite,
					},
				},
			),
			assertErr: testutil.NoError(),
		},
		{
			name:        "unscoped token",
			haveConfig:  newConfig(func(*config) {}),
			haveStatus:  http.StatusCreated,
			haveBody:    string(createdBody),
			want:        created,
			wantRequest: request(installationTokenRequest{}),
			assertErr:   testutil.NoError(),
		},
		{
			name: "foreign repository",
			haveConfig: newConfig(func(c *config) {
				c.Repositories = []string{"foreign/repo"}
			}),
			wantRequest: &recordedRequest{},
			assertErr:   testutil.ErrorCause(errForeignRepository),
		},
		{
			name: "invalid permission level",
			haveConfig: newConfig(func(c *config) {
				c.Permissions = map[string]string{"checks": "none"}
			}),
			wantRequest: &recordedRequest{},
			assertErr:   testutil.ErrorCause(errInvalidPermission),
		},
		{
			name:        "kms failure",
			haveConfig:  newConfig(func(*config) {}),
			haveKMSErr:  errKMS,
			wantRequest: &recordedRequest{},
			assertErr:   testutil.ErrorCause(errKMS),
		},
		{
			name: "github rejection",
			haveConfig: newConfig(func(c *config) {
				c.Permissions = map[string]string{"workflows": permissionWrite}
			}),
			haveStatus: http.StatusUnprocessableEntity,
			haveBody:   `{"message":"The permissions requested are not granted to this installation."}`,
			wantRequest: request(
				installationTokenRequest{
					Permissions: map[string]string{"workflows": permissionWrite},
				},
			),
			assertErr: testutil.ErrorCause(errUnexpectedStatus),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var (
				kcl     = fakeKMSClient{key: key, err: tt.haveKMSErr}
				srv, rr = newGitHubServer(t, &key.PublicKey, tt.haveStatus, tt.haveBody)
			)

			tt.haveConfig.APIURL = srv.URL + "/"

			got, err := tt.haveConfig.installationToken(
				t.Context(),
				owner,
				&kcl,
				srv.Client(),
				now,
			)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantRequest, rr)

			if kcl.in != nil {
				assert.Equal(
					t,
					&kms.SignInput{
						KeyId:            aws.String(keyARN),
						Message:          kcl.in.Message,
						MessageType:      types.MessageTypeDigest,
						SigningAlgorithm: types.SigningAlgorithmSpecRsassaPkcs1V15Sha256,
					},
					kcl.in,
				)
			}
		})
	}
}

func TestConfigRegion(t *testing.T) {
	for _, tt := range []struct {
		name       string
		haveConfig config

		want      string
		assertErr testutil.ErrorAssertion
	}{
		{
			name:       "region of the kms key",
			haveConfig: newConfig(func(*config) {}),
			want:       "eu-west-1",
			assertErr:  testutil.NoError(),
		},
		{
			name:       "missing app id",
			haveConfig: newConfig(func(c *config) { c.AppID = "" }),
			assertErr:  testutil.ErrorCause(errMissingAppID),
		},
		{
			name:       "missing installation id",
			haveConfig: newConfig(func(c *config) { c.InstallationID = 0 }),
			assertErr:  testutil.ErrorCause(errMissingInstallationID),
		},
		{
			name:       "missing kms key arn",
			haveConfig: newConfig(func(c *config) { c.KMSKeyARN = "" }),
			assertErr:  testutil.ErrorCause(errMissingKMSKeyARN),
		},
		{
			name:       "invalid kms key arn",
			haveConfig: newConfig(func(c *config) { c.KMSKeyARN = "alias/foo" }),
			assertErr: func(tb testing.TB, err error) {
				assert.ErrorContains(tb, err, "parse kms key arn")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.haveConfig.region()

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConfigEnvironment(t *testing.T) {
	c := config{EnvNames: []string{"GH_TOKEN", "GITHUB_TOKEN"}}

	assert.Equal(
		t,
		toolkit.Environment{
			Variables: []toolkit.Variable{
				{Key: "GH_TOKEN", Value: token},
				{Key: "GITHUB_TOKEN", Value: token},
			},
			Secrets: []string{token},
		},
		c.environment(installationToken{Token: token}),
	)
}

func TestNormalizeRepositories(t *testing.T) {
	for _, tt := range []struct {
		name      string
		haveOwner string
		haveRepos []string

		want      []string
		assertErr testutil.ErrorAssertion
	}{
		{
			name:      "names and owner references",
			haveOwner: owner,
			haveRepos: []string{"man", " Upfluence/helm-charts ", ""},
			want:      []string{"man", "helm-charts"},
			assertErr: testutil.NoError(),
		},
		{
			name:      "unknown owner accepts any reference",
			haveRepos: []string{"other/cli"},
			want:      []string{"cli"},
			assertErr: testutil.NoError(),
		},
		{
			name:      "foreign owner",
			haveOwner: owner,
			haveRepos: []string{"other/man"},
			assertErr: testutil.ErrorCause(errForeignRepository),
		},
		{
			name:      "empty name",
			haveOwner: owner,
			haveRepos: []string{"upfluence/"},
			assertErr: testutil.ErrorCause(errEmptyRepository),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeRepositories(tt.haveOwner, tt.haveRepos)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizePermissions(t *testing.T) {
	for _, tt := range []struct {
		name            string
		havePermissions map[string]string

		want      map[string]string
		assertErr testutil.ErrorAssertion
	}{
		{
			name:      "empty",
			assertErr: testutil.NoError(),
		},
		{
			name: "workflow syntax",
			havePermissions: map[string]string{
				"Pull-Requests":  " WRITE ",
				"administration": "Admin",
			},
			want: map[string]string{
				"pull_requests":  permissionWrite,
				"administration": permissionAdmin,
			},
			assertErr: testutil.NoError(),
		},
		{
			name:            "invalid level",
			havePermissions: map[string]string{"statuses": "none"},
			assertErr:       testutil.ErrorCause(errInvalidPermission),
		},
		{
			name:            "empty name",
			havePermissions: map[string]string{" ": permissionRead},
			assertErr:       testutil.ErrorCause(errEmptyPermissionKey),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePermissions(tt.havePermissions)

			tt.assertErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
