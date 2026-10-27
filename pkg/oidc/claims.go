package oidc

import (
	"encoding/json"
	"strings"

	"github.com/upfluence/errors"
)

const repoPropertyPrefix = "repo_property_"

type ClaimsAssertion func(Claims) error

type Audience []string

func (a *Audience) UnmarshalJSON(buf []byte) error {
	var s string

	if err := json.Unmarshal(buf, &s); err == nil {
		*a = Audience{s}

		return nil
	}

	var ss []string

	if err := json.Unmarshal(buf, &ss); err != nil {
		return errors.Wrap(err, "decode audience")
	}

	*a = ss

	return nil
}

// Claims are the claims of a GitHub Actions OIDC token, see
// https://docs.github.com/en/actions/reference/security/oidc#oidc-token-claims
type Claims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  Audience `json:"aud"`
	ID        string   `json:"jti"`
	ExpiresAt int64    `json:"exp"`
	IssuedAt  int64    `json:"iat"`
	NotBefore int64    `json:"nbf"`

	Actor                string `json:"actor"`
	ActorID              int64  `json:"actor_id,string"`
	BaseRef              string `json:"base_ref"`
	CheckRunID           int64  `json:"check_run_id,string"`
	Enterprise           string `json:"enterprise"`
	EnterpriseID         int64  `json:"enterprise_id,string"`
	Environment          string `json:"environment"`
	EnvironmentNodeID    string `json:"environment_node_id"`
	EventName            string `json:"event_name"`
	HeadRef              string `json:"head_ref"`
	IssuerScope          string `json:"issuer_scope"`
	JobWorkflowRef       string `json:"job_workflow_ref"`
	JobWorkflowSHA       string `json:"job_workflow_sha"`
	Ref                  string `json:"ref"`
	RefProtected         bool   `json:"ref_protected,string"`
	RefType              string `json:"ref_type"`
	Repository           string `json:"repository"`
	RepositoryID         int64  `json:"repository_id,string"`
	RepositoryOwner      string `json:"repository_owner"`
	RepositoryOwnerID    int64  `json:"repository_owner_id,string"`
	RepositoryVisibility string `json:"repository_visibility"`
	RunAttempt           int64  `json:"run_attempt,string"`
	RunID                int64  `json:"run_id,string"`
	RunNumber            int64  `json:"run_number,string"`
	RunnerEnvironment    string `json:"runner_environment"`
	SHA                  string `json:"sha"`
	Workflow             string `json:"workflow"`
	WorkflowRef          string `json:"workflow_ref"`
	WorkflowSHA          string `json:"workflow_sha"`

	// RepoProperties holds the repo_property_* claims, keyed by property name
	// without the prefix.
	RepoProperties map[string]string `json:"-"`
}

func (c *Claims) UnmarshalJSON(buf []byte) error {
	type claims Claims

	var (
		cc   claims
		raws map[string]json.RawMessage
	)

	if err := json.Unmarshal(buf, &cc); err != nil {
		return errors.Wrap(err, "decode claims")
	}

	if err := json.Unmarshal(buf, &raws); err != nil {
		return errors.Wrap(err, "decode raw claims")
	}

	for k, raw := range raws {
		name, ok := strings.CutPrefix(k, repoPropertyPrefix)

		if !ok {
			continue
		}

		var v string

		if err := json.Unmarshal(raw, &v); err != nil {
			return errors.Wrapf(err, "decode %q claim", k)
		}

		if cc.RepoProperties == nil {
			cc.RepoProperties = make(map[string]string)
		}

		cc.RepoProperties[name] = v
	}

	*c = Claims(cc)

	return nil
}
