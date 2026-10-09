# Scion Enterprise Banking PDLC Platform — Code Gap Analysis & Engineering Roadmap

This document maps Scion's current codebase (`pkg/hub/`, `pkg/ent/schema/`, `pkg/store/`, `pkg/sciontool/`, `harnesses/`, and `web/`) against the **6 Critical Platform Requirements** and **22 Operational & Governance PDLC Patterns** required for an enterprise-grade banking software engineering platform.

---

## 1. Executive Summary: What Exists vs. What Must Be Built in Code

| Platform Pillar | Current State in Scion Codebase | Platform Code Gap | Target Code Modules to Expand |
| :--- | :--- | :--- | :--- |
| **1. Accountability Model** | Hub RBAC (`none`, `readonly`, `baseline`, `full`), [`MutationAudit`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/mutationaudit.go), [`DecisionAudit`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/decisionaudit.go), and advisory [`sciontool status ask_user`](file:///Users/duboc/local/projects/scion/cmd/sciontool/commands/status.go#L80-L85). | No enforced, database-backed **Human Sign-Off / Approval Gate** entity binding a human reviewer's identity to specific artifact SHA-256 hashes before allowing an agent to proceed. | `pkg/ent/schema/approvalrequest.go`<br>`pkg/hub/handlers_approvals.go`<br>`cmd/sciontool/commands/approval.go`<br>`web/src/components/approvals/` |
| **2. Unattended Runtimes** | Detached containers, [`Schedule`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/schedule.go) (cron & one-shot timers for single agents), `max_duration` / `max_turns` limits, and outbound [`LifecycleHook`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/lifecyclehook.go). | No **Batch Campaign / Multi-Repo Sweep Engine** (bounded-concurrency fan-out across N repos/modules) and no **Inbound Event/Webhook Triggers** (for Alertmanager, CVE feeds, or PR events). | `pkg/ent/schema/campaign.go`<br>`pkg/hub/campaign_runner.go`<br>`pkg/hub/handlers_triggers.go` |
| **3. Knowledge Plane** | Per-template `mcp_servers` auto-translation ([`harnesses/claude/provision.py`](file:///Users/duboc/local/projects/scion/harnesses/claude/provision.py#L106-L117), [`harnesses/antigravity/provision.py`](file:///Users/duboc/local/projects/scion/harnesses/antigravity/provision.py#L47-L56)), shared volumes, and the Hub [`Skill`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/skill.go) registry (`skill://`, `gh://`, `gcp-skill://`). | No first-class **Hub Knowledge Corpus Registry** (`KnowledgeSource`) that manages enterprise corpora (ADRs, OpenAPI contracts, LGPD/BACEN rules, COBOL copybooks, Design Tokens), tracks index versions, and auto-injects them into pods. | `pkg/ent/schema/knowledgesource.go`<br>`pkg/hub/handlers_knowledge.go`<br>`pkg/runtimebroker/start_context.go` |
| **4. Continuous Observability** | Embedded `sciontool` OTLP collector (`localhost:4317`) parsing native token usage for `claude`, `codex`, and `copilot` ([`pkg/sciontool/telemetry/usage.go`](file:///Users/duboc/local/projects/scion/pkg/sciontool/telemetry/usage.go)) and exporting to GCP Cloud Monitoring/Logging. | 1. **AGY Token Gap:** [`harnesses/antigravity/provision.py`](file:///Users/duboc/local/projects/scion/harnesses/antigravity/provision.py#L161-L171) is calls-only (does not extract token counts from AGY transcripts).<br>2. **No Hub FinOps Rollup:** [`store.Agent`](file:///Users/duboc/local/projects/scion/pkg/store/models.go#L56-L59) only stores `CurrentTurns` and `CurrentModelCalls`, not token counts or cost USD. | `pkg/sciontool/telemetry/usage.go`<br>`pkg/store/models.go`<br>`pkg/ent/schema/agent.go`<br>`pkg/sciontool/hub/client.go` |
| **5. Lifecycle Provenance** | Input tracking: template `content_hash`, [`SkillInjection`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/skill_injection.go), and immutable creator/parent [`Agent.Ancestry`](file:///Users/duboc/local/projects/scion/pkg/store/models.go#L98). | No **Output & Regulatory Lineage Ledger** (`ProvenanceAttestation`) linking `Requirement/Epic ID → Input Hashes → Agent Lineage → Git Commit/PR → Test Evidence → Security/LGPD Approvals`. | `pkg/ent/schema/provenanceattestation.go`<br>`pkg/hub/handlers_provenance.go`<br>`cmd/sciontool/commands/provenance.go` |
| **6. Multi-Harness Capability** | Declarative harness configs (`claude`, `antigravity`, `gemini-cli`, `opencode`, `codex`), `model_aliases` (`small`, `medium`, `large`), and manual agent reincarnation (`ReincarnationState`). | Static harness selection at agent creation; no **Dynamic Task-Class Router** or **Automated Quota/SLA Fallback Chain** (`claude ↔ antigravity ↔ gemini-cli`) on HTTP 429/503 errors. | `pkg/hub/harness_router.go`<br>`pkg/hub/httpdispatcher.go`<br>`pkg/config/settings_v1.go` |

---

## 2. Detailed Code Findings by Requirement

### 2.1 Accountability Model (Human-in-the-Loop Sign-Off & Intent Attribution)
* **Current Code Inspection:**
  * [`pkg/ent/schema/mutationaudit.go`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/mutationaudit.go#L33-L81) records `actor_principal_id`, `actor_credential_id`, `target_type`, `target_id`, `before_summary`, `after_summary`, and `correlation_id` for Hub control-plane changes.
  * [`cmd/sciontool/commands/status.go`](file:///Users/duboc/local/projects/scion/cmd/sciontool/commands/status.go#L80-L129) updates the agent's status to `WAITING_FOR_INPUT` (`ask_user`) or `BLOCKED` (`blocked`), which sends a [`hub.StatusUpdate`](file:///Users/duboc/local/projects/scion/pkg/sciontool/hub/client.go) to the Hub.
* **Why Code Expansion Is Needed:**
  * `sciontool status ask_user` is a status indicator, not an authorization gate. Any chat reply resumes the agent, and there is no verification that the responder holds a specific role (e.g., Tech Lead, Security Reviewer, DBA) or that the exact file hash of `/workspace/DESIGN.md` or `/workspace/migration.sql` was signed off.
* **Recommended Implementation:**
  * Add an `ApprovalRequest` Ent schema and `/api/v1/projects/{projectId}/approvals` endpoints.
  * Add `sciontool approval request --gate <spec|security|lgpd|migration|release> --file /workspace/DESIGN.md --intent "..." --wait`, which computes SHA-256 hashes of the target files, creates a `pending` approval record on the Hub, marks the agent `WAITING_FOR_INPUT`, and blocks until an authorized principal approves the request.

### 2.2 Lifecycle Provenance (End-to-End Traceability & Pattern #11)
* **Current Code Inspection:**
  * [`pkg/store/models.go`](file:///Users/duboc/local/projects/scion/pkg/store/models.go#L80-L99) stores `AppliedConfig` (template ID/hash, harness config, model) and `Ancestry` (`[]string` of principal IDs from root user through parent orchestrators).
  * [`pkg/ent/schema/skill_injection.go`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/skill_injection.go) records every versioned skill (`skill_version_id`, `content_hash`) provisioned into the container.
* **Why Code Expansion Is Needed:**
  * Banking regulators (BACEN / LGPD / internal audit — **Pattern #11**) require forward and backward traceability between a business requirement or regulatory mandate, the AI reasoning/prompts used, the generated code/tests, and the human sign-off. Today, outputs stay inside `/workspace` or external Git branches without a structured ledger in `hub.db`.
* **Recommended Implementation:**
  * Add `ProvenanceAttestation` in `pkg/ent/schema/` storing:
    * `requirement_id` (e.g., `EPIC-4821`, `BACEN-RES-4893`)
    * `pdlc_pattern` (e.g., `02-cobol-modernization`, `04-lgpd-security-review`)
    * `agent_id` & `ancestry_chain`
    * `input_hashes` (template `content_hash`, skill version hashes, knowledge corpus hashes)
    * `output_artifacts` (Git commit SHA, branch, PR URL, file SHA-256 manifest)
    * `verification_evidence` (test runner command, exit code, pass/fail counts, coverage)
    * `approval_ids` (foreign keys to signed-off `ApprovalRequest` records)

### 2.3 Multi-Harness Routing & Automated Fallback (Pattern #10)
* **Current Code Inspection:**
  * [`pkg/hub/template_bootstrap.go`](file:///Users/duboc/local/projects/scion/pkg/hub/template_bootstrap.go#L137-L159) resolves `default_harness_config` statically from `scion-agent.yaml`.
  * [`harnesses/claude/provision.py`](file:///Users/duboc/local/projects/scion/harnesses/claude/provision.py#L261-L314) and [`harnesses/antigravity/provision.py`](file:///Users/duboc/local/projects/scion/harnesses/antigravity/provision.py#L70-L95) map generic model tier aliases (`small`, `medium`, `large`, `extra-large`) to harness-native models.
* **Why Code Expansion Is Needed:**
  * If an agent or schedule dispatches a task without hardcoding `--harness-config`, or if a primary model endpoint in Vertex AI Model Garden experiences quota exhaustion / regional degradation, the Hub should dynamically select the optimal harness/model based on **task complexity, cost budget, and live availability**.
* **Recommended Implementation:**
  * Introduce `HarnessRoutingPolicy` in project/hub settings supporting:
    * `task_routes`: e.g., `architecture | cobol | security-audit → claude (large)`, `frontend | synthetic-data | mutation-testing | debt-sweep → antigravity (medium)`
    * `fallback_chain`: e.g., `["claude", "antigravity", "gemini-cli"]` triggered automatically on provisioning failure or 429/5xx provider errors.

### 2.4 Continuous Observability & FinOps Telemetry
* **Current Code Inspection:**
  * [`pkg/sciontool/telemetry/usage.go`](file:///Users/duboc/local/projects/scion/pkg/sciontool/telemetry/usage.go#L81-L99) parses `com.anthropic.claude_code.events` (`input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_creation_tokens`) and emits `scion.usage.tokens` and `gen_ai.api.calls` over OTLP.
  * [`harnesses/antigravity/provision.py`](file:///Users/duboc/local/projects/scion/harnesses/antigravity/provision.py#L161-L171) sets `SCION_USAGE_SOURCE=hooks`, which currently records call counts without token counts.
  * [`pkg/store/models.go`](file:///Users/duboc/local/projects/scion/pkg/store/models.go#L56-L59) only persists `CurrentTurns` and `CurrentModelCalls` on the `Agent` record.
* **Recommended Implementation:**
  * Parse token counts from Antigravity's transcript/hook files in `pkg/sciontool/telemetry/`.
  * Persist cumulative `input_tokens`, `output_tokens`, `cache_read_tokens`, `cache_write_tokens`, and `estimated_cost_usd` on the Hub's `agents` table so the Web UI can render cost-per-agent, cost-per-pod-lineage, and cost-per-PDLC-pattern.

### 2.5 Unattended Batch Campaigns & Event Triggers (Patterns #2, #6, #7, #8, #19, #22)
* **Current Code Inspection:**
  * [`pkg/ent/schema/schedule.go`](file:///Users/duboc/local/projects/scion/pkg/ent/schema/schedule.go) supports recurring cron triggers targeting a single agent or single dispatch.
* **Recommended Implementation:**
  * Add a **`Campaign` (Batch Fan-Out) Runner** in `pkg/hub/` that accepts a target list (e.g., 50 repositories or COBOL programs), a template (`swe-modernizer` or `debt-sweeper`), and `max_concurrency` (e.g., 5 parallel containers), queuing and dispatching workers automatically as slots free up.
  * Add **Inbound Webhook Triggers** (`POST /api/v1/projects/{id}/triggers/{slug}`) to launch **Pattern #7 (Autonomous Incident Triage)** from Cloud Monitoring alerts or **Pattern #19 (Zero-Day Auto-Patching)** from security advisories.

---

## 3. Mapping the 22 Banking PDLC Patterns to Scion Pods & Platform Features

| # | Pattern Name | PDLC Pillar | Assigned Scion Pod / Template | Recommended Harness (Vertex AI) | Key Platform Feature Used |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **1** | Spec-to-Code Synthesis | Architecture & Build | `arch-spec-agent` | `claude` (Opus) | Knowledge Plane + Approval Gate |
| **2** | Legacy Mainframe Modernization (COBOL/JCL → Spring Boot) | Build & Refactoring | `swe-modernizer` | `claude` (Opus) | Batch Campaign Runner + Provenance |
| **3** | Automated API Contract Enforcement | Architecture & Design | `arch-spec-agent` | `claude` (Sonnet/Opus) | Knowledge Plane (OpenAPI Catalog) |
| **4** | Agentic Security & Compliance Review (OWASP/LGPD) | Testing & Security | `sec-compliance-auditor` | `claude` (Opus) | Approval Gate + Provenance Ledger |
| **5** | Dynamic Mutation & Integration Testing | Testing & QA | `sre-qa` | `antigravity` (`agy`) | Unattended Runtime + Test Attestation |
| **6** | Architectural Drift Detection | Governance & Design | `arch-spec-agent` | `claude` (Opus) | Recurring Cron Schedule (`scion schedule`) |
| **7** | Autonomous Incident Triage & RCA | Maintain & Operate | `sre-qa` | `antigravity` (`agy`) | Inbound Alert Webhook + OTel Logs |
| **8** | Dependency & Framework Modernization | Maintenance & Build | `swe-modernizer` | `claude` / `antigravity` | Batch Campaign Runner + Cron Schedule |
| **9** | Contextualized Knowledge Retrieval | Plan & Architecture | `eng-lead` / All Pods | `claude` / `antigravity` | Shared Knowledge Plane Volume + MCP |
| **10** | Dynamic Multi-Harness Fallback Routing | Platform & Operations | `eng-lead` + Hub Router | `claude` ↔ `antigravity` | Multi-Harness Router & Fallback Policy |
| **11** | Regulatory & Compliance Lineage Audit | Governance & Audit | `sec-compliance-auditor` | `claude` (Opus) | Provenance Attestation Ledger |
| **12** | Database Schema Migration Automation | Build & Database | `swe-backend` | `claude` (Opus) | DBA Approval Gate + Rollback Check |
| **13** | Accelerated Developer Onboarding | Enablement & Support | `eng-lead` / `sre-qa` | `antigravity` (`agy`) | Knowledge Plane + Interactive Walkthrough |
| **14** | LGPD-Compliant Synthetic Data Generation | Testing & Validation | `sre-qa` | `antigravity` (`agy`) | LGPD Anonymization Skill + Verifier |
| **15** | Runbook-to-Workflow Conversion | Operations & Infra | `sre-qa` | `antigravity` (`agy`) | Approval Gate + Scheduled Workflows |
| **16** | Standardized Microservice Chassis Bootstrapping | Architecture & Build | `swe-backend` | `claude` (Opus) | Knowledge Plane Chassis Blueprint |
| **17** | Design System Token Mapping | Front-End Build | `swe-frontend` | `antigravity` (`agy`) | Knowledge Plane Design Tokens |
| **18** | Automated Telemetry & Tracing Insertion | Maintain & Operate | `sre-qa` | `antigravity` (`agy`) | OTel Instrumentation Skill |
| **19** | Zero-Day Vulnerability Auto-Patching | Maintenance & Security | `sec-compliance-auditor` | `claude` (Opus) | Inbound CVE Trigger + Batch Campaign |
| **20** | Latency Profiling & Refactoring | Optimization | `swe-backend` | `claude` (Opus) | Benchmark Evidence Attestation |
| **21** | Feature Epic Decomposition | Planning & Spec | `eng-lead` | `claude` (Opus) | Multi-Agent Lineage Dispatch |
| **22** | Unattended Background Technical Debt Sweeps | Modernization | `debt-sweeper` | `antigravity` (`agy`) | Recurring Cron (`0 2 * * *`) + Limits |

---

## 4. Phased Implementation Plan for Scion Core

1. **Phase 1 — Accountability & Provenance (`pkg/ent`, `pkg/hub`, `cmd/sciontool`, `web`):**
   - Implement `ApprovalRequest` and `ProvenanceAttestation` schemas, REST endpoints, `sciontool approval` / `sciontool provenance` CLI subcommands, and Web UI sign-off cards.
2. **Phase 2 — FinOps Telemetry & Multi-Harness Router (`pkg/sciontool/telemetry`, `pkg/hub`):**
   - Add token extraction for `antigravity`, persist token/cost counters on `store.Agent`, and add `HarnessRoutingPolicy` with automatic fallback chains.
3. **Phase 3 — Unattended Campaigns & Inbound Triggers (`pkg/hub`, `cmd`):**
   - Build the bounded-concurrency `Campaign` batch runner and inbound webhook trigger endpoints for incident triage (#7) and CVE patching (#19).
4. **Phase 4 — Hub Knowledge Plane Registry (`pkg/ent`, `pkg/hub`, `pkg/runtimebroker`):**
   - Add `KnowledgeSource` entities to auto-mount versioned enterprise corpora and MCP servers into project pods.
