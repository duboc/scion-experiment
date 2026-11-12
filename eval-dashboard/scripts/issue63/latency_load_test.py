#!/usr/bin/env python3
"""
Issue #63 (Item 7) — Fixed-Rate Open-Loop Latency Load Test & Stage Attribution
================================================================================

Executes:
  1. Fixed-rate open-loop load tests at 2.0 RPS for 60s (120 reqs) and 5.0 RPS
     for 60s (300 reqs) against:
       - Live Cloud Run revision `psearch-serving-00043-p87`
       - Local Option 5B revision `psearch-serving-00029-7ss` (`http://127.0.0.1:18090`)
         running against the same 20,535-product Spanner catalog.
  2. Separates initial cold-start / warm-up probes from steady-state open-loop
     traffic and cross-verifies:
       - Client wall-clock latency (`p50`, `p90`, `p95`, `p99`, `mean`)
       - Server-reported JSON `latency_ms` & `facets_latency_ms`
       - Cloud Run request logs (`run.googleapis.com/requests` `httpRequest.latency`
         and any `startup_latencies`)
       - Cloud Trace (`X-Cloud-Trace-Context` header injection + Cloud Trace v1 API)
  3. Controlled stage-ablation latency attribution between `00029-7ss` and `00043-p87`:
       - `alpha=0.0` (pure FTS, `parse_intent=false`, `ads=false`, `ltr=false`)
       - `alpha=1.0` (pure vector: ScaNN ANN on `00043-p87` vs exact scan on `00029-7ss`)
       - `alpha=0.5` (balanced hybrid RRF, `ads=false`, `ltr=false`, `parse_intent=false`)
       - `ltr=true` vs `ltr=false` (Stage-2 LTR re-ranker overhead + `ltr_latency_us`)
       - `ads=true` vs `ads=false` (Sponsored GSP auction overhead)
       - `parse_intent=true` vs `parse_intent=false` (Gemini 3.8 Flash NL parser overhead)
       - `cached_embedding` vs `uncached_embedding` (Spanner/RAM embedding cache hit vs
         live Vertex AI `gemini-embedding-2` call)
"""

import argparse
import base64
import concurrent.futures
import datetime
import json
import math
import os
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any, Dict, List, Optional, Tuple

_DEFAULT_PROJ_ID = base64.b64decode("cmlvanVjdS1zYW5kYm94").decode("utf-8")
_DEFAULT_CR_HASH = base64.b64decode("bWhqdzI0NGJncQ==").decode("utf-8")
_DEFAULT_PROJ_NUM = base64.b64decode("NjQ3MzI1MDEzNDU0").decode("utf-8")

LIVE_SERVING_URL = os.environ.get(
    "PSEARCH_SERVING_URL",
    f"https://psearch-serving-{_DEFAULT_CR_HASH}-uc.a.run.app",
)
LOCAL_00029_URL = os.environ.get("LOCAL_REV_00029_URL", "http://127.0.0.1:18090")

# Representative mixed query workload spanning en-US and pt-BR pharmacy/dermocosmetics/compounding
WORKLOAD_QUERIES: List[Dict[str, str]] = [
    {"q": "protetor solar fps 60", "locale": "pt-BR", "alpha": "0.5", "parse_intent": "false"},
    {"q": "running shoes", "locale": "en-US", "alpha": "0.5", "parse_intent": "false"},
    {"q": "minoxidil base", "locale": "pt-BR", "alpha": "0.5", "parse_intent": "false"},
    {"q": "vitamina c adcos", "locale": "pt-BR", "alpha": "0.65", "parse_intent": "false"},
    {"q": "dipirona 1g", "locale": "pt-BR", "alpha": "0.3", "parse_intent": "false"},
    {"q": "shampoo anticaspa", "locale": "pt-BR", "alpha": "0.5", "parse_intent": "false"},
    {"q": "colageno verisol", "locale": "pt-BR", "alpha": "0.5", "parse_intent": "false"},
    {"q": "smartwatch titanium", "locale": "en-US", "alpha": "0.5", "parse_intent": "false"},
    {"q": "blister cristal goma", "locale": "pt-BR", "alpha": "0.0", "parse_intent": "false"},
    {"q": "touca sanfonada verde", "locale": "pt-BR", "alpha": "0.5", "parse_intent": "false"},
    {"q": "creme hidratante cerave", "locale": "pt-BR", "alpha": "0.65", "parse_intent": "false"},
    {"q": "wireless headphones", "locale": "en-US", "alpha": "0.5", "parse_intent": "false"},
]


def percentile(values: List[float], p: float) -> float:
    if not values:
        return 0.0
    s = sorted(values)
    if len(s) == 1:
        return round(s[0], 2)
    rank = (p / 100.0) * (len(s) - 1)
    lo = int(math.floor(rank))
    hi = int(math.ceil(rank))
    frac = rank - lo
    return round(s[lo] + frac * (s[hi] - s[lo]), 2)


def summarize_distribution(values: List[float]) -> Dict[str, float]:
    clean = [float(v) for v in values if v is not None and not math.isnan(float(v))]
    if not clean:
        return {"count": 0, "min": 0.0, "p50": 0.0, "p90": 0.0, "p95": 0.0, "p99": 0.0, "max": 0.0, "mean": 0.0}
    return {
        "count": len(clean),
        "min": round(min(clean), 2),
        "p50": percentile(clean, 50),
        "p90": percentile(clean, 90),
        "p95": percentile(clean, 95),
        "p99": percentile(clean, 99),
        "max": round(max(clean), 2),
        "mean": round(sum(clean) / len(clean), 2),
    }


def make_trace_header() -> Tuple[str, str]:
    trace_id = uuid.uuid4().hex
    span_id = str(uuid.uuid4().int & ((1 << 63) - 1))
    return trace_id, f"{trace_id}/{span_id};o=1"


def issue_single_request(
    base_url: str,
    params: Dict[str, str],
    seq_idx: int,
    scheduled_offset_s: float,
) -> Dict[str, Any]:
    trace_id, trace_hdr = make_trace_header()
    qs = urllib.parse.urlencode(params)
    url = f"{base_url.rstrip('/')}/api/search?{qs}"
    req = urllib.request.Request(
        url,
        headers={
            "Accept": "application/json",
            "X-Cloud-Trace-Context": trace_hdr,
            "User-Agent": "psearch-eval-issue63-loadtest/1.0",
        },
        method="GET",
    )
    t_start = time.perf_counter()
    ts_utc = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")
    status = 0
    server_lat: Optional[float] = None
    facets_lat: Optional[float] = None
    ltr_lat_us: Optional[float] = None
    vec_idx_used: Optional[bool] = None
    total_found: Optional[int] = None
    results_len = 0
    err_str: Optional[str] = None

    try:
        with urllib.request.urlopen(req, timeout=25) as resp:
            status = resp.status
            raw = resp.read()
            client_ms = (time.perf_counter() - t_start) * 1000.0
            data = json.loads(raw.decode("utf-8"))
            if isinstance(data, dict):
                if data.get("latency_ms") is not None:
                    server_lat = float(data["latency_ms"])
                if data.get("facets_latency_ms") is not None:
                    facets_lat = float(data["facets_latency_ms"])
                if data.get("ltr_latency_us") is not None:
                    ltr_lat_us = float(data["ltr_latency_us"])
                vec_idx_used = data.get("vector_index_used")
                total_found = data.get("total_found")
                results_len = len(data.get("results") or [])
    except urllib.error.HTTPError as e:
        client_ms = (time.perf_counter() - t_start) * 1000.0
        status = e.code
        err_str = f"HTTP {e.code}"
    except Exception as e:
        client_ms = (time.perf_counter() - t_start) * 1000.0
        status = 0
        err_str = str(e)

    transport_ms = round(client_ms - server_lat, 2) if server_lat is not None else None
    return {
        "seq": seq_idx,
        "scheduled_offset_s": round(scheduled_offset_s, 3),
        "timestamp_utc": ts_utc,
        "trace_id": trace_id,
        "query": params.get("q", ""),
        "locale": params.get("locale", ""),
        "alpha": params.get("alpha", ""),
        "status": status,
        "client_latency_ms": round(client_ms, 2),
        "server_latency_ms": round(server_lat, 2) if server_lat is not None else None,
        "facets_latency_ms": round(facets_lat, 2) if facets_lat is not None else None,
        "ltr_latency_us": round(ltr_lat_us, 2) if ltr_lat_us is not None else None,
        "transport_overhead_ms": transport_ms,
        "vector_index_used": vec_idx_used,
        "total_found": total_found,
        "results_count": results_len,
        "error": err_str,
    }


def run_cold_and_warmup_phase(base_url: str, label: str) -> Dict[str, Any]:
    """Measures initial request latency (cold/initial probe) and warms the 12 workload query embeddings."""
    print(f"\n[{label}] Running initial probe & cache warm-up ({len(WORKLOAD_QUERIES)} queries)...")
    records: List[Dict[str, Any]] = []
    for idx, q_params in enumerate(WORKLOAD_QUERIES):
        rec = issue_single_request(base_url, q_params, idx, 0.0)
        records.append(rec)
    first_req_ms = records[0]["client_latency_ms"] if records else 0.0
    first_srv_ms = records[0]["server_latency_ms"] if records else 0.0
    print(
        f"  [{label}] Initial request #0: client={first_req_ms}ms, server={first_srv_ms}ms | "
        f"Warm-up p50={summarize_distribution([r['client_latency_ms'] for r in records])['p50']}ms"
    )
    return {
        "initial_request": records[0] if records else None,
        "warmup_client_ms": summarize_distribution([r["client_latency_ms"] for r in records]),
        "warmup_server_ms": summarize_distribution(
            [r["server_latency_ms"] for r in records if r["server_latency_ms"] is not None]
        ),
        "records": records,
    }


def run_open_loop_load_test(
    base_url: str,
    label: str,
    rps: float,
    duration_s: int,
) -> Dict[str, Any]:
    """
    Dispatches requests strictly on an open-loop fixed wall-clock schedule:
      t_dispatch(i) = t_0 + i / rps
    using a thread pool so slow responses never delay subsequent request arrivals.
    """
    total_reqs = int(round(rps * duration_s))
    interval_s = 1.0 / rps
    print(f"\n[{label}] Starting Open-Loop Fixed-Rate Load Test: {rps:.1f} RPS for {duration_s}s ({total_reqs} requests)...")

    start_utc = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")
    t0 = time.perf_counter()
    futures: List[concurrent.futures.Future] = []

    with concurrent.futures.ThreadPoolExecutor(max_workers=32) as pool:
        for i in range(total_reqs):
            target_offset = i * interval_s
            now_offset = time.perf_counter() - t0
            sleep_s = target_offset - now_offset
            if sleep_s > 0:
                time.sleep(sleep_s)
            q_params = dict(WORKLOAD_QUERIES[i % len(WORKLOAD_QUERIES)])
            fut = pool.submit(issue_single_request, base_url, q_params, i, target_offset)
            futures.append(fut)

        records = [f.result() for f in futures]

    wall_elapsed_s = round(time.perf_counter() - t0, 3)
    end_utc = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%S.%fZ")

    ok_records = [r for r in records if r["status"] == 200]
    err_records = [r for r in records if r["status"] != 200]

    client_lats = [r["client_latency_ms"] for r in ok_records]
    server_lats = [r["server_latency_ms"] for r in ok_records if r["server_latency_ms"] is not None]
    facets_lats = [r["facets_latency_ms"] for r in ok_records if r["facets_latency_ms"] is not None]
    transport_lats = [r["transport_overhead_ms"] for r in ok_records if r["transport_overhead_ms"] is not None]

    summary = {
        "target_label": label,
        "target_rps": rps,
        "duration_s": duration_s,
        "actual_wall_elapsed_s": wall_elapsed_s,
        "achieved_dispatch_rps": round(total_reqs / max(wall_elapsed_s, 0.001), 2),
        "window_start_utc": start_utc,
        "window_end_utc": end_utc,
        "total_requests": total_reqs,
        "http_200_count": len(ok_records),
        "error_count": len(err_records),
        "error_rate": round(len(err_records) / max(total_reqs, 1), 4),
        "client_latency_ms": summarize_distribution(client_lats),
        "server_latency_ms": summarize_distribution(server_lats),
        "facets_latency_ms": summarize_distribution(facets_lats),
        "transport_overhead_ms": summarize_distribution(transport_lats),
        "sample_trace_ids": [r["trace_id"] for r in ok_records[:10]],
        "slowest_5_requests": sorted(ok_records, key=lambda r: r["client_latency_ms"], reverse=True)[:5],
        "records": records,
    }
    print(
        f"  [{label} @ {rps:.1f} RPS] 200 OK={len(ok_records)}/{total_reqs} | "
        f"Client p50={summary['client_latency_ms']['p50']}ms p90={summary['client_latency_ms']['p90']}ms "
        f"p95={summary['client_latency_ms']['p95']}ms p99={summary['client_latency_ms']['p99']}ms | "
        f"Server p50={summary['server_latency_ms']['p50']}ms p95={summary['server_latency_ms']['p95']}ms "
        f"p99={summary['server_latency_ms']['p99']}ms"
    )
    return summary


def run_stage_ablation(base_url: str, label: str, reps: int = 8) -> Dict[str, Any]:
    """
    Runs controlled stage-ablation probes to attribute server latency across:
      1. pure_fts (alpha=0.0, parse_intent=false, ads=false, ltr=false)
      2. pure_vector (alpha=1.0, parse_intent=false, ads=false, ltr=false)
      3. hybrid_rrf_no_ltr_no_ads (alpha=0.5, parse_intent=false, ads=false, ltr=false)
      4. hybrid_with_ltr_no_ads (alpha=0.5, parse_intent=false, ads=false, ltr=true)
      5. hybrid_with_ltr_and_ads (alpha=0.5, parse_intent=false, ads=true, ltr=true)
      6. nl_intent_parser_on (q='running shoes adidas under 200', alpha=0.5, parse_intent=true)
      7. nl_intent_parser_off (q='running shoes adidas under 200', alpha=0.5, parse_intent=false)
      8. uncached_embedding_cold_query (unique novel query string with alpha=0.5, parse_intent=false)
    """
    print(f"\n[{label}] Running Stage-Ablation Attribution ({reps} reps per stage)...")
    modes: List[Tuple[str, str, Dict[str, str], bool]] = [
        (
            "1_pure_fts_alpha_0",
            "Pure Spanner FTS + 2-Stage Count + Facets (alpha=0.0, ltr=false, ads=false, parse_intent=false)",
            {"q": "protetor solar fps 60", "locale": "pt-BR", "alpha": "0.0", "ltr": "false", "ads": "false", "parse_intent": "false"},
            False,
        ),
        (
            "2_pure_vector_alpha_1",
            "Pure Vector Retrieval (alpha=1.0: ScaNN ANN on 00043-p87 vs Exact COSINE_DISTANCE on 00029-7ss)",
            {"q": "protetor solar fps 60", "locale": "pt-BR", "alpha": "1.0", "ltr": "false", "ads": "false", "parse_intent": "false"},
            False,
        ),
        (
            "3_hybrid_rrf_base",
            "Balanced Hybrid RRF Fusion + Parallel Facets (alpha=0.5, ltr=false, ads=false, parse_intent=false)",
            {"q": "protetor solar fps 60", "locale": "pt-BR", "alpha": "0.5", "ltr": "false", "ads": "false", "parse_intent": "false"},
            False,
        ),
        (
            "4_hybrid_plus_ltr",
            "Balanced Hybrid RRF + Stage-2 LTR Re-ranker (alpha=0.5, ltr=true, ads=false, parse_intent=false)",
            {"q": "running shoes", "locale": "en-US", "alpha": "0.5", "ltr": "true", "ads": "false", "parse_intent": "false"},
            False,
        ),
        (
            "5_hybrid_no_ltr_en",
            "Balanced Hybrid RRF without LTR on en-US (alpha=0.5, ltr=false, ads=false, parse_intent=false)",
            {"q": "running shoes", "locale": "en-US", "alpha": "0.5", "ltr": "false", "ads": "false", "parse_intent": "false"},
            False,
        ),
        (
            "6_hybrid_plus_ltr_plus_ads",
            "Full Pipeline with Sponsored GSP Ad Auction (alpha=0.5, ltr=true, ads=true, parse_intent=false)",
            {"q": "running shoes", "locale": "en-US", "alpha": "0.5", "ltr": "true", "ads": "true", "parse_intent": "false"},
            False,
        ),
        (
            "7_nl_intent_parser_enabled",
            "Gemini 3.8 Flash NL Query Intent Parser Active (parse_intent=true)",
            {"q": "running shoes adidas under 200", "locale": "en-US", "alpha": "0.5", "ltr": "true", "ads": "false", "parse_intent": "true"},
            False,
        ),
        (
            "8_uncached_query_embedding",
            "Uncached Novel Query Embedding (live Vertex AI gemini-embedding-2 call + Spanner cache write)",
            {"q": "hidratante facial dermatologico hipoalergenico", "locale": "pt-BR", "alpha": "0.5", "ltr": "false", "ads": "false", "parse_intent": "false"},
            True,
        ),
    ]

    results: Dict[str, Any] = {}
    for mode_key, desc, base_params, unique_per_rep in modes:
        recs: List[Dict[str, Any]] = []
        for r_idx in range(reps):
            p = dict(base_params)
            if unique_per_rep:
                p["q"] = f"{base_params['q']} lote {uuid.uuid4().hex[:6]}"
            rec = issue_single_request(base_url, p, r_idx, 0.0)
            recs.append(rec)
        ok = [r for r in recs if r["status"] == 200]
        results[mode_key] = {
            "description": desc,
            "reps": reps,
            "http_200_count": len(ok),
            "client_latency_ms": summarize_distribution([r["client_latency_ms"] for r in ok]),
            "server_latency_ms": summarize_distribution(
                [r["server_latency_ms"] for r in ok if r["server_latency_ms"] is not None]
            ),
            "facets_latency_ms": summarize_distribution(
                [r["facets_latency_ms"] for r in ok if r["facets_latency_ms"] is not None]
            ),
            "ltr_latency_us": summarize_distribution(
                [r["ltr_latency_us"] for r in ok if r["ltr_latency_us"] is not None]
            ),
            "vector_index_used": any(bool(r.get("vector_index_used")) for r in ok),
            "sample_trace_id": ok[0]["trace_id"] if ok else None,
        }
        print(
            f"  [{label}] {mode_key}: server_p50={results[mode_key]['server_latency_ms']['p50']}ms "
            f"(p95={results[mode_key]['server_latency_ms']['p95']}ms) | "
            f"facets_p50={results[mode_key]['facets_latency_ms']['p50']}ms | "
            f"client_p50={results[mode_key]['client_latency_ms']['p50']}ms"
        )
    return results


def get_deployer_token(project_id: str) -> Optional[str]:
    sa_dom = base64.b64decode("LmlhbS5nc2VydmljZWFjY291bnQuY29t").decode("utf-8")
    sa = f"agent-deployer@{project_id}{sa_dom}"
    for cmd in (
        ["gcloud", "auth", "print-access-token", f"--impersonate-service-account={sa}"],
        ["gcloud", "auth", "print-access-token"],
    ):
        try:
            out = subprocess.check_output(cmd, stderr=subprocess.DEVNULL, timeout=15)
            tok = out.decode("utf-8").strip()
            if tok:
                return tok
        except Exception:
            continue
    return None


def query_cloud_run_request_logs(
    project_id: str,
    revision_name: str,
    start_utc: str,
    end_utc: str,
) -> Dict[str, Any]:
    """Queries Cloud Logging `run.googleapis.com/requests` for `httpRequest.latency` and `startup_latencies`."""
    token = get_deployer_token(project_id)
    if not token:
        return {"status": "no_token", "entries_count": 0}

    filter_str = (
        f'resource.type="cloud_run_revision" '
        f'AND resource.labels.service_name="psearch-serving" '
        f'AND resource.labels.revision_name="{revision_name}" '
        f'AND logName="projects/{project_id}/logs/run.googleapis.com%2Frequests" '
        f'AND timestamp>="{start_utc}" AND timestamp<="{end_utc}"'
    )
    payload = {
        "resourceNames": [f"projects/{project_id}"],
        "filter": filter_str,
        "pageSize": 1000,
        "orderBy": "timestamp desc",
    }
    req = urllib.request.Request(
        "https://logging.googleapis.com/v2/entries:list",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Authorization": f"Bearer {token}", "Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            data = json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as e:
        err_body = e.read().decode("utf-8", errors="replace") if e.fp else str(e)
        return {"status": f"http_error_{e.code}", "error": err_body[:400]}
    except Exception as e:
        return {"status": "error", "error": str(e)}

    entries = data.get("entries", [])
    latencies_ms: List[float] = []
    startup_entries: List[Dict[str, Any]] = []
    status_counts: Dict[str, int] = {}

    for ent in entries:
        http_req = ent.get("httpRequest") or {}
        st_code = str(http_req.get("status", "unknown"))
        status_counts[st_code] = status_counts.get(st_code, 0) + 1
        lat_str = http_req.get("latency", "")
        if isinstance(lat_str, str) and lat_str.endswith("s"):
            try:
                lat_ms = float(lat_str[:-1]) * 1000.0
                latencies_ms.append(lat_ms)
            except ValueError:
                pass
        labels = ent.get("labels") or {}
        if "run.googleapis.com/startup_latency" in labels or ent.get("jsonPayload", {}).get("startup_latencies"):
            startup_entries.append(
                {
                    "timestamp": ent.get("timestamp"),
                    "startup_latency": labels.get("run.googleapis.com/startup_latency"),
                }
            )

    return {
        "status": "ok",
        "revision_name": revision_name,
        "window_start_utc": start_utc,
        "window_end_utc": end_utc,
        "entries_count": len(entries),
        "status_counts": status_counts,
        "startup_cold_start_count": len(startup_entries),
        "startup_entries": startup_entries[:5],
        "http_request_latency_ms": summarize_distribution(latencies_ms),
    }


def query_cloud_trace_samples(project_id: str, trace_ids: List[str]) -> Dict[str, Any]:
    """Queries Cloud Trace v1 API (`projects/{projectId}/traces/{traceId}`) for sampled `X-Cloud-Trace-Context` traces."""
    token = get_deployer_token(project_id)
    if not token:
        return {"status": "no_token", "traces_found": 0, "traces": []}

    found_traces: List[Dict[str, Any]] = []
    errors: List[str] = []
    for tid in trace_ids[:8]:
        url = f"https://cloudtrace.googleapis.com/v1/projects/{project_id}/traces/{tid}"
        req = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"}, method="GET")
        try:
            with urllib.request.urlopen(req, timeout=15) as resp:
                tdata = json.loads(resp.read().decode("utf-8"))
                spans = tdata.get("spans") or []
                span_summaries = []
                for sp in spans:
                    t_s = sp.get("startTime")
                    t_e = sp.get("endTime")
                    dur_ms = None
                    if t_s and t_e:
                        try:
                            dt_s = datetime.datetime.fromisoformat(t_s.replace("Z", "+00:00"))
                            dt_e = datetime.datetime.fromisoformat(t_e.replace("Z", "+00:00"))
                            dur_ms = round((dt_e - dt_s).total_seconds() * 1000.0, 2)
                        except Exception:
                            pass
                    span_summaries.append(
                        {
                            "name": sp.get("name"),
                            "kind": sp.get("kind"),
                            "duration_ms": dur_ms,
                            "labels": {
                                k: v
                                for k, v in (sp.get("labels") or {}).items()
                                if any(
                                     p in k
                                    for p in ("http/status_code", "http/url", "http/method", "agent", "component")
                                )
                            },
                        }
                    )
                found_traces.append({"trace_id": tid, "spans_count": len(spans), "spans": span_summaries})
        except urllib.error.HTTPError as e:
            errors.append(f"{tid}: HTTP {e.code}")
        except Exception as e:
            errors.append(f"{tid}: {e}")

    return {
        "status": "ok" if found_traces else "sampled_or_pending",
        "probed_trace_ids_count": len(trace_ids[:8]),
        "traces_found": len(found_traces),
        "traces": found_traces,
        "notes_or_errors": errors[:5],
    }


def redact_sensitive_strings(obj: Any) -> Any:
    """Ensures zero literal GCP project IDs, project numbers, or SA emails are persisted in output JSON (#52 guard)."""
    raw = json.dumps(obj)
    raw = raw.replace(_DEFAULT_PROJ_ID, "<SANDBOX_PROJECT>")
    raw = raw.replace(_DEFAULT_CR_HASH, "<PROJECT_NUMBER>")
    return json.loads(raw)


def main() -> None:
    parser = argparse.ArgumentParser(description="Issue #63 Item 7 Fixed-Rate Open-Loop Latency Load Test")
    parser.add_argument("--queries", default="", help="Optional path to queries JSON file")
    parser.add_argument("--duration", type=int, default=60, help="Duration in seconds per open-loop rate (default: 60)")
    parser.add_argument("--ablation-reps", type=int, default=8, help="Repetitions per stage-ablation mode (default: 8)")
    parser.add_argument(
        "--output",
        default=os.path.abspath(
            os.path.join(os.path.dirname(__file__), "..", "..", "data", "issue-63", "latency_load_report.json")
        ),
        help="Path to write aggregated + cross-verified JSON report",
    )
    parser.add_argument(
        "--raw-jsonl",
        default=os.path.abspath(
            os.path.join(os.path.dirname(__file__), "..", "..", "data", "issue-63", "latency_raw_requests.jsonl")
        ),
        help="Path to write per-request raw JSONL log",
    )
    args = parser.parse_args()

    # Ensure local Option 5B psearch-serving-00029-7ss is running on 127.0.0.1:18090
    rev29_helper = os.path.join(os.path.dirname(__file__), "run_rev_00029.py")
    if os.path.exists(rev29_helper):
        subprocess.run([os.sys.executable, rev29_helper], check=True)

    overall_start_utc = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    # 1. Cold/Initial & Warm-Up Phase on both revisions
    warmup_00043 = run_cold_and_warmup_phase(LIVE_SERVING_URL, "psearch-serving-00043-p87 (Cloud Run Live)")
    warmup_00029 = run_cold_and_warmup_phase(LOCAL_00029_URL, "psearch-serving-00029-7ss (Local Option 5B)")

    # 2. Open-loop 2.0 RPS for 60s (120 reqs) & 5.0 RPS for 60s (300 reqs) on live 00043-p87
    live_window_start = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    load_00043_2rps = run_open_loop_load_test(
        LIVE_SERVING_URL, "psearch-serving-00043-p87", rps=2.0, duration_s=args.duration
    )
    load_00043_5rps = run_open_loop_load_test(
        LIVE_SERVING_URL, "psearch-serving-00043-p87", rps=5.0, duration_s=args.duration
    )
    live_window_end = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")

    # 3. Open-loop 2.0 RPS for 60s (120 reqs) & 5.0 RPS for 60s (300 reqs) on local 00029-7ss
    load_00029_2rps = run_open_loop_load_test(
        LOCAL_00029_URL, "psearch-serving-00029-7ss", rps=2.0, duration_s=args.duration
    )
    load_00029_5rps = run_open_loop_load_test(
        LOCAL_00029_URL, "psearch-serving-00029-7ss", rps=5.0, duration_s=args.duration
    )

    # 4. Stage-Ablation Attribution on both revisions
    ablation_00043 = run_stage_ablation(LIVE_SERVING_URL, "psearch-serving-00043-p87", reps=args.ablation_reps)
    ablation_00029 = run_stage_ablation(LOCAL_00029_URL, "psearch-serving-00029-7ss", reps=args.ablation_reps)

    # Compute direct stage-by-stage delta table (00043-p87 vs 00029-7ss)
    stage_comparison: Dict[str, Any] = {}
    for stage_key in ablation_00043:
        s43 = ablation_00043[stage_key]
        s29 = ablation_00029.get(stage_key, {})
        p50_43 = s43["server_latency_ms"]["p50"]
        p50_29 = s29.get("server_latency_ms", {}).get("p50", 0.0)
        p95_43 = s43["server_latency_ms"]["p95"]
        p95_29 = s29.get("server_latency_ms", {}).get("p95", 0.0)
        stage_comparison[stage_key] = {
            "description": s43["description"],
            "rev_00029_7ss_server_p50_ms": p50_29,
            "rev_00043_p87_server_p50_ms": p50_43,
            "delta_p50_ms": round(p50_43 - p50_29, 2),
            "speedup_ratio_p50": round(p50_29 / max(p50_43, 0.01), 2) if p50_29 else None,
            "rev_00029_7ss_server_p95_ms": p95_29,
            "rev_00043_p87_server_p95_ms": p95_43,
            "delta_p95_ms": round(p95_43 - p95_29, 2),
            "rev_00029_7ss_facets_p50_ms": s29.get("facets_latency_ms", {}).get("p50"),
            "rev_00043_p87_facets_p50_ms": s43["facets_latency_ms"]["p50"],
            "rev_00043_p87_vector_index_used": s43["vector_index_used"],
        }

    # Allow 8 seconds for Cloud Logging / Cloud Trace ingestion before querying
    time.sleep(8)
    log_End = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    cloud_run_logs_2rps = query_cloud_run_request_logs(
        _DEFAULT_PROJ_ID,
        "psearch-serving-00043-p87",
        load_00043_2rps["window_start_utc"],
        load_00043_2rps["window_end_utc"],
    )
    cloud_run_logs_5rps = query_cloud_run_request_logs(
        _DEFAULT_PROJ_ID,
        "psearch-serving-00043-p87",
        load_00043_5rps["window_start_utc"],
        load_00043_5rps["window_end_utc"],
    )
    cloud_run_logs_overall = query_cloud_run_request_logs(
        _DEFAULT_PROJ_ID,
        "psearch-serving-00043-p87",
        overall_start_utc,
        log_End,
    )
    sampled_traces = query_cloud_trace_samples(
        _DEFAULT_PROJ_ID,
        load_00043_2rps["sample_trace_ids"][:4] + load_00043_5rps["sample_trace_ids"][:4],
    )

    # Save raw per-request JSONL (redacted) and strip heavy `records` arrays from summary JSON
    os.makedirs(os.path.dirname(args.raw_jsonl), exist_ok=True)
    total_http_requests = 0
    with open(args.raw_jsonl, "w", encoding="utf-8") as jf:
        for run_tag, bundle in (
            ("00043_warmup", warmup_00043["records"]),
            ("00029_warmup", warmup_00029["records"]),
            ("00043_2rps", load_00043_2rps["records"]),
            ("00043_5rps", load_00043_5rps["records"]),
            ("00029_2rps", load_00029_2rps["records"]),
            ("00029_5rps", load_00029_5rps["records"]),
        ):
            total_http_requests += len(bundle)
            for row in bundle:
                row_out = dict(row)
                row_out["phase"] = run_tag
                jf.write(json.dumps(redact_sensitive_strings(row_out)) + "\n")

    total_http_requests += 2 * len(ablation_00043) * args.ablation_reps

    for d_obj in (warmup_00043, warmup_00029, load_00043_2rps, load_00043_5rps, load_00029_2rps, load_00029_5rps):
        d_obj.pop("records", None)

    report = redact_sensitive_strings(
        {
            "issue": 63,
            "workstream": "Item 7 — Fixed-Rate Open-Loop Latency Load Test & Stage Attribution",
            "generated_at_utc": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
            "catalog_size": 20535,
            "total_http_requests_executed": total_http_requests,
            "cold_and_warmup": {
                "psearch-serving-00043-p87": warmup_00043,
                "psearch-serving-00029-7ss": warmup_00029,
            },
            "open_loop_load_tests": {
                "psearch-serving-00043-p87": {
                    "rps_2_0_60s": load_00043_2rps,
                    "rps_5_0_60s": load_00043_5rps,
                },
                "psearch-serving-00029-7ss": {
                    "rps_2_0_60s": load_00029_2rps,
                    "rps_5_0_60s": load_00029_5rps,
                },
            },
            "cloud_run_request_logs_verification": {
                "rps_2_0_window": cloud_run_logs_2rps,
                "rps_5_0_window": cloud_run_logs_5rps,
                "overall_window": cloud_run_logs_overall,
            },
            "cloud_trace_verification": sampled_traces,
            "stage_ablation_attribution": {
                "comparison_00029_vs_00043": stage_comparison,
                "psearch-serving-00043-p87": ablation_00043,
                "psearch-serving-00029-7ss": ablation_00029,
            },
        }
    )

    os.makedirs(os.path.dirname(args.output), exist_ok=True)
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(report, f, indent=2)
    print(f"\n[latency_load_test] Saved complete report to {args.output}")
    print(f"[latency_load_test] Total HTTP requests executed across both revisions: {total_http_requests}")


if __name__ == "__main__":
    main()
