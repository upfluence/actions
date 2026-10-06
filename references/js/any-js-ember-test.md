# any-js-ember-test

Public testing-only reusable workflow for Ember apps and libraries. Current tests are required; additional dependency scenarios are advisory by default. The `any-js-lint_and_test.yml` wrapper runs lint once and calls this workflow. Both workflows can be called from public or private repositories.

## Opt in

1. Add `ember-try` 4.x as a development dependency.
2. Create `config/ember-try.js` using the configuration below, retaining any existing local scenarios. It reads the scenario and variant JSON from environment variables. No helper package, adapter, or variants file is required.

   ```js
   'use strict';

   const variants = JSON.parse(process.env.EMBER_TEST_VARIANTS || '[{"name":"default","command":"pnpm test:ember"}]');

   module.exports = async function () {
     return {
       packageManager: 'pnpm',
       command: 'pnpm test',
       scenarios: JSON.parse(process.env.EMBER_TRY_SCENARIOS || '[]').flatMap((scenario) =>
         variants.map((variant) => ({
           ...scenario,
           name: `${scenario.name}-${variant.name}`,
           command: variant.command
         }))
       )
     };
   };
   ```

3. Pass test variants directly when calling the workflow, keeping your existing lint/typecheck jobs:

   ```yaml
   jobs:
     test:
       needs: [typecheck] # Optional
       uses: upfluence/actions/.github/workflows/any-js-ember-test.yml@master
       secrets: inherit
       with:
         timeout-minutes: 12
         test-variants: |
           [
             { "name": "default", "command": "pnpm test:default" },
             { "name": "wednesday", "command": "pnpm test:wednesday" }
           ]
   ```

For existing callers that omit the input, variants default to one entry running `pnpm test:ember`. Both the testing-only workflow and the combined wrapper run tests on every caller event, including branch pushes and release tags. The wrapper's existing master/main skip applies only to lint.

## Shared targets

Set the org-level Actions variable `EMBER_TRY_SCENARIOS` to a JSON array such as the following. A repo variable with the same name overrides the org default. No configured scenarios means current tests only.

```json
[
  {
    "name": "ember-4.12",
    "env": {
      "EMBER_OPTIONAL_FEATURES": "{\"jquery-integration\":false}"
    },
    "npm": {
      "devDependencies": {
        "ember-source": "~4.12.3",
        "ember-cli": "~4.12.3"
      }
    }
  },
  {
    "name": "ember-4.12-data-4.12",
    "env": {
      "EMBER_OPTIONAL_FEATURES": "{\"jquery-integration\":false}"
    },
    "npm": {
      "devDependencies": {
        "ember-source": "~4.12.3",
        "ember-cli": "~4.12.3",
        "ember-data": "4.12.8"
      }
    }
  }
]
```

The first scenario leaves Ember Data's existing dependency specification unchanged; the second sets it to exactly `4.12.8`. `EMBER_OPTIONAL_FEATURES` is itself JSON encoded as a string inside the scenario's `env`.

Each entry is a native ember-try scenario (`name`, `npm`, and optional `env`). Target-version optional features belong in its `env`, including the JSON-stringified `EMBER_OPTIONAL_FEATURES`. Commands and brand/filter details belong to repository test variants, not shared targets. Add Ember 5/6/7 entries to the variable as needed; adjust Node/tooling for newer targets where necessary.

**Changing the org variable affects all opted-in repos on their next run.** Keep secrets out of the JSON.

## Matrices

The current job uses `fromJSON` for the variants array. The compatibility job uses two axes directly:

```yaml
matrix:
  scenario: ${{ fromJSON(inputs.scenarios || vars.EMBER_TRY_SCENARIOS || '[]') }}
  variant: ${{ fromJSON(inputs.test-variants) }}
```

GitHub builds the cross-product. Two targets and two variants produce four compatibility jobs, plus two required current jobs. Compatibility runs on every caller event and is skipped only when no scenarios are configured. `continue-on-error` applies only to compatibility; `fail-fast: false` keeps variants independent.

Job labels distinguish the baseline from additional targets: `Current tests (default)` versus `Extra scenario: ember-4.12 (default)`. The target and variant stay in a single label rather than slash-separated segments, so they remain visible in GitHub's job sidebar.

There is no preparation job, file reader, custom matrix builder, schema validator, or generated runtime config. Compatibility exposes `test-variants` as `EMBER_TEST_VARIANTS` and runs the matching scenario in the repo's `config/ember-try.js`, such as `ember-4.12-wednesday`.

## Inputs

| Input | Default | Effect |
| --- | --- | --- |
| `node-version` | `20` | Node version for test jobs. |
| `timeout-minutes` | `10` | Timeout per test job. |
| `scenarios` | empty | Overrides `vars.EMBER_TRY_SCENARIOS`; `[]` disables additional targets. |
| `test-variants` | default / test:ember | JSON array of test variant names and commands. |
| `compatibility-blocking` | `false` | Require additional targets to pass. |
| `ember-try-working-directory` | `.` | Ember CLI project directory for compatibility tests, relative to the repo root. |

Use `secrets: inherit`: private registry installs use the caller's `PAT_TOKEN` and `FONTAWESOME_NPM_AUTH_TOKEN`. The workflow also uses `vars.PNPM_VERSION` and a committed pnpm lockfile. Authentication remains available during ember-try reinstalls. No SSH-agent setup or SSH key is required by this workflow. Check required-status names when adopting or changing the workflow; no deployment occurs in compatibility runners.

GitHub SSH-style Git dependencies are rewritten to HTTPS and authenticated with
`PAT_TOKEN`, which must have read access to those repositories. A Git credential
helper reads the token from the job environment rather than embedding it in URLs
or saving its value in Git configuration. Checkout does not persist its repo-scoped
credentials, and the Git setup remains active for ember-try's dependency reinstalls.
Missing or insufficient credentials fail without interactive prompts.

## V2 addons with a separate test app

Point `ember-try-working-directory` at the test app and keep build orchestration in
the caller's package scripts. Installs and current tests still run at the repository
root; only the compatibility command changes directory.

For a pnpm workspace like hypermarket, whose root `test` script already builds the
addon and runs the test app:

```yaml
jobs:
  tests:
    uses: upfluence/actions/.github/workflows/any-js-ember-test.yml@master
    secrets: inherit
    with:
      ember-try-working-directory: packages/test-app
      test-variants: |
        [{ "name": "default", "command": "pnpm -w test" }]
```

Use the configuration snippet above in `packages/test-app/config/ember-try.js`.
The command must work both from the repository root (current tests) and the test app
(compatibility tests). `pnpm -w test` selects the workspace root in either case and
ensures the addon is rebuilt after ember-try changes the target dependencies.
Keep any shell chaining inside package scripts, not in ember-try's command string.

## Locally

Export `EMBER_TRY_SCENARIOS` with the same JSON shown under **Shared targets**, then set `EMBER_TEST_VARIANTS` to the caller workflow's input and run:

```sh
pnpm test # Current dependencies
export EMBER_TEST_VARIANTS='[{"name":"default","command":"pnpm test:default"},{"name":"wednesday","command":"pnpm test:wednesday"}]'
pnpm exec ember try:each
pnpm exec ember try:one ember-4.12-wednesday
pnpm exec ember try:one ember-4.12-data-4.12-default
```

Set `EMBER_TEST_VARIANTS` locally to the same JSON supplied by the caller workflow. If unset, it defaults to the same single `test:ember` variant. Invalid JSON is reported by `JSON.parse` or GitHub's `fromJSON`; there is no separate validation framework.
