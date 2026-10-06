# any-js-lint_and_test

Public reusable workflow that runs lint once and delegates tests to [`any-js-ember-test`](any-js-ember-test.md).
Tests run on every caller event; the existing master/main skip applies only to lint.
Apps that already own their lint/typecheck jobs can call the testing-only workflow directly.

## Usage

```yaml
jobs:
  lint-and-test:
    uses: upfluence/actions/.github/workflows/any-js-lint_and_test.yml@master
    secrets: inherit
    with:
      test-variants: |
        [
          { "name": "default", "command": "pnpm test:default" },
          { "name": "wednesday", "command": "pnpm test:wednesday" }
        ]
```

The wrapper calls `./.github/workflows/any-js-ember-test.yml` from the same repository revision. No private reusable workflow or SSH-agent setup is involved in the lint-and-test path.

## Inputs

| Name | Type | Default | Effect |
| --- | --- | --- | --- |
| node-version | string | '20' | Sets the Node version. |
| timeout-minutes | number | 10 | Sets the timeout per lint/test job. |
| scenarios | string | empty | Additional scenario JSON; defaults to `vars.EMBER_TRY_SCENARIOS`. |
| test-variants | string | default / test:ember | JSON array of test variant names and commands. |
| compatibility-blocking | boolean | false | Require additional targets to pass. |
| ember-try-working-directory | string | . | Ember CLI project directory for compatibility tests. |

See the testing workflow reference for shared variable setup, registry secrets,
default/Wednesday variants, local execution, and required-check migration notes.

## Actions used

| Action | Version |
| --- | --- |
| actions/checkout | v6 |
| pnpm/action-setup | v4 |
| actions/setup-node | v6 |
