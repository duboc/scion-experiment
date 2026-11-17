# Contribute to PSearch

This guide explains the contribution workflow for the PSearch repository and the Scion multi-agent software factory.
This document follows the Google developer documentation style guide and the ASD-STE100 Simplified Technical English standard.

> [!IMPORTANT]
> Do not write secrets, tokens, or real Google Cloud project IDs in tracked files.
> Store local environment values in `docs/scion/config/factory.env` and use `PROJECT_ID` in documentation.

## Before you begin

### Sign the Contributor License Agreement

You must sign a [Contributor License Agreement](https://cla.developers.google.com/about) (CLA) before you submit code to this project.
You or your employer retain the copyright to your contribution.
The CLA grants permission to use and distribute your contributions as part of the project.

Open the [Google Contributor License Agreement portal](https://cla.developers.google.com/) to view your current agreements or to sign a new CLA.

### Follow the community guidelines

This project follows [Google's Open Source Community Guidelines](https://opensource.google/conduct/).

### Keep secrets and project IDs out of Git

Follow these security rules for every change:

1. Never commit API keys, service account JSON keys, or bearer tokens.
2. Replace real Google Cloud project IDs with `PROJECT_ID` or `HUB_PROJECT_ID` in tracked files.
3. Put environment-specific values in `docs/scion/config/factory.env` (which `.gitignore` excludes).

## Understand workspace and file ownership

In the Scion software factory, all agents share a single `/workspace` Git worktree.
Each role owns specific paths to prevent merge conflicts:

| Role | Owned paths and scope |
|---|---|
| `lead` | Architecture Decision Records (`knowledge-plane/adr/`), OpenAPI contracts (`knowledge-plane/contracts/`), issue triage, and commit slot scheduling |
| `backend` | Go serving packages (`src/psearch/serving/` except `routes.go` and `storefront_test.go`), `internal/api/routes_wave6.go`, Spanner migrations (`src/iac/migrations/`), root `README.md`, `docs/USAGE.md`, `docs/contributing.md`, `tooling/loadtest/`, and `tooling/enrichment/` |
| `frontend` | React UI (`src/application/ui/`), `src/application/ui/README.md`, embedded storefront (`src/psearch/serving/internal/api/routes.go`), and storefront tests (`src/psearch/serving/internal/api/storefront_test.go`) |
| `reviewer` | Independent code review in private worktrees, contract verification, and `REVIEW.md` |
| `deployer` | Pre-deploy `cmd/sqlcheck` gate, Cloud Spanner DDL execution, Cloud Run deployment, and `DEPLOYMENT.md` |

Follow these rules when you work in `/workspace`:

1. Edit only the files assigned to your role.
2. Do not run `git stash`, `git reset`, or `git checkout --` on shared files.
3. Do not commit generated build artifacts such as `src/application/ui/build/`.

## Follow the commit slot protocol

Only one agent can hold the commit slot at a time.
Wait for `lead` to grant an exclusive commit slot before you stage or commit files.

1. Run your local test suite and verify that all checks pass.
2. Send a `DONE` message to `lead` with your modified file list and test results.
3. Wait until `lead` replies with `COMMIT SLOT GRANTED`.
4. Stage only the explicit file paths that belong to your issue:

   ```bash
   git add FILE_PATH_1 FILE_PATH_2
   ```

5. Check the staged file list before you create the commit:

   ```bash
   git diff --cached --name-only
   ```

6. Commit your changes with an issue-tagged Conventional Commit message:

   ```bash
   git commit -m "feat(search): per-search deadline and admission control (#66)"
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
TYPE(SCOPE): IMPERATIVE_SUMMARY (#ISSUE_NUMBER)
```

Supported commit types:

- `feat`: Adds a new backend or UI capability.
- `fix`: Fixes a bug, query error, or regression.
- `perf`: Improves query latency, caching, or resource usage.
- `docs`: Updates documentation or Architecture Decision Records.
- `test`: Adds or updates automated tests.
- `chore`: Updates build scripts, dependencies, or configuration.

## Run pre-deploy verification and `cmd/sqlcheck`

Every change must pass local verification and the real-Spanner `cmd/sqlcheck` gate before deployment.

### 1. Run unit, race, and forbidden-ID tests

1. Run static analysis, the race detector, and the forbidden-identifier guard from `src/psearch/serving/`:

   ```bash
   cd src/psearch/serving
   go vet ./... && FORBIDDEN_ID_PATTERN="YOUR_PRIVATE_PROJECT_REGEX" go test -race -count=1 ./...
   ```

2. Run the UI production build from `src/application/ui/` if you changed frontend files:

   ```bash
   cd src/application/ui
   npm run build
   ```

### 2. Run the real-Spanner `cmd/sqlcheck` gate

Unit tests in memory mode do not validate Cloud Spanner SQL planner rules.
Before every deployment, run `cmd/sqlcheck` against the live Cloud Spanner database:

1. Add a test case in `src/psearch/serving/cmd/sqlcheck/main.go` for every new or modified SQL query.
2. Confirm that every `FORCE_INDEX` hint matches the index definition in `src/iac/migrations/`.
3. Include `AND COLUMN_NAME IS NOT NULL` in the `WHERE` clause whenever a query forces a `NULL_FILTERED` or `VECTOR` index.
4. Execute `cmd/sqlcheck` against the target Cloud Spanner database and verify zero SQL errors:

   ```bash
   cd src/psearch/serving
   PROJECT_ID=PROJECT_ID \
   SPANNER_INSTANCE_ID=psearch-instance \
   SPANNER_DATABASE_ID=psearch-db \
   ACTIVE_EMBEDDING_VERSION=v2 \
   GENAI_LOCATION=global \
   TEXT_FOLDING=true \
   go run ./cmd/sqlcheck
   ```

## Review and deployment workflow

After your commit lands on `origin/main`, the factory completes these steps:

1. `reviewer` inspects the commit diff against the [OpenAPI contract](../knowledge-plane/contracts/biggy-search-openapi.yaml) and the corresponding ADR.
2. `deployer` runs `cmd/sqlcheck` against the live Cloud Spanner database.
3. `deployer` deploys the new `psearch-serving` revision to Cloud Run from a clean `git archive` snapshot.
4. `deployer` runs smoke tests and latency checks against the live service URL (keeping load tests at 2 rps or less unless approved by the Product Owner).
5. `lead` closes the GitHub issue after `reviewer` and `deployer` report `PASS`.
