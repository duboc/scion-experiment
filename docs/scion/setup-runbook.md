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
What this factory used:

- VM `e2-standard-4`, 200 GB disk, Ubuntu 22.04, with a dedicated service account (scope `cloud-platform`). In
  `HUB_PROJECT` that SA has `roles/aiplatform.user` (all model calls), `roles/logging.logWriter`,
  `roles/monitoring.metricWriter` and `roles/cloudtrace.agent` (telemetry).
- Docker, git, `sqlite3` and python3 on the VM; the Hub runs as OS user `HUB_OS_USER`, which owns `~/.scion` and can
  use Docker.
- systemd unit `scion-hub.service` running
  `scion server start --foreground --hosted --enable-hub --enable-web --web-port 8080 --enable-runtime-broker`.
- IAP in front of the web UI (`server.auth.mode: proxy`), and `server.hub.agent_endpoint` set to the VM's internal
  address so agents reach the Hub directly.
- `~/.scion/settings.yaml` as in [`config/hub-settings.example.yaml`](config/hub-settings.example.yaml): every harness
  on Vertex AI with the VM's identity, the PO in `admin_emails`.

**Check:** you can sign in to the web UI through IAP as the PO, and `scripts/run-on-hub.sh scripts/scion-admin.py
projects` lists the `global` project.

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
gh label create "$BUILD_PROJECT" -R "$GITHUB_REPO" --description "build factory backlog"
gh label create "$EVAL_PROJECT" -R "$GITHUB_REPO" --description "eval factory backlog"
for n in 1 2 3 4; do gh label create "wave-$n" -R "$GITHUB_REPO"; done
```

(Run these with the values from `factory.env`, e.g. after `set -a; . config/factory.env; set +a`.)

Store the PAT in the Hub so agents get it as `GITHUB_TOKEN`; Scion's git credential helper and `gh` wrapper use it.
The web UI (Settings, Secrets) keeps it out of your shell history; the CLI equivalent is
`scion hub secret set GITHUB_TOKEN <token>`. User-scope secrets only reach agents started by other agents when set
with `--allow-progeny`; a project-scope secret avoids the question.

**Check:** after step 8, `gh auth status` and `git ls-remote origin` succeed inside an agent
(`docker exec -u scion <project>--lead sh -c 'cd /workspace && gh auth status && git ls-remote origin | head -1'`).

## 6. Hub projects and the deployer identity

Create the two Hub-managed projects in the web UI, named `BUILD_PROJECT` and `EVAL_PROJECT`. Without a git remote they
get one shared folder each, `~/.scion/projects/<slug>`, mounted as `/workspace` in all their agents (L8). Decide now:
a project's git remote can't be added later.

Make each folder a checkout of the repository on its branch, as the Hub OS user on the VM (the variables are your
`factory.env` values; on the VM, type them in or export them first):

```bash
cd ~/.scion/projects/$BUILD_PROJECT && git init -b $BUILD_BRANCH && git remote add origin https://github.com/$GITHUB_REPO.git
git fetch origin && git checkout -B $BUILD_BRANCH origin/$BUILD_BRANCH      # skip for an empty repository
printf '.scion/\n.agents/\nscratchpad/\n*.env\n.env*\n' >> .gitignore
# same for $EVAL_PROJECT with $EVAL_BRANCH
```

For a private repository, `git fetch` on the VM needs a credential; the agents themselves get theirs from the Hub
secret (step 5).

Register the deployer identity in each project (from a CLI context linked to that project), so deploy agents can run
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

**Check:** the check run prints `provisioner: patched` and exits 0.

## 8. Templates and agents

```bash
scripts/push-templates.py --project "$BUILD_PROJECT" --roles lead,backend,frontend,reviewer,deployer          # dry run
scripts/push-templates.py --project "$BUILD_PROJECT" --roles lead,backend,frontend,reviewer,deployer --apply
scripts/push-templates.py --project "$EVAL_PROJECT" \
  --roles eval-lead,factory-analyst,search-evaluator,ui-craftsman,eval-deployer --apply
```

([templates/README.md](templates/README.md#uploading-to-the-hub) explains what it does.) Then start the agents as the
admin, in each project:

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

**Check:** after the next quarter hour, `schedule list` shows `runs=1 errors=0`, and the leads' terminals show they
ran the heartbeat steps.

## 10. Agent-status sync for the dashboard

```bash
scripts/install-sync-cron.sh --dry-run      # read what it will do
scripts/install-sync-cron.sh
```

**Check:** the first run prints `synced 10 agents`, and `~/.scion/factory-sync.log` on the VM gains a line every
minute.

## 11. Start the work

Tell each lead to go, with the rules of autonomy spelled out:

```bash
scripts/run-on-hub.sh scripts/scion-admin.py message --project "$BUILD_PROJECT" --agent lead --text \
  "PO -> lead: GO. Work the backlog on your own in wave order, at most 2 issues in flight, evidence on every issue. Escalate to me only IAM, cost or scope decisions."
```

**Check:** within one heartbeat the lead comments a plan on the first issue and briefs a worker. From here on, follow
[operations-runbook.md](operations-runbook.md).
