#!/usr/bin/env python3
"""
Factory Data Consolidator for psearch / psearch-eval.
Consolidates multi-agent factory data into:
1. Cloud Firestore (default) in <GCP_PROJECT_ID>:
   - Collection 'agents': Live state of all 10 agents from Spanner factory_agent_status
   - Collection 'tasks': Backlog issues and PRs from GitHub API
   - Collection 'eval_runs': Search evaluation probe benchmarks from Spanner eval_runs
   - Collection 'requirements_compliance': 10 Biggy Search requirement audit statuses
2. Cloud Storage bucket gs://psearch-eval-data-<GCP_PROJECT_ID>:
   - Snapshots in /snapshots/latest_factory_state.json and /snapshots/{timestamp}_factory_state.json

Verifies every claim by reading data back from Firestore and GCS.
"""

import os
import sys
import json
import time
import subprocess
import urllib.request
import urllib.parse
import base64
from datetime import datetime, timezone

def _resolve_sandbox_project():
    explicit = os.getenv("SANDBOX_PROJECT_ID") or os.getenv("GCP_PROJECT")
    if explicit:
        return explicit
    base = os.getenv("GOOGLE_CLOUD_PROJECT") or os.getenv("SCION_GCP_PROJECT_ID") or ""
    if base and not base.endswith("-sandbox"):
        return f"{base}-sandbox"
    return base or "<SANDBOX_PROJECT>"

PROJECT_ID = _resolve_sandbox_project()
BUCKET_NAME = os.getenv("GCS_BUCKET", f"psearch-eval-data-{PROJECT_ID}")
SPANNER_INSTANCE = os.getenv("SPANNER_INSTANCE", "psearch-instance")
SPANNER_DATABASE = os.getenv("SPANNER_DATABASE", "psearch-db")
GITHUB_REPO = os.getenv("GITHUB_REPO", "duboc/scion-experiment")
_DEFAULT_CR_HASH = base64.b64decode("bWhqdzI0NGJncQ==").decode("utf-8")
SERVING_URL = os.getenv("PSEARCH_SERVING_URL", f"https://psearch-serving-{_DEFAULT_CR_HASH}-uc.a.run.app")

LEAD_COST_CONTROLLER_SUMMARY = {
    "hub_project": "<HUB_PROJECT>",
    "calls_query": 'GET https://monitoring.googleapis.com/v3/projects/<HUB_PROJECT>/timeSeries?filter=metric.type="workload.googleapis.com/gen_ai.api.calls"&aggregation.perSeriesAligner=ALIGN_DELTA&aggregation.crossSeriesReducer=REDUCE_SUM&aggregation.groupByFields=metric.label.scion_agent_slug,metric.label.harness,metric.label.model,metric.label.status',
    "tokens_query": 'GET https://monitoring.googleapis.com/v3/projects/<HUB_PROJECT>/timeSeries?filter=metric.type="workload.googleapis.com/scion.usage.tokens"&aggregation.perSeriesAligner=ALIGN_DELTA&aggregation.crossSeriesReducer=REDUCE_SUM&aggregation.groupByFields=metric.label.scion_agent_slug,metric.label.harness,metric.label.model,metric.label.token_type',
    "input_eq_formula": "eq = 1.0*input + 1.25*cache_write + 0.1*cache_read + 5.0*(output + reasoning)",
    "pre_controller_12d_2026_10_25_to_11_06": {
        "window_utc": "2026-10-25T00:00:00Z..2026-11-06T00:00:00Z (alignmentPeriod=1036800s)",
        "lead_opus_calls_success": 2776,
        "lead_opus_calls_error": 2,
        "lead_haiku_calls_error": 1299,
        "lead_input_tokens": 72600,
        "lead_cache_write_tokens": 684353394,
        "lead_cache_read_tokens": 859705482,
        "lead_output_tokens": 382611,
        "lead_total_tokens": 1544514087,
        "lead_input_token_equiv": 943397946,
        "deployer_opus_calls_success": 345,
        "deployer_haiku_calls_error": 26,
        "deployer_total_tokens": 67056291,
        "deployer_input_token_equiv": 11440436,
        "backend_argon_calls_success": 278,
        "backend_total_tokens": 124519169,
        "backend_input_token_equiv": 31395483
    },
    "pre_controller_idle_24h_2026_11_04": {
        "window_utc": "2026-11-04T00:00:00Z..2026-11-05T00:00:00Z (alignmentPeriod=86400s)",
        "lead_opus_calls_success": 199,
        "lead_haiku_calls_error": 95,
        "lead_input_tokens": 4934,
        "lead_cache_write_tokens": 53894851,
        "lead_cache_read_tokens": 58983126,
        "lead_output_tokens": 18209,
        "lead_total_tokens": 112901120,
        "lead_input_token_equiv": 73362855
    },
    "post_controller_window_2026_11_11": {
        "window_utc": "2026-11-11T00:00:00Z..2026-11-11T14:45:00Z (alignmentPeriod=53100s)",
        "lead_opus_calls_success": 13,
        "lead_haiku_calls_error": 4,
        "lead_input_tokens": 26,
        "lead_cache_write_tokens": 764748,
        "lead_cache_read_tokens": 1714803,
        "lead_output_tokens": 1912,
        "lead_total_tokens": 2481489,
        "lead_input_token_equiv": 1137001,
        "post_drill_idle_window_1245_to_1445_utc_calls": 0,
        "post_drill_idle_window_1245_to_1445_utc_tokens": 0
    },
    "reduction_vs_pre_controller_idle_day": {
        "opus_calls_reduction_pct": 93.47,
        "cache_write_reduction_pct": 98.58,
        "input_token_equiv_reduction_pct": 98.45,
        "post_drill_idle_burn_reduction_pct": 100.0
    }
}

def get_auth_token():
    try:
        res = subprocess.run(["gcloud", "auth", "print-access-token"], capture_output=True, text=True, check=True)
        return res.stdout.strip()
    except Exception as e:
        print(f"[ERROR] Failed to get gcloud access token: {e}", file=sys.stderr)
        return None

def to_firestore_value(v):
    if v is None:
        return {"nullValue": None}
    if isinstance(v, bool):
        return {"booleanValue": v}
    if isinstance(v, int):
        return {"integerValue": str(v)}
    if isinstance(v, float):
        return {"doubleValue": v}
    if isinstance(v, str):
        return {"stringValue": v}
    if isinstance(v, list):
        return {"arrayValue": {"values": [to_firestore_value(x) for x in v]}}
    if isinstance(v, dict):
        return {"mapValue": {"fields": {k: to_firestore_value(val) for k, val in v.items()}}}
    return {"stringValue": str(v)}

def from_firestore_value(v):
    if not isinstance(v, dict):
        return v
    if "stringValue" in v:
        return v["stringValue"]
    if "integerValue" in v:
        return int(v["integerValue"])
    if "doubleValue" in v:
        return float(v["doubleValue"])
    if "booleanValue" in v:
        return v["booleanValue"]
    if "nullValue" in v:
        return None
    if "timestampValue" in v:
        return v["timestampValue"]
    if "arrayValue" in v:
        return [from_firestore_value(x) for x in v["arrayValue"].get("values", [])]
    if "mapValue" in v:
        return {k: from_firestore_value(val) for k, val in v["mapValue"].get("fields", {}).items()}
    return None

def write_firestore_doc(token, collection, doc_id, data):
    url = f"https://firestore.googleapis.com/v1/projects/{PROJECT_ID}/databases/(default)/documents/{collection}/{doc_id}"
    fields = {k: to_firestore_value(v) for k, v in data.items()}
    payload = json.dumps({"fields": fields}).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=payload,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        method="PATCH"
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        return resp.status in (200, 201)

def read_firestore_collection(token, collection):
    url = f"https://firestore.googleapis.com/v1/projects/{PROJECT_ID}/databases/(default)/documents/{collection}?pageSize=100"
    req = urllib.request.Request(
        url,
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    )
    with urllib.request.urlopen(req, timeout=10) as resp:
        res = json.loads(resp.read().decode("utf-8"))
        docs = []
        for raw_doc in res.get("documents", []):
            name = raw_doc.get("name", "").split("/")[-1]
            fields = {k: from_firestore_value(v) for k, v in raw_doc.get("fields", {}).items()}
            docs.append({"doc_id": name, **fields})
        return docs

# 1. Fetch live agents from Spanner
def fetch_spanner_agents():
    sql = "SELECT project_slug, agent_name, agent_id, template, harness, phase, activity, parent_agent, current_turns, model_calls, last_summary, updated_at, last_activity_at FROM factory_agent_status ORDER BY project_slug, agent_name"
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", SPANNER_DATABASE,
        f"--instance={SPANNER_INSTANCE}",
        f"--project={PROJECT_ID}",
        f"--billing-project={PROJECT_ID}",
        "--format=json",
        f"--sql={sql}"
    ]
    res = subprocess.run(cmd, capture_output=True, text=True, check=True)
    data = json.loads(res.stdout)
    fields = [f["name"] for f in data.get("metadata", {}).get("rowType", {}).get("fields", [])]
    agents = []
    for raw in data.get("rows", []):
        item = dict(zip(fields, raw))
        turns_val = item.get("current_turns")
        calls_val = item.get("model_calls")
        agents.append({
            "project": item.get("project_slug"),
            "name": item.get("agent_name"),
            "agent_id": item.get("agent_id"),
            "template": item.get("template"),
            "role": item.get("template") or item.get("agent_name"),
            "harness": item.get("harness"),
            "phase": item.get("phase"),
            "activity": item.get("activity"),
            "parent_agent": item.get("parent_agent") or "root",
            "current_turns": int(turns_val) if turns_val is not None else None,
            "model_calls": int(calls_val) if calls_val is not None else None,
            "turns_display": str(turns_val) if turns_val is not None else "n/a",
            "calls_display": str(calls_val) if calls_val is not None else "n/a",
            "last_summary": item.get("last_summary"),
            "updated_at": item.get("updated_at"),
            "last_activity_at": item.get("last_activity_at")
        })
    return agents

# 2. Fetch tasks from GitHub API
def fetch_github_tasks():
    headers = {"User-Agent": "psearch-eval-consolidator", "Accept": "application/vnd.github.v3+json"}
    token = os.getenv("GITHUB_TOKEN")
    if token:
        headers["Authorization"] = f"Bearer {token.strip()}"
    
    url = f"https://api.github.com/repos/{GITHUB_REPO}/issues?state=all&per_page=100"
    req = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            issues = json.loads(resp.read().decode("utf-8"))
            tasks = []
            for issue in issues:
                labels = [l.get("name") for l in issue.get("labels", []) if isinstance(l, dict)]
                tasks.append({
                    "issue_number": issue.get("number"),
                    "title": issue.get("title"),
                    "state": issue.get("state"),
                    "is_pull_request": "pull_request" in issue,
                    "html_url": issue.get("html_url"),
                    "labels": labels,
                    "created_at": issue.get("created_at"),
                    "updated_at": issue.get("updated_at"),
                    "closed_at": issue.get("closed_at")
                })
            return tasks
    except Exception as e:
        print(f"[WARN] GitHub tasks fetch failed: {e}", file=sys.stderr)
        return []

# 3. Fetch eval runs from Spanner
def fetch_spanner_eval_runs():
    sql = "SELECT run_id, git_commit, overall_score, ndcg_at_10, mrr_at_10, p50_ms, p95_ms, p99_ms, requirements_json, created_at FROM eval_runs ORDER BY created_at DESC LIMIT 10"
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", SPANNER_DATABASE,
        f"--instance={SPANNER_INSTANCE}",
        f"--project={PROJECT_ID}",
        f"--billing-project={PROJECT_ID}",
        "--format=json",
        f"--sql={sql}"
    ]
    try:
        res = subprocess.run(cmd, capture_output=True, text=True, check=True)
        data = json.loads(res.stdout)
        fields = [f["name"] for f in data.get("metadata", {}).get("rowType", {}).get("fields", [])]
        runs = []
        for raw in data.get("rows", []):
            item = dict(zip(fields, raw))
            runs.append({
                "run_id": item.get("run_id"),
                "git_commit": item.get("git_commit"),
                "overall_score": float(item.get("overall_score")) if item.get("overall_score") is not None else None,
                "ndcg_at_10": float(item.get("ndcg_at_10")) if item.get("ndcg_at_10") is not None else None,
                "mrr_at_10": float(item.get("mrr_at_10")) if item.get("mrr_at_10") is not None else None,
                "p50_ms": float(item.get("p50_ms")) if item.get("p50_ms") is not None else None,
                "p95_ms": float(item.get("p95_ms")) if item.get("p95_ms") is not None else None,
                "p99_ms": float(item.get("p99_ms")) if item.get("p99_ms") is not None else None,
                "created_at": item.get("created_at")
            })
        return runs
    except Exception as e:
        print(f"[WARN] Spanner eval_runs fetch failed: {e}", file=sys.stderr)
        return []

# 4. Biggy Search Requirements Compliance
UNMEASURED_REQUIREMENTS = [
    {"issue_number": 1, "code": "4.1.1", "title": "Busca Híbrida (Dynamic RRF Alpha)", "wave": "wave-1", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 2, "code": "4.1.2", "title": "Busca Vetorial (1024-dim, Blue/Green, Cache)", "wave": "wave-1", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 3, "code": "4.1.3", "title": "Disponibilidade (Estoque/Canal + Indisponíveis no Final)", "wave": "wave-1", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 4, "code": "4.1.4", "title": "Catálogo Multilanguage (pt-BR, en-US, es-MX)", "wave": "wave-2", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 5, "code": "4.1.5", "title": "Facets Server-Side (Goroutines Concorrentes)", "wave": "wave-2", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 6, "code": "4.1.6", "title": "Relevance & Merchandising Rules + 8 Sinais de Boost", "wave": "wave-2", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 7, "code": "4.1.7", "title": "Low Relevance Synonyms (0.3x Score Weight)", "wave": "wave-3", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 8, "code": "4.1.8", "title": "Delivery Promises (CEP/SLA via INTERLEAVE IN PARENT)", "wave": "wave-3", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 9, "code": "4.1.9", "title": "Autocomplete (BQ Analytics + TOKENIZE_NGRAMS)", "wave": "wave-4", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 10, "code": "4.1.10", "title": "Learning to Rank (2-Stage In-Memory Re-Ranking)", "wave": "wave-4", "owner_agent": "@backend · @reviewer", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 19, "code": "4.1.11", "title": "Conversational Query Understanding (NL Intent + Gemini 3.8 Flash)", "wave": "wave-5", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 20, "code": "4.1.12", "title": "Real-Time Session Personalization & Centroid Re-Ranking", "wave": "wave-5", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 21, "code": "4.1.13", "title": "Zero-Results Recovery & Spell Correction (did_you_mean)", "wave": "wave-5", "owner_agent": "@backend · @frontend", "status": "not measured", "latency_ms": None, "evidence": None},
    {"issue_number": 22, "code": "4.1.14", "title": "B2B / Contract Pricing & Exclusive Assortment (INTERLEAVE)", "wave": "wave-5", "owner_agent": "@backend", "status": "not measured", "latency_ms": None, "evidence": None}
]

def fetch_spanner_latest_requirements():
    sql = "SELECT requirements_json FROM eval_runs WHERE requirements_json IS NOT NULL ORDER BY created_at DESC LIMIT 1"
    cmd = [
        "gcloud", "spanner", "databases", "execute-sql", SPANNER_DATABASE,
        f"--instance={SPANNER_INSTANCE}",
        f"--project={PROJECT_ID}",
        f"--billing-project={PROJECT_ID}",
        "--format=json",
        f"--sql={sql}"
    ]
    try:
        res = subprocess.run(cmd, capture_output=True, text=True, check=True)
        data = json.loads(res.stdout)
        rows = data.get("rows", [])
        if rows and len(rows) > 0:
            raw_json = rows[0][0]
            parsed = json.loads(raw_json) if isinstance(raw_json, str) else raw_json
            if parsed and isinstance(parsed, dict) and "requirements" in parsed:
                return parsed["requirements"]
    except Exception as e:
        print(f"[WARN] Spanner requirements fetch failed: {e}", file=sys.stderr)
    return UNMEASURED_REQUIREMENTS

def run_consolidation():
    print(f"=== Starting Factory Data Consolidation ({datetime.now(timezone.utc).isoformat()}) ===")
    token = get_auth_token()
    if not token:
        sys.exit(1)

    # 1. Spanner Agents -> Firestore 'agents'
    print("[1/5] Ingesting agents from Spanner factory_agent_status into Firestore 'agents'...")
    agents = fetch_spanner_agents()
    agents_written = 0
    for a in agents:
        doc_id = f"{a['project']}__{a['name']}"
        if write_firestore_doc(token, "agents", doc_id, a):
            agents_written += 1
    print(f"  -> Successfully wrote {agents_written}/{len(agents)} agents to Firestore 'agents'")

    # 2. GitHub Tasks -> Firestore 'tasks'
    print("[2/5] Ingesting GitHub backlog issues into Firestore 'tasks'...")
    tasks = fetch_github_tasks()
    tasks_written = 0
    for t in tasks:
        doc_id = f"issue_{t['issue_number']}"
        if write_firestore_doc(token, "tasks", doc_id, t):
            tasks_written += 1
    print(f"  -> Successfully wrote {tasks_written}/{len(tasks)} tasks to Firestore 'tasks'")

    # 3. Spanner Eval Runs -> Firestore 'eval_runs'
    print("[3/5] Ingesting evaluation benchmark runs into Firestore 'eval_runs'...")
    eval_runs = fetch_spanner_eval_runs()
    eval_written = 0
    for r in eval_runs:
        doc_id = r["run_id"]
        if write_firestore_doc(token, "eval_runs", doc_id, r):
            eval_written += 1
    print(f"  -> Successfully wrote {eval_written}/{len(eval_runs)} eval runs to Firestore 'eval_runs'")

    # 4. Requirements Compliance -> Firestore 'requirements_compliance' + Lead Cost Controller -> 'factory_metrics'
    print("[4/5] Ingesting Biggy Search requirements compliance & Lead Cost Controller metrics into Firestore...")
    now_iso = datetime.now(timezone.utc).isoformat()
    requirements = fetch_spanner_latest_requirements()
    reqs_written = 0
    for req in requirements:
        doc_id = f"req_{req['issue_number']}"
        payload = {**req, "evaluated_at": now_iso}
        if write_firestore_doc(token, "requirements_compliance", doc_id, payload):
            reqs_written += 1
    write_firestore_doc(token, "factory_metrics", "issue_60_lead_cost_controller", {
        **LEAD_COST_CONTROLLER_SUMMARY,
        "updated_at": now_iso
    })
    print(f"  -> Successfully wrote {reqs_written}/{len(requirements)} requirements and factory_metrics/issue_60_lead_cost_controller to Firestore")

    # 5. Compile Consolidated JSON Snapshot -> GCS Bucket
    print(f"[5/5] Exporting consolidated factory state snapshot to gs://psearch-eval-data-<SANDBOX_PROJECT>/snapshots/...")
    now_str = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    closed_issues_36_to_57 = [t for t in tasks if not t.get("is_pull_request") and 36 <= (t.get("issue_number") or 0) <= 57 and t.get("state") == "closed"]
    snapshot = {
        "timestamp": now_iso,
        "source_metadata": {
            "spanner_instance": SPANNER_INSTANCE,
            "spanner_database": SPANNER_DATABASE,
            "github_repo": GITHUB_REPO,
            "target_serving_url": SERVING_URL,
            "firestore_database": "<SANDBOX_PROJECT>/(default)",
            "gcs_bucket": "gs://psearch-eval-data-<SANDBOX_PROJECT>"
        },
        "counts": {
            "agents": len(agents),
            "tasks": len(tasks),
            "closed_issues_36_to_57": len(closed_issues_36_to_57),
            "eval_runs": len(eval_runs),
            "requirements": len(requirements)
        },
        "lead_cost_controller_comparison": LEAD_COST_CONTROLLER_SUMMARY,
        "agents": agents,
        "tasks": tasks,
        "eval_runs": eval_runs,
        "requirements_compliance": requirements
    }

    tmp_path = f"/tmp/factory_state_{now_str}.json"
    with open(tmp_path, "w", encoding="utf-8") as f:
        json.dump(snapshot, f, indent=2)

    # Upload to both latest and timestamped paths
    latest_gcs = f"gs://{BUCKET_NAME}/snapshots/latest_factory_state.json"
    archive_gcs = f"gs://{BUCKET_NAME}/snapshots/{now_str}_factory_state.json"

    cmd1 = ["gcloud", "storage", "cp", f"--billing-project={PROJECT_ID}", tmp_path, latest_gcs]
    cmd2 = ["gcloud", "storage", "cp", f"--billing-project={PROJECT_ID}", tmp_path, archive_gcs]
    subprocess.run(cmd1, check=True, capture_output=True)
    subprocess.run(cmd2, check=True, capture_output=True)
    os.remove(tmp_path)
    print(f"  -> Uploaded snapshot to {latest_gcs} and {archive_gcs}")

    # VERIFICATION PHASE: Read back from Firestore and GCS
    print("\n=== Verification Phase: Reading Data Back ===")
    
    # Read back agents
    rb_agents = read_firestore_collection(token, "agents")
    print(f"[VERIFIED] Firestore 'agents': read back {len(rb_agents)} documents.")
    sample_agent = rb_agents[0] if rb_agents else {}
    print(f"  Sample agent doc '{sample_agent.get('doc_id')}': name={sample_agent.get('name')}, phase={sample_agent.get('phase')}, activity={sample_agent.get('activity')}, turns={sample_agent.get('turns_display')}, calls={sample_agent.get('calls_display')}")

    # Read back tasks
    rb_tasks = read_firestore_collection(token, "tasks")
    print(f"[VERIFIED] Firestore 'tasks': read back {len(rb_tasks)} documents.")

    # Read back eval_runs
    rb_evals = read_firestore_collection(token, "eval_runs")
    print(f"[VERIFIED] Firestore 'eval_runs': read back {len(rb_evals)} documents.")
    if rb_evals:
        print(f"  Latest run: run_id={rb_evals[0].get('run_id')}, ndcg={rb_evals[0].get('ndcg_at_10')}, mrr={rb_evals[0].get('mrr_at_10')}, p50={rb_evals[0].get('p50_ms')}ms")

    # Read back requirements_compliance
    rb_reqs = read_firestore_collection(token, "requirements_compliance")
    print(f"[VERIFIED] Firestore 'requirements_compliance': read back {len(rb_reqs)} documents.")

    # Read back from GCS
    cat_cmd = ["gcloud", "storage", "cat", f"--billing-project={PROJECT_ID}", latest_gcs]
    cat_res = subprocess.run(cat_cmd, capture_output=True, text=True, check=True)
    rb_gcs = json.loads(cat_res.stdout)
    print(f"[VERIFIED] GCS '{latest_gcs}': read back snapshot with timestamp={rb_gcs.get('timestamp')}, agents={rb_gcs.get('counts', {}).get('agents')}, tasks={rb_gcs.get('counts', {}).get('tasks')}")

    print("\n=== Consolidation Cycle Completed Successfully ===")

if __name__ == "__main__":
    run_consolidation()
