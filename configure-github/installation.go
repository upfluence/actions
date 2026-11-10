package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/upfluence/errors"
)

const (
	permissionRead  = "read"
	permissionWrite = "write"
	permissionAdmin = "admin"
)

var (
	errUnexpectedStatus   = errors.New("unexpected status")
	errForeignRepository  = errors.New("repository is not owned by the repository owner")
	errInvalidPermission  = errors.New("invalid permission level, expected read, write or admin")
	errEmptyRepository    = errors.New("empty repository name")
	errEmptyPermissionKey = errors.New("empty permission name")
)

type installationTokenRequest struct {
	Repositories []string          `json:"repositories,omitempty"`
	Permissions  map[string]string `json:"permissions,omitempty"`
}

type installationToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func createInstallationToken(ctx context.Context, cl *http.Client, apiURL string, installationID int64, jwt string, itr installationTokenRequest) (installationToken, error) {
	body, err := json.Marshal(itr)

	if err != nil {
		return installationToken{}, errors.Wrap(err, "encode request")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf(
			"%s/app/installations/%d/access_tokens",
			strings.TrimSuffix(apiURL, "/"),
			installationID,
		),
		bytes.NewReader(body),
	)

	if err != nil {
		return installationToken{}, errors.Wrap(err, "build request")
	}

	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := cl.Do(req)

	if err != nil {
		return installationToken{}, errors.Wrap(err, "request installation token")
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

		return installationToken{}, errors.Wrapf(
			errUnexpectedStatus,
			"request installation token (status %d): %s",
			resp.StatusCode,
			bytes.TrimSpace(msg),
		)
	}

	var t installationToken

	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return installationToken{}, errors.Wrap(err, "decode installation token")
	}

	return t, nil
}

// normalizeRepositories accepts both name and owner/name references, the
// installation token API only takes names within the installation owner.
func normalizeRepositories(owner string, rs []string) ([]string, error) {
	var res []string

	for _, r := range rs {
		r = strings.TrimSpace(r)

		if r == "" {
			continue
		}

		o, name, ok := strings.Cut(r, "/")

		if !ok {
			res = append(res, r)

			continue
		}

		if owner != "" && !strings.EqualFold(o, owner) {
			return nil, errors.Wrapf(errForeignRepository, "%q", r)
		}

		if name == "" {
			return nil, errors.Wrapf(errEmptyRepository, "%q", r)
		}

		res = append(res, name)
	}

	return res, nil
}

// normalizePermissions accepts the workflow permissions syntax
// (pull-requests) along with the API one (pull_requests).
func normalizePermissions(ps map[string]string) (map[string]string, error) {
	if len(ps) == 0 {
		return nil, nil
	}

	res := make(map[string]string, len(ps))

	for k, v := range ps {
		k = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "-", "_")
		v = strings.ToLower(strings.TrimSpace(v))

		if k == "" {
			return nil, errEmptyPermissionKey
		}

		switch v {
		case permissionRead, permissionWrite, permissionAdmin:
		default:
			return nil, errors.Wrapf(errInvalidPermission, "%s=%s", k, v)
		}

		res[k] = v
	}

	return res, nil
}
