#!/usr/bin/env python3
"""
Live Search Quality & SLA Probe Worker for @search-evaluator.
Evaluates psearch-serving across all 10 Biggy Search requirements,
measuring NDCG@10, MRR@10, and latency percentiles.
"""

import os
import sys
import time
import math
import json
import urllib.request
import urllib.parse
from datetime import datetime, timezone

try:
    from google.cloud import firestore
    HAS_FIRESTORE = True
except ImportError:
    HAS_FIRESTORE = False

PROJECT_ID = os.getenv("GCP_PROJECT", "riojucu-sandbox")
SERVING_URL = os.getenv("PSEARCH_SERVING_URL", "https://psearch-serving-mhjw244bgq-uc.a.run.app")

GOLDEN_QUERIES = [
    {"query": "running shoes", "locale": "en-US", "alpha": 0.65, "expected": ["prod-001", "prod-003", "prod-002"]},
    {"query": "tenis corrida", "locale": "pt-BR", "alpha": 0.65, "expected": ["prod-002", "prod-001", "prod-003"]},
    {"query": "developer laptop", "locale": "en-US", "alpha": 0.50, "expected": ["prod-005", "prod-007", "prod-006"]},
    {"query": "noise cancelling headphones", "locale": "en-US", "alpha": 0.50, "expected": ["prod-008", "prod-009"]},
    {"query": "geladeira inox", "locale": "pt-BR", "alpha": 0.50, "expected": ["prod-011", "prod-012"]},
    {"query": "cafeteira espresso", "locale": "pt-BR", "alpha": 0.50, "expected": ["prod-013"]},
]

def compute_ndcg_and_mrr(returned_ids, expected_ids, k=10):
    expected_set = set(expected_ids)
    dcg = 0.0
    mrr = 0.0
    for idx, doc_id in enumerate(returned_ids[:k]):
        if doc_id in expected_set:
            dcg += 1.0 / math.log2(idx + 2)
            if mrr == 0.0:
                mrr = 1.0 / (idx + 1)
    
    idcg = sum(1.0 / math.log2(i + 2) for i in range(min(len(expected_ids), k)))
    ndcg = (dcg / idcg) if idcg > 0.0 else 0.0
    return ndcg, mrr

def probe_endpoint(url, timeout=5.0):
    start = time.perf_counter()
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "psearch-eval/search-evaluator"})
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            elapsed_ms = (time.perf_counter() - start) * 1000.0
            data = json.loads(resp.read().decode("utf-8"))
            return data, elapsed_ms, resp.status
    except Exception as e:
        elapsed_ms = (time.perf_counter() - start) * 1000.0
        return None, elapsed_ms, 500

def run_probes():
    print(f"Executing Search Quality Probes against: {SERVING_URL}")
    latencies = []
    ndcgs = []
    mrrs = []

    for item in GOLDEN_QUERIES:
        q_params = urllib.parse.urlencode({
            "q": item["query"],
            "alpha": str(item["alpha"]),
            "locale": item["locale"],
            "limit": "10"
        })
        url = f"{SERVING_URL}/api/search?{q_params}"
        data, lat, status = probe_endpoint(url)
        latencies.append(lat)

        returned_ids = []
        if data and isinstance(data, dict):
            results = data.get("results", [])
            returned_ids = [r.get("id") for r in results if isinstance(r, dict)]

        ndcg, mrr = compute_ndcg_and_mrr(returned_ids, item["expected"])
        ndcgs.append(ndcg)
        mrrs.append(mrr)
        print(f"  Query: '{item['query']}' [{item['locale']}] -> Latency: {lat:.1f}ms, NDCG@10: {ndcg:.3f}, MRR@10: {mrr:.3f}")

    latencies.sort()
    p50 = latencies[len(latencies) // 2] if latencies else 0.0
    p95 = latencies[int(len(latencies) * 0.95)] if latencies else 0.0
    avg_ndcg = sum(ndcgs) / len(ndcgs) if ndcgs else 0.0
    avg_mrr = sum(mrrs) / len(mrrs) if mrrs else 0.0

    print("\n--- Summary Benchmark ---")
    print(f"NDCG@10: {avg_ndcg:.3f} | MRR@10: {avg_mrr:.3f} | p50: {p50:.1f}ms | p95: {p95:.1f}ms")

    if HAS_FIRESTORE:
        try:
            db = firestore.Client(project=PROJECT_ID)
            db.collection("eval_runs").add({
                "timestamp": firestore.SERVER_TIMESTAMP,
                "target_url": SERVING_URL,
                "ndcg_at_10": round(avg_ndcg, 3),
                "mrr_at_10": round(avg_mrr, 3),
                "p50_latency_ms": round(p50, 1),
                "p95_latency_ms": round(p95, 1),
                "queries_evaluated": len(GOLDEN_QUERIES)
            })
            print(f"[SUCCESS] Recorded eval run in Firestore ({PROJECT_ID})")
        except Exception as e:
            print(f"[WARN] Failed to write to Firestore: {e}", file=sys.stderr)

if __name__ == "__main__":
    run_probes()
