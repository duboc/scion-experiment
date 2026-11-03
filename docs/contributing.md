# How to contribute

This guide explains the contribution workflow for the Intelligent Product Search (`psearch`) repository and the Scion multi-agent software factory.
All documentation in this repository follows Google Developer documentation style and the **ASD-STE100** (Simplified Technical English) standard.

> [!IMPORTANT]
> Do not write secrets, tokens, or real Google Cloud project IDs in tracked files.
> Store local environment values in `docs/scion/config/factory.env` and use `<PROJECT_ID>` in documentation.

## Before you begin

### Sign the Contributor License Agreement

You must sign a [Contributor License Agreement](https://cla.developers.google.com/about) (CLA) before you submit code to this project.
You or your employer keep the copyright to your contribution.
The CLA grants permission to use and distribute your contributions as part of the project.

Open <https://cla.developers.google.com/> to view your current agreements or to sign a new CLA.

### Follow the community guidelines

This project follows [Google's Open Source Community Guidelines](https://opensource.google/conduct/).

### Keep secrets and project IDs out of Git

Follow these security rules for every change:

1. Never commit API keys, service account JSON keys, or bearer tokens.
2. Replace real Google Cloud project IDs with `<PROJECT_ID>` or `<HUB_PROJECT_ID>` in tracked files.
3. Put environment-specific values in `docs/scion/config/factory.env` (which `.gitignore` excludes).

## Shared workspace and file ownership

In the Scion software factory, all agents share a single `/workspace` Git worktree.
Each role owns specific paths to prevent merge conflicts:

| Role | Owned paths and scope |
|---|---|
| `lead` | Architecture Decision Records (`knowledge-plane/adr/`), OpenAPI contracts (`knowledge-plane/contracts/`), issue triage, and commit slot scheduling |
| `backend` | Go serving handlers, models, services, Spanner migrations (`src/psearch/serving/`), root `README.md`, and `docs/USAGE.md` |
| `frontend` | React UI (`src/application/ui/`), embedded storefront (`internal/api/routes.go`), storefront tests (`internal/api/storefront_test.go`), and `docs/contributing.md` |
| `reviewer` | Code review, contract verification, and test validation |
| `deployer` | Pre-deploy `sqlcheck` gate, Cloud Spanner DDL execution, and Cloud Run deployment |

Follow these rules when you work in `/workspace`:

1. Edit only the files assigned to your role.
2. Do not run `git stash`, `git reset`, or `git checkout --` on shared files.
3. Do not commit generated build artifacts such as `src/application/ui/build/`.

## Commit slot protocol

Only one agent can hold the commit slot at a time.
Wait for `lead` to grant an exclusive commit slot before you stage or commit files.

1. Run your local test suite and verify that all checks pass.
2. Send a `DONE` message to `lead` with your modified file list and test results.
3. Wait until `lead` replies with `COMMIT SLOT GRANTED`.
4. Stage only the explicit file paths that belong to your issue:

   ```bash
   git add <file-1> <file-2>
   ```

5. Check the staged file list before you create the commit:

   ```bash
   git diff --cached --name-only
   ```

6. Commit your changes with an issue-tagged Conventional Commit message:

   ```bash
   git commit -m "feat(ui): search telemetry beacons and analytics drawer (#43)"
   ```

7. Fetch `origin` and confirm that `origin/main` is an ancestor of `HEAD`:

   ```bash
   git fetch origin
   git merge-base --is-ancestor origin/main HEAD
   ```

   > [!WARNING]
   > Do not run `git pull --rebase` when another agent has unstaged changes in `/workspace`.
   > Stop and notify `lead` if `git merge-base` fails.

8. Push your commit to `origin/main`:

   ```bash
   git push origin main
   ```

9. Send the new commit SHA to `lead` immediately to release the commit slot.

### Format commit messages

Use Conventional Commits with the GitHub issue number at the end of the subject line:

```text
<type>(<scope>): <imperative summary> (#<issue-number>)
```

Supported commit types:

- `feat`: Adds a new backend or UI capability.
- `fix`: Fixes a bug, query error, or regression.
- `docs`: Updates documentation or Architecture Decision Records.
- `test`: Adds or updates automated tests.
- `chore`: Updates build scripts, dependencies, or configuration.

## Pre-deploy verification and the real-Spanner `sqlcheck` gate

Every change must pass local verification and the real-Spanner `sqlcheck` gate before deployment.

### 1. Run unit and storefront tests

1. Run the Go test suite from `src/psearch/serving/`:

   ```bash
   go test ./...
   ```

2. Run the UI production build from `src/application/ui/` if you changed frontend files:

   ```bash
   npm run build
   ```

### 2. Run the real-Spanner `sqlcheck` gate

Unit tests with mock database drivers do not validate Cloud Spanner SQL dialect rules.
Before every deployment, run the `sqlcheck` gate against the live Cloud Spanner instance:

1. Verify that all SQL queries use valid Cloud Spanner functions (such as `JSON_VALUE` for JSON columns).
2. Confirm that every `FORCE_INDEX` hint matches the exact index definition in `migrations/`.
3. Include `AND <column> IS NOT NULL` in the `WHERE` clause whenever a query forces a `NULL_FILTERED` index.
4. Execute `sqlcheck` against the target Cloud Spanner database and verify zero query plan errors.

## Review and deployment workflow

After your commit lands on `origin/main`, the factory completes these steps:

1. `reviewer` inspects the commit diff against the OpenAPI contract (`knowledge-plane/contracts/biggy-search-openapi.yaml`) and the corresponding ADR.
2. `deployer` runs the real-Spanner `sqlcheck` gate against the staging or production database.
3. `deployer` deploys the new `psearch-serving` revision to Cloud Run.
4. `deployer` runs smoke tests and latency checks against the live service URL.
5. `lead` closes the GitHub issue after `reviewer` and `deployer` report `PASS`.