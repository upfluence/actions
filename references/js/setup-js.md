# setup-js

Public composite action that prepares a JavaScript job in one step: it checks the repository out, authenticates Git against GitHub over HTTPS, sets up pnpm and Node, and installs the dependencies with `pnpm install --frozen-lockfile`. It replaces the checkout, `webfactory/ssh-agent`, `pnpm/action-setup`, `actions/setup-node` and `pnpm install` steps that were repeated across the JavaScript workflows. It can be called from public or private repositories.

## Usage

```yaml
jobs:
  typecheck:
    runs-on: ubuntu-24.04
    steps:
      - uses: upfluence/actions/setup-js@master
        with:
          github-token: ${{ secrets.PAT_TOKEN }}
          node-version: ${{ vars.NODE_VERSION }}
          pnpm-version: ${{ vars.PNPM_VERSION }}
      - run: pnpm tsc --noEmit
```

The action does the checkout, so call it before any step that needs the repository. Steps that do not need it (a JDK, apt packages, a scanner download) can run before it.

## Inputs

| Input | Default | Effect |
| --- | --- | --- |
| `github-token` | required | PAT used as `GITHUB_TOKEN` by `pnpm install` and by the Git credential helper. It must be able to read the private GitHub dependencies of the project. |
| `node-version` | `20` | Node version to install. An empty value falls back to `20`. |
| `pnpm-version` | empty | pnpm version to install. When empty, the `packageManager` field of `package.json` is used. |
| `registry-url` | empty | npm registry configured by `actions/setup-node`, for jobs that publish. |
| `ref` | empty | Branch, tag or SHA to check out. Defaults to the ref or SHA of the triggering event. |
| `fetch-depth` | `1` | Number of commits to fetch; `0` fetches the whole history. |
| `checkout-token` | `github.token` | Token used by the checkout. |
| `persist-credentials` | `false` | Keep the checkout token in the Git config for the rest of the job. |

## Git authentication

SSH-style GitHub dependency URLs (`git@github.com:`, `ssh://git@github.com/` and `git+ssh://git@github.com/`) are rewritten to HTTPS and authenticated with `github-token`. A Git credential helper reads the token from the environment of the process asking for credentials, so it is never embedded in a URL or written to the Git config. No SSH agent or SSH key is needed. Missing or insufficient credentials fail without an interactive prompt.

`github-token` must be a PAT that can read the private repositories the lockfile refers to; the token of the workflow cannot. Without access, the install fails with `Repository not found` while cloning the dependency.

The token is only exported to the install step. A later step that clones or installs private GitHub dependencies, like the `ember try` reinstalls in [`any-js-ember-test`](any-js-ember-test.md), has to export `GITHUB_TOKEN` itself, for instance from the workflow-level `env`.

## pnpm version

Composite actions cannot read the `vars` context, so the action cannot look up the `PNPM_VERSION` organization variable itself. Pass it as `pnpm-version: ${{ vars.PNPM_VERSION }}`.

## Node version

The same limitation applies to the `NODE_VERSION` organization variable: pass it as `node-version: ${{ vars.NODE_VERSION }}`. A variable that is unset, or not visible to the repository (an organization variable limited to private repositories is empty in public ones), gives an empty value. The runner only applies an input default when the input is not passed at all, so the action handles the empty case itself and falls back to `20` rather than skipping the Node installation. A workflow that pins its own version can pass it instead, for instance `${{ env.NODE_VERSION }}`.

## Environment

Variables referenced by the project's `.npmrc`, such as `FONTAWESOME_NPM_AUTH_TOKEN`, are not inputs. Keep them in the workflow or job `env`: the install inherits them.

## Checkout

The checkout is always done, with `actions/checkout`. `ref`, `fetch-depth`, `checkout-token` and `persist-credentials` are passed through. The other checkout options (another repository or path, submodules, LFS) are not supported, and jobs needing them keep their own steps.

Credentials are not persisted by default. A job that pushes commits sets `persist-credentials: 'true'`, with a token that is allowed to push:

```yaml
- uses: upfluence/actions/setup-js@master
  with:
    github-token: ${{ secrets.PAT_TOKEN }}
    checkout-token: ${{ secrets.PAT_TOKEN }}
    persist-credentials: 'true'
    fetch-depth: 0
    node-version: ${{ vars.NODE_VERSION }}
    pnpm-version: ${{ vars.PNPM_VERSION }}
```

A pull request check that must run on the head commit instead of the merge commit sets `ref: ${{ github.event.pull_request.head.sha }}`.

## Publishing

`registry-url` makes `actions/setup-node` write an npm configuration that reads `NODE_AUTH_TOKEN`. Set it on the publishing step:

```yaml
- uses: upfluence/actions/setup-js@master
  with:
    github-token: ${{ secrets.PAT_TOKEN }}
    node-version: ${{ vars.NODE_VERSION }}
    pnpm-version: ${{ vars.PNPM_VERSION }}
    registry-url: https://npm.pkg.github.com
- run: npm publish
  env:
    NODE_AUTH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

## Migrating from ssh-agent

1. Replace the `webfactory/ssh-agent`, checkout, `pnpm/action-setup`, `actions/setup-node` and `pnpm install` steps with the action.
2. Pass the PAT as `github-token`; the `SSH_PRIVATE_KEY` secret is no longer used.
3. Pass the versions the replaced steps used: `${{ vars.PNPM_VERSION }}` for pnpm, and `${{ vars.NODE_VERSION }}` or the workflow's own value for Node.
4. Remove any step rewriting `https://github.com/` to `git@github.com:`; it conflicts with the HTTPS rewrite.
5. Keep the workflow-level `env` the install relies on, for instance `FONTAWESOME_NPM_AUTH_TOKEN`.

## Actions used

| Action | Version |
| --- | --- |
| actions/checkout | v6 |
| pnpm/action-setup | v4 |
| actions/setup-node | v6 |
