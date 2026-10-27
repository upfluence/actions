package oidc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/upfluence/pkg/testutil"
)

func errorContains(msg string) testutil.ErrorAssertion {
	return func(t testing.TB, err error) {
		assert.ErrorContains(t, err, msg)
	}
}

func TestClaimsUnmarshalJSON(t *testing.T) {
	for _, tt := range []struct {
		name     string
		haveJSON string
		want     Claims
		errFn    testutil.ErrorAssertion
	}{
		{
			name: "full token",
			haveJSON: `{
				"jti": "example-id",
				"sub": "repo:octo-org/octo-repo:environment:prod",
				"environment": "prod",
				"environment_node_id": "EN_kwDOABCD",
				"aud": "https://github.com/octo-org",
				"ref": "refs/heads/main",
				"ref_protected": "true",
				"sha": "example-sha",
				"repository": "octo-org/octo-repo",
				"repository_owner": "octo-org",
				"actor_id": "12",
				"repository_visibility": "private",
				"repository_id": "74",
				"repository_owner_id": "65",
				"enterprise": "octo-enterprise",
				"enterprise_id": "3",
				"run_id": "1234",
				"run_number": "10",
				"run_attempt": "2",
				"check_run_id": "5678",
				"runner_environment": "github-hosted",
				"actor": "octocat",
				"workflow": "example-workflow",
				"workflow_ref": "octo-org/octo-repo/.github/workflows/ci.yml@refs/heads/main",
				"workflow_sha": "example-workflow-sha",
				"head_ref": "",
				"base_ref": "",
				"event_name": "workflow_dispatch",
				"ref_type": "branch",
				"job_workflow_ref": "octo-org/octo-automation/.github/workflows/oidc.yml@refs/heads/main",
				"job_workflow_sha": "example-job-workflow-sha",
				"issuer_scope": "enterprise",
				"iss": "https://token.actions.githubusercontent.com",
				"nbf": 1632492967,
				"exp": 1632493867,
				"iat": 1632493567
			}`,
			want: Claims{
				Issuer:               "https://token.actions.githubusercontent.com",
				Subject:              "repo:octo-org/octo-repo:environment:prod",
				Audience:             Audience{"https://github.com/octo-org"},
				ID:                   testJTI,
				ExpiresAt:            1632493867,
				IssuedAt:             1632493567,
				NotBefore:            1632492967,
				Actor:                "octocat",
				ActorID:              12,
				CheckRunID:           5678,
				Enterprise:           "octo-enterprise",
				EnterpriseID:         3,
				Environment:          "prod",
				EnvironmentNodeID:    "EN_kwDOABCD",
				EventName:            "workflow_dispatch",
				IssuerScope:          "enterprise",
				JobWorkflowRef:       "octo-org/octo-automation/.github/workflows/oidc.yml@refs/heads/main",
				JobWorkflowSHA:       "example-job-workflow-sha",
				Ref:                  testRef,
				RefProtected:         true,
				RefType:              "branch",
				Repository:           testRepository,
				RepositoryID:         74,
				RepositoryOwner:      testOwner,
				RepositoryOwnerID:    65,
				RepositoryVisibility: testVisibility,
				RunAttempt:           2,
				RunID:                1234,
				RunNumber:            10,
				RunnerEnvironment:    "github-hosted",
				SHA:                  "example-sha",
				Workflow:             "example-workflow",
				WorkflowRef:          "octo-org/octo-repo/.github/workflows/ci.yml@refs/heads/main",
				WorkflowSHA:          "example-workflow-sha",
			},
			errFn: testutil.NoError(),
		},
		{
			name: "repo properties",
			haveJSON: `{
				"sub": "repo:my-org/my-repo:ref:refs/heads/main",
				"repo_property_business_unit": "payments",
				"repo_property_regions": "us-east-1,eu-west-1"
			}`,
			want: Claims{
				Subject: "repo:my-org/my-repo:ref:refs/heads/main",
				RepoProperties: map[string]string{
					"business_unit": "payments",
					"regions":       "us-east-1,eu-west-1",
				},
			},
			errFn: testutil.NoError(),
		},
		{
			name:     "audience list",
			haveJSON: `{"aud": ["foo", "bar"]}`,
			want:     Claims{Audience: Audience{"foo", "bar"}},
			errFn:    testutil.NoError(),
		},
		{
			name:     "invalid audience",
			haveJSON: `{"aud": 42}`,
			errFn:    errorContains("decode audience"),
		},
		{
			name:     "non numeric id",
			haveJSON: `{"actor_id": "abc"}`,
			errFn:    errorContains("decode claims"),
		},
		{
			name:     "empty numeric id",
			haveJSON: `{"repository_id": ""}`,
			errFn:    errorContains("decode claims"),
		},
		{
			name:     "non string repo property",
			haveJSON: `{"repo_property_pci_compliant": true}`,
			errFn:    errorContains(`decode "repo_property_pci_compliant" claim`),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got Claims

			err := json.Unmarshal([]byte(tt.haveJSON), &got)

			tt.errFn(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
