# Setup runbook: a new factory from scratch

This builds the same setup as the original factory: a self-hosted Scion Hub on a GCE VM, a sandbox project, a build
project and an eval project, heartbeats, and the agent-status sync. Plan for half a day; most of it is waiting on
builds and IAM.

Each step ends with a **check**. Don't go on until it passes.

## 0. What you need

- Two GCP projects with billing: `HUB_PROJECT` (the Hub VM and its IAP proxy) and `SANDBOX_PROJECT` (everything the
  agents build and deploy). Keeping them apart means agents can't damage the Hub.
- A GitHub repository (`GITHUB_REPO`) and a personal access token with repo scope (contents, issues, pull requests).
  Every agent acts as the token's owner (L14).
- Locally: `gcloud`, `gh`, `python3`, `bash`. Owner rights on the sandbox; rights to create a VM and an IAP proxy in
  the Hub project.
- A backlog: issues with acceptance criteria, labeled with the build project's slug and `wave-1` ... `wave-N`; a
  `knowledge-plane/INDEX.md` (or similar) in the repo that points to the specs and ADRs. The agents are only as good as
  these issues.

## 1. Configure

```bash
cd docs/scion
cp config/factory.env.example config/factory.env
$EDITOR config/factory.env         # git-ignored; never commit it
```

**Check:** `scripts/render-templates.py --out /tmp/tpl` finishes without "missing values".

## 2. Hub VM and Scion server

Follow Scion's own guides for the VM and the server:
[Hub Setup on GCE](https://github.com/GoogleCloudPlatform/scion/blob/main/docs-site/src/content/docs/hosted/single-node/hub-setup-gce.md)
and [Authentication](https://github.com/GoogleCloudPlatform/scion/blob/main/docs-site/src/content/docs/hosted/single-node/auth.md).
What this factory used (deployed via Scion's `scripts/single-node-vm/deploy.sh`):

- VM `e2-standard-4`, 200 GB disk, Ubuntu 22.04, with a dedicated service account (scope `cloud-platform`). In
  `HUB_PROJECT` that SA has `roles/aiplatform.user` (all model calls), `roles/logging.logWriter`,
  `roles/monitoring.metricWriter` and `roles/cloudtrace.agent` (telemetry).
- Built with `deploy.sh`: `machine_size: small`, `disk_size_gb: 200`, `admin_email: $PO_EMAIL`,
  `release_channel: nightly`, `update_policy: auto`, `container_images.source: build`.
- Docker, git, `sqlite3` and python3 on the VM; the Hub runs as OS user `HUB_OS_USER`, which owns `~/.scion` and can
  use Docker.
- systemd unit `scion-hub.service` running
  `scion server start --foreground --hosted --enable-hub --enable-web --web-port 8080 --enable-runtime-broker`.
- IAP in front of the web UI (`server.auth.mode: proxy`), and `server.hub.agent_endpoint` set to the VM's internal
  address so agents reach the Hub directly.
- `~/.scion/settings.yaml` as in [`config/hub-settings.example.yaml`](config/hub-settings.example.yaml): every harness
  on Vertex AI with the VM's identity, the PO in `admin_emails`.

**Check:** you can sign in to the web UI through IAP as the PO, `scripts/run-on-hub.sh scripts/scion-admin.py projects`
lists the projects, and `scripts/run-on-hub.sh scripts/scion-admin.py experiments` succeeds (exercising the admin JWT minting path).

## 3. Hub experiments

Turn off the beta terminal experiment, which breaks the web terminal button (L2):

```bash
scripts/run-on-hub.sh scripts/scion-admin.py experiments --set web.terminal_workspace=false
```

**Check:** `scripts/run-on-hub.sh scripts/scion-admin.py experiments` shows `web.terminal_workspace enabled=False`.

## 4. Sandbox

```bash
DRY_RUN=1 scripts/setup-sandbox.sh --with-spanner --with-bigquery     # read the plan
read -rs GITHUB_TOKEN && export GITHUB_TOKEN                          # paste the PAT; not echoed, not in history
scripts/setup-sandbox.sh --with-spanner --with-bigquery
unset GITHUB_TOKEN
```

Grant up front everything the backlog will obviously need (runtime identities, data stores, secrets): an IAM decision
that waits for the PO stalls the factory for hours (L17).

**Check:** run it again; every step reports "exists" and nothing new is created.

## 5. GitHub

```bash
gh label create "$BUILD_PROJECT" -R "$GITHUB_REPO" --description "build factory backlog" --force
gh label create "$EVAL_PROJECT" -R "$GITHUB_REPO" --description "eval factory backlog" --force
for n in 1 2 3 4; do gh label create "wave-$n" -R "$GITHUB_REPO" --force; done
```

(Run these with the values from `factory.env`, e.g. after `set -a; . config/factory.env; set +a`.)

Set up branch protection rulesets on the repository to prevent accidental force-pushes or deletions on active factory branches:

```bash
gh api -X POST repos/$GITHUB_REPO/rulesets \
  --input - << 'EOF'
{
  "name": "factory-branch-protection",
  "target": "branch",
  "enforcement": "active",
  "conditions": {
    "ref_name": {
      "include": [
        "refs/heads/main",
        "refs/heads/eval-dashboard"
      ],
      "exclude": []
    }
  },
  "rules": [
    {"type": "deletion"},
    {"type": "non_fast_forward"}
  ]
}
EOF
```

Store the PAT in the Hub as secret `GITHUB_TOKEN`. Note: Hub secrets default to injection mode `as_needed`, which skips injecting them into container environments unless explicitly declared. To ensure agents receive the token, set injection mode to **Always** (C2):
- In Web UI: Project → Settings → Secrets → New Secret (`GITHUB_TOKEN`, Injection mode: **Always**).
- Or via CLI:
  ```bash
  scion hub secret set --project="$BUILD_PROJECT" GITHUB_TOKEN "$GITHUB_PAT"
  scion hub secret update --project="$BUILD_PROJECT" GITHUB_TOKEN --injection-mode always
  scion hub secret set --project="$EVAL_PROJECT" GITHUB_TOKEN "$GITHUB_PAT"
  scion hub secret update --project="$EVAL_PROJECT" GITHUB_TOKEN --injection-mode always
  ```

**Check:** `gh api repos/$GITHUB_REPO/rulesets` lists `factory-branch-protection`, and after step 8, `gh auth status` and `git ls-remote origin` succeed inside an agent
(`docker exec -u scion <project>--lead sh -c 'cd /workspace && gh auth status && git ls-remote origin | head -1'`).

## 6. Hub projects and the deployer identity

Create the two Hub-managed projects in the web UI, named `BUILD_PROJECT` and `EVAL_PROJECT`. Without a git remote they
get one shared folder each, `~/.scion/projects/<slug>`, mounted as `/workspace` in all their agents (L8).
*(Alternative: create git-backed projects with `gitRemote`, `workspaceMode: "shared"`, and `githubToken`, which clones automatically and sets up branch tracking).*

Make each folder a checkout of the repository on its branch, as the Hub OS user on the VM (the variables are your
`factory.env` values; on the VM, type them in or export them first):

```bash
cd ~/.scion/projects/$BUILD_PROJECT && git init -b $BUILD_BRANCH && git remote add origin https://github.com/$GITHUB_REPO.git
git -c credential.helper='!f(){ echo username=x-access-token; echo "password=$GITHUB_TOKEN"; }; f' fetch origin
git checkout -B $BUILD_BRANCH origin/$BUILD_BRANCH      # skip for an empty repository
mkdir -p .git/info && printf '.scion/\n.agents/\nscratchpad/\n*.env\n.env*\n' >> .git/info/exclude
# same for $EVAL_PROJECT with $EVAL_BRANCH
```
*(Writing to `.git/info/exclude` instead of `.gitignore` keeps the working tree clean and prevents permanent untracked/dirty conflicts between agents).*

Register the deployer identity in each project (via Web UI Project Settings → Service Accounts, or from a CLI context linked to that project), so deploy agents can run
as it (Scion `assign` mode):

```bash
scion project service-accounts add "$DEPLOYER_SA_NAME@$SANDBOX_PROJECT.iam.gserviceaccount.com" \
  --gcp-project "$SANDBOX_PROJECT" --name "Sandbox deployer"
scion project service-accounts list          # note the ID; `verify <id>` re-checks impersonation
```

**Check:** `scion project service-accounts list` shows the SA as verified in both projects.

## 7. Patch the Antigravity hooks (before any Antigravity agent starts)

```bash
scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py --apply
scripts/run-on-hub.sh scripts/patch-antigravity-hooks.py          # check
```

Without this, Antigravity agents can't run any tool while Hub telemetry is on (L6).
The `--apply` command patches all `provision.py` files (under `harness-configs`, `workspace-storage`, and agent homes), all `hooks.json` files, stages an executable pre-start hook `30-project-custom` across all agent homes, and creates/activates `agy-telemetry-off` in `project_pre_start_hooks` in the Hub database via the API, ensuring hooks stay patched across container starts and upgrades (C1).

**Check:** the check run prints `provision.py: patched`, `30-project-custom: patched`, `project <slug>: hook agy-telemetry-off active`, and exits 0.

## 8. Templates and agents

```bash
scripts/push-templates.py --project "$BUILD_PROJECT" --roles lead,backend,frontend,reviewer,deployer          # dry run
scripts/push-templates.py --project "$BUILD_PROJECT" --roles lead,backend,frontend,reviewer,deployer --apply
scripts/push-templates.py --project "$EVAL_PROJECT" \
  --roles eval-lead,factory-analyst,search-evaluator,ui-craftsman,eval-deployer --apply
```

([templates/README.md](templates/README.md#uploading-to-the-hub) explains what it does; reviewer uses Gemini 2.5 Pro via `model: google/gemini-2.5-pro`). Then start the agents as the
admin, in each project (assigning the deployer service account registered in Step 6 via `--service-account <id>`):

```bash
scion start lead -t lead "Read your instructions, then run your wake-up steps."
scion start backend -t backend "Stand by for briefs from lead."
scion start frontend -t frontend "Stand by for briefs from lead."
scion start reviewer -t reviewer "Stand by for review requests from lead."
scion start deployer -t deployer --service-account <deployer-sa-id> "Stand by for deploy requests from lead."
# eval project: eval-lead, factory-analyst, search-evaluator, ui-craftsman, and eval-deployer --service-account <id>
```

Agents started by the admin send their notifications to the admin, not to the leads (L5); the heartbeats in step 9
cover that. (Having each lead start its own team would route notifications to it, but whether a lead may assign the
deployer identity was not tested.)

**Check:** `scripts/run-on-hub.sh scripts/factory-status.sh --panes 10` shows 10 running agents, each terminal at a
prompt, the OpenCode one on "Gemini 2.5 Pro" (L3).

## 9. Heartbeats

Create them as the admin, so they keep working if a lead is recreated (L11):

```bash
hb="$(scripts/render-templates.py --file config/heartbeat-message.txt)"
scripts/run-on-hub.sh scripts/scion-admin.py schedule create --project "$BUILD_PROJECT" \
  --name "$BUILD_PROJECT-lead-heartbeat" --cron "*/15 * * * *" --agent lead --text "$hb"
scripts/run-on-hub.sh scripts/scion-admin.py schedule create --project "$EVAL_PROJECT" \
  --name "$EVAL_PROJECT-lead-heartbeat" --cron "*/15 * * * *" --agent eval-lead --text "$hb"
```
*(Note: on re-run, `schedule create` returns HTTP 409 if a schedule by that name already exists; use `schedule update` instead).*

**Check:** after the next quarter hour, `schedule list` shows `runs=1 errors=0`, and the leads' terminals show they
ran the heartbeat steps.

## 10. Agent-status sync and backups

Install the Hub -> Spanner status sync:

```bash
scripts/install-sync-cron.sh --dry-run      # read what it will do
scripts/install-sync-cron.sh
```

Install daily consistent Hub backups and cron:

```bash
scripts/run-on-hub.sh scripts/backup-hub.sh --install-cron
```

**Check:** `crontab -l` on the Hub VM lists both `sync_factory_agents.py` and `backup-hub.sh`, the sync first run prints `synced 10 agents`, `~/.scion/factory-sync.log` gains a line every minute, and `~/backups` contains initial database and state archives.

## 11. Start the work

Tell each lead to go, with the rules of autonomy spelled out:

```bash
scripts/run-on-hub.sh scripts/scion-admin.py message --project "$BUILD_PROJECT" --agent lead --text \
  "PO -> lead: GO. Work the backlog on your own in wave order, at most 2 issues in flight, evidence on every issue. Escalate to me only IAM, cost or scope decisions."
```

**Check:** within one heartbeat the lead comments a plan on the first issue and briefs a worker. From here on, follow
[operations-runbook.md](operations-runbook.md).
