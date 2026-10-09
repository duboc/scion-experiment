# Playbook: GitHub Issue Software Factory Workflow (`psearch`)

1. **Trigger:** Product Owner (`duboc`) instructs `@lead`: `"Deliver GitHub Issue #N"`.
2. **Plan:** `@lead` runs `gh issue view N --repo duboc/scion-experiment` and reads the linked ADR in `/workspace/knowledge-plane/adr/`.
3. **Build (Parallel):**
   - `@backend` (`opencode`) implements Go + Spanner changes in `/workspace/src/psearch/serving/` and unit tests (`go test ./...`), without running `git commit`.
   - `@frontend` (`antigravity`) implements UI changes in `/workspace/src/application/ui/`, without running `git commit`.
4. **Serialized Commits:** `@lead` grants the commit slot to `@backend` first, then `@frontend`, pushing each commit to `origin/main`.
5. **Review:** `@reviewer` (`hermes`) runs `go test ./...`, verifies ADR compliance, updates `/workspace/REVIEW.md`, commits and pushes.
6. **Deploy & Close Issue:** `@deployer` (`claude`, `agent-deployer@riojucu-sandbox.iam.gserviceaccount.com`) applies any Spanner DDL on `psearch-db`, deploys `psearch-serving` to Cloud Run in `riojucu-sandbox`, verifies live endpoints with `curl`, commits `/workspace/DEPLOYMENT.md` with `Closes #N`, pushes to `origin/main`, and posts a verification summary comment on GitHub Issue `#N` (`gh issue comment N ...`).
