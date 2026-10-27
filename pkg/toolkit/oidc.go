package toolkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/upfluence/errors"
)

var (
	ErrOIDCTokenUnavailable = errors.New(
		"oidc token unavailable, is the id-token: write permission granted?",
	)

	errUnexpectedStatus = errors.New("unexpected status")
	errEmptyOIDCToken   = errors.New("oidc token response has no value")
)

type OIDCTokenFetcher interface {
	// Fetch returns an OIDC token for the given audience, an empty audience
	// falls back to GitHub's default (the repository owner URL).
	Fetch(context.Context, string) (string, error)
}

type oidcConfig struct {
	URL   string
	Token string
}

type oidcTokenFetcher struct {
	url    string
	token  string
	client *http.Client
}

func newOIDCTokenFetcher(oc oidcConfig) *oidcTokenFetcher {
	return &oidcTokenFetcher{
		url:    oc.URL,
		token:  oc.Token,
		client: http.DefaultClient,
	}
}

func (otf *oidcTokenFetcher) Fetch(ctx context.Context, audience string) (string, error) {
	if otf.url == "" || otf.token == "" {
		return "", ErrOIDCTokenUnavailable
	}

	u, err := url.Parse(otf.url)

	if err != nil {
		return "", errors.Wrap(err, "parse oidc token request url")
	}

	if audience != "" {
		q := u.Query()

		q.Set("audience", audience)
		u.RawQuery = q.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)

	if err != nil {
		return "", errors.Wrap(err, "build oidc token request")
	}

	req.Header.Set("Authorization", "bearer "+otf.token)
	req.Header.Set("Accept", "application/json")

	resp, err := otf.client.Do(req)

	if err != nil {
		return "", errors.Wrap(err, "request oidc token")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.Wrapf(
			errUnexpectedStatus,
			"request oidc token (status %d)",
			resp.StatusCode,
		)
	}

	var payload struct {
		Value string `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", errors.Wrap(err, "decode oidc token response")
	}

	if payload.Value == "" {
		return "", errEmptyOIDCToken
	}

	return payload.Value, nil
}
