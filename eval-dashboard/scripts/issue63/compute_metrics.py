#!/usr/bin/env python3
"""
Issue #63 — Items 4 & 5: TREC / ir_measures Metric Computation, 10,000-Resample Paired Bootstrap CIs,
Per-Query Delta Distributions, Organic vs. Sponsored Split, and Same-Window Revision Comparison
(2026-10-14 psearch-serving-00029-7ss vs. Current psearch-serving-00043-p87).

Usage:
  python3 eval-dashboard/scripts/issue63/compute_metrics.py \
    --queries eval-dashboard/data/issue-63/queries_dev_80.json \
    --qrels eval-dashboard/data/issue-63/qrels_dev_80.tsv \
    --runs-dir eval-dashboard/data/issue-63
"""

import argparse
from datetime import datetime, timezone
import json
import os
import subprocess
import sys

try:
    import numpy as np
    import pytrec_eval
    import ir_measures
    from ir_measures import nDCG, RR, R
except ImportError:
    if "-h" not in sys.argv and "--help" not in sys.argv:
        import urllib.request
        if subprocess.call([sys.executable, "-m", "pip", "--version"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL) != 0:
            urllib.request.urlretrieve("https://bootstrap.pypa.io/get-pip.py", "/tmp/get-pip.py")
            subprocess.check_call([sys.executable, "/tmp/get-pip.py", "--user", "--break-system-packages", "--quiet"])
        subprocess.check_call(
            [sys.executable, "-m", "pip", "install", "--user", "--quiet", "--break-system-packages", "numpy", "scipy", "pytrec-eval-terrier", "ir-measures"]
        )
        import site
        site.main()
        import numpy as np
        import pytrec_eval
        import ir_measures
        from ir_measures import nDCG, RR, R

CONFIG_SPECS = [
    ("C1", "c1_lexical", "C1: Pure Lexical FTS (alpha=0.0, no-LTR, no-ads) [00043-p87]"),
    ("C2", "c2_vector", "C2: Pure Vector Semantic (alpha=1.0, no-LTR, no-ads) [00043-p87]"),
    ("C3", "c3_hybrid_025", "C3: Hybrid RRF (alpha=0.25, no-LTR, no-ads) [00043-p87]"),
    ("C4", "c4_hybrid_ltr", "C4: Hybrid + LTR (alpha=0.50, LTR=true, no-ads) [00043-p87]"),
    ("C5", "c5_full_stack_00043", "C5: Full Production Stack (alpha=0.50, LTR=true, ads=true, NL=true) [00043-p87]"),
    ("C6", "c6_rev_00029_7ss", "C6: 2026-10-14 Revision (psearch-serving-00029-7ss local vs today's Spanner DB)"),
]


def load_queries(queries_path: str) -> list:
    with open(queries_path, "r", encoding="utf-8") as f:
        data = json.load(f)
    raw_list = data["queries"] if isinstance(data, dict) and "queries" in data else data
    normalized = []
    for idx, item in enumerate(raw_list, 1):
        if isinstance(item, str):
            normalized.append({"query_id": f"q{idx:02d}", "query": item, "stratum": "holdout", "locale": "pt-BR"})
        elif isinstance(item, dict):
            q_obj = dict(item)
            q_obj["query_id"] = str(item.get("query_id") or item.get("id") or f"q{idx:02d}")
            q_obj["query"] = str(item.get("query") or item.get("q") or item.get("text") or "")
            q_obj.setdefault("stratum", "holdout")
            q_obj.setdefault("locale", "pt-BR")
            normalized.append(q_obj)
    return normalized


def load_qrels(qrels_path: str) -> tuple[dict, dict]:
    """
    Returns:
      qrels_graded: {qid: {doc_id: int_grade_0_to_3}}
      qrels_binary_ge2: {qid: {doc_id: 1 if grade >= 2 else 0}}
    """
    qrels_graded = {}
    qrels_binary = {}
    with open(qrels_path, "r", encoding="utf-8") as f:
        for line in f:
            parts = line.strip().split()
            if len(parts) < 4:
                continue
            qid, _, doc_id, grade_str = parts[0], parts[1], parts[2], parts[3]
            g = int(grade_str)
            qrels_graded.setdefault(qid, {})[doc_id] = g
            qrels_binary.setdefault(qid, {})[doc_id] = 1 if g >= 2 else 0
    return qrels_graded, qrels_binary


def is_sponsored_result(item: dict) -> bool:
    if not isinstance(item, dict):
        return False
    if item.get("is_sponsored") is True or item.get("sponsored") is True:
        return True
    score = item.get("score") or {}
    return float(score.get("ad_relevance", 0) or 0) > 0 or int(score.get("cpc_micros", 0) or 0) > 0


def build_trec_runs_from_jsonl(
    jsonl_path: str,
    trec_combined_out: str,
    trec_organic_out: str,
    run_tag: str,
) -> tuple[dict, dict, dict]:
    """
    Reads raw_responses_<slug>.jsonl, writes standard 6-column TREC run files for Combined SERP
    and Organic-Only SERP, and returns:
      run_combined: {qid: {doc_id: score}}
      run_organic:  {qid: {doc_id: score}}
      raw_by_qid:   {qid: full_row_dict}
    """
    run_combined = {}
    run_organic = {}
    raw_by_qid = {}

    with open(jsonl_path, "r", encoding="utf-8") as f, \
         open(trec_combined_out, "w", encoding="utf-8") as fc, \
         open(trec_organic_out, "w", encoding="utf-8") as fo:
        for line in f:
            if not line.strip():
                continue
            row = json.loads(line)
            qid = row["query_id"]
            raw_by_qid[qid] = row
            results = (row.get("response_body") or {}).get("results") or []

            run_combined[qid] = {}
            for rank_idx, r in enumerate(results, 1):
                pid = r.get("id")
                if not pid or pid in run_combined[qid]:
                    continue
                # Monotonically decreasing rank score preserves exact SERP rank ordering in trec_eval
                synthetic_score = round(1000.0 - rank_idx * 1.0, 4)
                run_combined[qid][pid] = synthetic_score
                fc.write(f"{qid} Q0 {pid} {rank_idx} {synthetic_score:.4f} {run_tag}_combined\n")

            run_organic[qid] = {}
            org_results = [r for r in results if not is_sponsored_result(r)]
            for rank_idx, r in enumerate(org_results, 1):
                pid = r.get("id")
                if not pid or pid in run_organic[qid]:
                    continue
                synthetic_score = round(1000.0 - rank_idx * 1.0, 4)
                run_organic[qid][pid] = synthetic_score
                fo.write(f"{qid} Q0 {pid} {rank_idx} {synthetic_score:.4f} {run_tag}_organic\n")

    return run_combined, run_organic, raw_by_qid


def evaluate_run_pytrec_and_ir_measures(
    qrels_graded: dict,
    qrels_binary: dict,
    run_dict: dict,
    query_ids: list,
) -> dict:
    """
    Computes per-query metrics using BOTH pytrec_eval and ir_measures:
      - ndcg_at_10 (graded 0..3)
      - ndcg_at_5  (graded 0..3)
      - mrr_at_10  (binary g >= 2)
      - recall_at_10 (binary g >= 2)
      - recall_at_50 (binary g >= 2)
    """
    # 1. pytrec_eval graded evaluator for nDCG@10 and nDCG@5
    eval_graded = pytrec_eval.RelevanceEvaluator(qrels_graded, {"ndcg_cut_5", "ndcg_cut_10"})
    res_graded = eval_graded.evaluate(run_dict)

    # 2. pytrec_eval binary (g >= 2) evaluator for Recall@10, Recall@50
    eval_bin = pytrec_eval.RelevanceEvaluator(qrels_binary, {"recall_10", "recall_50", "recip_rank"})
    res_bin = eval_bin.evaluate(run_dict)

    # 3. ir_measures verification across the same qrels & run
    irm_measures = [nDCG @ 10, nDCG @ 5, RR(rel=2) @ 10, R(rel=2) @ 10, R(rel=2) @ 50]
    irm_per_q = {qid: {} for qid in query_ids}
    for m_val in ir_measures.iter_calc(irm_measures, qrels_graded, run_dict):
        if m_val.query_id in irm_per_q:
            irm_per_q[m_val.query_id][str(m_val.measure)] = float(m_val.value)

    per_query = {}
    for qid in query_ids:
        pg = res_graded.get(qid, {})
        pb = res_bin.get(qid, {})
        im = irm_per_q.get(qid, {})
        per_query[qid] = {
            "ndcg_at_10": float(im.get("nDCG@10", pg.get("ndcg_cut_10", 0.0))),
            "ndcg_at_10_pytrec": float(pg.get("ndcg_cut_10", 0.0)),
            "ndcg_at_5": float(im.get("nDCG@5", pg.get("ndcg_cut_5", 0.0))),
            "mrr_at_10": float(im.get("RR(rel=2)@10", pb.get("recip_rank", 0.0))),
            "recall_at_10": float(im.get("R(rel=2)@10", pb.get("recall_10", 0.0))),
            "recall_at_50": float(im.get("R(rel=2)@50", pb.get("recall_50", 0.0))),
        }
    return per_query


def bootstrap_ci(values: list, n_resamples: int = 10000, seed: int = 63) -> dict:
    arr = np.array(values, dtype=float)
    n = len(arr)
    if n == 0:
        return {"mean": 0.0, "ci95_low": 0.0, "ci95_high": 0.0, "half_width": 0.0, "std": 0.0, "n": 0}
    rng = np.random.default_rng(seed)
    idx = rng.integers(0, n, size=(n_resamples, n))
    means = np.mean(arr[idx], axis=1)
    low, high = np.percentile(means, [2.5, 97.5])
    mean_val = float(np.mean(arr))
    return {
        "mean": round(mean_val, 4),
        "ci95_low": round(float(low), 4),
        "ci95_high": round(float(high), 4),
        "half_width": round(float((high - low) / 2.0), 4),
        "std": round(float(np.std(arr, ddof=1)) if n > 1 else 0.0, 4),
        "n": n,
    }


def compute_paired_delta(
    per_q_a: dict,
    per_q_b: dict,
    queries: list,
    metric: str = "ndcg_at_10",
    n_resamples: int = 10000,
) -> dict:
    qids = [q["query_id"] for q in queries]
    q_lookup = {q["query_id"]: q for q in queries}
    deltas = []
    per_q_rows = []
    for qid in qids:
        va = per_q_a.get(qid, {}).get(metric, 0.0)
        vb = per_q_b.get(qid, {}).get(metric, 0.0)
        d = va - vb
        deltas.append(d)
        per_q_rows.append(
            {
                "query_id": qid,
                "query": q_lookup[qid]["query"],
                "stratum": q_lookup[qid].get("stratum", ""),
                "val_a": round(va, 4),
                "val_b": round(vb, 4),
                "delta": round(d, 4),
            }
        )

    ci = bootstrap_ci(deltas, n_resamples=n_resamples, seed=63)
    arr = np.array(deltas, dtype=float)
    q25, med, q75 = np.percentile(arr, [25, 50, 75]) if len(arr) > 0 else (0.0, 0.0, 0.0)
    improved = [r for r in per_q_rows if r["delta"] > 0.01]
    degraded = [r for r in per_q_rows if r["delta"] < -0.01]
    tied = [r for r in per_q_rows if abs(r["delta"]) <= 0.01]

    sorted_rows = sorted(per_q_rows, key=lambda x: x["delta"], reverse=True)
    return {
        "metric": metric,
        "mean_delta": ci["mean"],
        "ci95_low": ci["ci95_low"],
        "ci95_high": ci["ci95_high"],
        "ci95_half_width": ci["half_width"],
        "median_delta": round(float(med), 4),
        "iqr_delta": round(float(q75 - q25), 4),
        "min_delta": round(float(np.min(arr)), 4) if len(arr) > 0 else 0.0,
        "max_delta": round(float(np.max(arr)), 4) if len(arr) > 0 else 0.0,
        "improved_queries_count": len(improved),
        "degraded_queries_count": len(degraded),
        "tied_queries_count": len(tied),
        "top_5_gains": sorted_rows[:5],
        "top_5_regressions": list(reversed(sorted_rows[-5:])),
        "per_query_deltas": per_q_rows,
    }


def compute_top10_jaccard(raw_a: dict, raw_b: dict, qids: list) -> float:
    scores = []
    for qid in qids:
        res_a = ((raw_a.get(qid, {}).get("response_body") or {}).get("results") or [])[:10]
        res_b = ((raw_b.get(qid, {}).get("response_body") or {}).get("results") or [])[:10]
        set_a = {r.get("id") for r in res_a if r.get("id")}
        set_b = {r.get("id") for r in res_b if r.get("id")}
        if not set_a and not set_b:
            scores.append(1.0)
        elif not set_a or not set_b:
            scores.append(0.0)
        else:
            scores.append(len(set_a & set_b) / float(len(set_a | set_b)))
    return round(float(np.mean(scores)), 4) if scores else 0.0


def main() -> None:
    parser = argparse.ArgumentParser(description="Compute trec_eval / ir_measures metrics and bootstrap CIs.")
    parser.add_argument("--queries", required=True, help="Path to queries JSON file")
    parser.add_argument("--qrels", required=True, help="Path to TREC qrels TSV file")
    parser.add_argument("--runs-dir", "--data-dir", dest="runs_dir", default="eval-dashboard/data/issue-63", help="Directory containing raw_responses_*.jsonl")
    parser.add_argument("--out", "--out-json", dest="out", default="", help="Output JSON metrics report path")
    args = parser.parse_args()

    out_path = args.out or os.path.join(args.runs_dir, "metrics_report_dev_80.json")
    queries = load_queries(args.queries)
    qrels_graded, qrels_binary = load_qrels(args.qrels)

    all_qids = [q["query_id"] for q in queries]
    pos_queries = [q for q in queries if q.get("stratum") != "negative_control_zero_results"]
    neg_queries = [q for q in queries if q.get("stratum") == "negative_control_zero_results"]
    pos_qids = [q["query_id"] for q in pos_queries]
    neg_qids = [q["query_id"] for q in neg_queries]

    strata = sorted({q.get("stratum", "unknown") for q in queries})
    qids_by_stratum = {
        s: [q["query_id"] for q in queries if q.get("stratum") == s]
        for s in strata
    }

    per_config_summary = {}
    per_config_per_query_combined = {}
    per_config_per_query_organic = {}
    raw_by_config = {}

    for cfg_id, slug, label in CONFIG_SPECS:
        jsonl_path = os.path.join(args.runs_dir, f"raw_responses_{slug}.jsonl")
        if not os.path.exists(jsonl_path):
            print(f"[compute_metrics] Skipping missing {jsonl_path}")
            continue
        trec_comb = os.path.join(args.runs_dir, f"trec_run_{slug}_combined.txt")
        trec_org = os.path.join(args.runs_dir, f"trec_run_{slug}_organic_only.txt")
        run_comb, run_org, raw_map = build_trec_runs_from_jsonl(jsonl_path, trec_comb, trec_org, slug)
        raw_by_config[cfg_id] = raw_map

        pq_comb = evaluate_run_pytrec_and_ir_measures(qrels_graded, qrels_binary, run_comb, all_qids)
        pq_org = evaluate_run_pytrec_and_ir_measures(qrels_graded, qrels_binary, run_org, all_qids)
        per_config_per_query_combined[cfg_id] = pq_comb
        per_config_per_query_organic[cfg_id] = pq_org

        # Positive-recall metrics (N=70 on dev set)
        metrics_pos = {}
        for m in ("ndcg_at_10", "ndcg_at_5", "mrr_at_10", "recall_at_10", "recall_at_50"):
            vals = [pq_comb[qid][m] for qid in pos_qids]
            metrics_pos[m] = bootstrap_ci(vals, n_resamples=10000, seed=63)

        metrics_pos_organic = {}
        for m in ("ndcg_at_10", "ndcg_at_5", "mrr_at_10", "recall_at_10", "recall_at_50"):
            vals = [pq_org[qid][m] for qid in pos_qids]
            metrics_pos_organic[m] = bootstrap_ci(vals, n_resamples=10000, seed=63)

        # All-80 queries metrics
        metrics_all80 = {}
        for m in ("ndcg_at_10", "ndcg_at_5", "mrr_at_10", "recall_at_10", "recall_at_50"):
            vals = [pq_comb[qid][m] for qid in all_qids]
            metrics_all80[m] = bootstrap_ci(vals, n_resamples=10000, seed=63)

        # Per-stratum breakdown
        by_stratum = {}
        for s, s_qids in qids_by_stratum.items():
            by_stratum[s] = {
                m: bootstrap_ci([pq_comb[qid][m] for qid in s_qids], n_resamples=10000, seed=63)
                for m in ("ndcg_at_10", "ndcg_at_5", "mrr_at_10", "recall_at_10", "recall_at_50")
            }

        # Negative-control diagnostics (Stratum 6)
        neg_zero_cnt = 0
        neg_fp_counts = []
        for qid in neg_qids:
            res_list = ((raw_map.get(qid, {}).get("response_body") or {}).get("results") or [])[:10]
            if len(res_list) == 0:
                neg_zero_cnt += 1
            neg_fp_counts.append(len(res_list))

        per_config_summary[cfg_id] = {
            "config_id": cfg_id,
            "slug": slug,
            "label": label,
            "trec_run_combined_file": trec_comb,
            "trec_run_organic_file": trec_org,
            "positive_queries_n": len(pos_qids),
            "all_queries_n": len(all_qids),
            "combined_serp_positive_queries": metrics_pos,
            "organic_only_positive_queries": metrics_pos_organic,
            "combined_serp_all_queries": metrics_all80,
            "by_stratum": by_stratum,
            "negative_control_diagnostics": {
                "negative_queries_n": len(neg_qids),
                "clean_zero_result_count": neg_zero_cnt,
                "clean_zero_result_rate_pct": round((neg_zero_cnt / max(len(neg_qids), 1)) * 100.0, 2),
                "false_positive_query_rate_pct": round(((len(neg_qids) - neg_zero_cnt) / max(len(neg_qids), 1)) * 100.0, 2),
                "mean_false_positive_items_at_10": round(float(np.mean(neg_fp_counts)) if neg_fp_counts else 0.0, 2),
            },
        }

    # Paired bootstrap comparisons across configurations (on positive-recall queries)
    paired_comparisons = {
        "C5_full_vs_C1_lexical": compute_paired_delta(
            per_config_per_query_combined["C5"], per_config_per_query_combined["C1"], pos_queries, "ndcg_at_10"
        ),
        "C5_full_vs_C2_vector": compute_paired_delta(
            per_config_per_query_combined["C5"], per_config_per_query_combined["C2"], pos_queries, "ndcg_at_10"
        ),
        "C3_hybrid025_vs_C1_lexical": compute_paired_delta(
            per_config_per_query_combined["C3"], per_config_per_query_combined["C1"], pos_queries, "ndcg_at_10"
        ),
        "C4_hybrid_ltr_vs_C3_hybrid025": compute_paired_delta(
            per_config_per_query_combined["C4"], per_config_per_query_combined["C3"], pos_queries, "ndcg_at_10"
        ),
        "C5_full_vs_C4_hybrid_ltr": compute_paired_delta(
            per_config_per_query_combined["C5"], per_config_per_query_combined["C4"], pos_queries, "ndcg_at_10"
        ),
        # Item 5: Same-window revision comparisons (Current 00043-p87 vs 2026-10-14 00029-7ss)
        "item5_C5_00043_vs_C6_00029_ndcg10": compute_paired_delta(
            per_config_per_query_combined["C5"], per_config_per_query_combined["C6"], pos_queries, "ndcg_at_10"
        ),
        "item5_C5_00043_vs_C6_00029_mrr10": compute_paired_delta(
            per_config_per_query_combined["C5"], per_config_per_query_combined["C6"], pos_queries, "mrr_at_10"
        ),
        "item5_C5_00043_vs_C6_00029_recall10": compute_paired_delta(
            per_config_per_query_combined["C5"], per_config_per_query_combined["C6"], pos_queries, "recall_at_10"
        ),
        "item5_C4_00043_vs_C6_00029_ndcg10": compute_paired_delta(
            per_config_per_query_combined["C4"], per_config_per_query_combined["C6"], pos_queries, "ndcg_at_10"
        ),
        "item5_C3_00043_vs_C6_00029_ndcg10": compute_paired_delta(
            per_config_per_query_combined["C3"], per_config_per_query_combined["C6"], pos_queries, "ndcg_at_10"
        ),
    }

    # Sponsored vs Organic Analysis (Item 4)
    c5_raw = raw_by_config.get("C5", {})
    dev_sponsored_impressions = []
    for q in queries:
        qid = q["query_id"]
        body = (c5_raw.get(qid, {}).get("response_body") or {})
        scnt = int(body.get("sponsored_count", 0) or 0)
        results = body.get("results") or []
        spons_items = [r for r in results if is_sponsored_result(r)]
        if scnt > 0 or spons_items:
            for s_it in spons_items:
                pid = s_it.get("id")
                dev_sponsored_impressions.append(
                    {
                        "query_id": qid,
                        "query": q["query"],
                        "stratum": q.get("stratum"),
                        "sponsored_product_id": pid,
                        "title": s_it.get("title"),
                        "ad_relevance": (s_it.get("score") or {}).get("ad_relevance", 0.0),
                        "cpc_micros": (s_it.get("score") or {}).get("cpc_micros", 0),
                        "rubric_grade": qrels_graded.get(qid, {}).get(pid, 0),
                    }
                )

    aux_ads_path = os.path.join(args.runs_dir, "raw_responses_aux_ads_en_us.jsonl")
    aux_ads_impressions = []
    if os.path.exists(aux_ads_path):
        with open(aux_ads_path, "r", encoding="utf-8") as af:
            for line in af:
                if not line.strip():
                    continue
                row = json.loads(line)
                body = row.get("response_body") or {}
                results = body.get("results") or []
                spons_items = [r for r in results if is_sponsored_result(r)]
                org_items = [r for r in results if not is_sponsored_result(r)]
                aux_ads_impressions.append(
                    {
                        "query_id": row["query_id"],
                        "query": row["query"],
                        "locale": row["locale"],
                        "sponsored_count": int(body.get("sponsored_count", 0) or 0),
                        "slot0_sponsored_id": spons_items[0].get("id") if spons_items else None,
                        "slot0_sponsored_title": spons_items[0].get("title") if spons_items else None,
                        "slot0_ad_relevance": (spons_items[0].get("score") or {}).get("ad_relevance", 0.0) if spons_items else 0.0,
                        "slot0_cpc_micros": (spons_items[0].get("score") or {}).get("cpc_micros", 0) if spons_items else 0,
                        "first_organic_id": org_items[0].get("id") if org_items else None,
                        "first_organic_title": org_items[0].get("title") if org_items else None,
                    }
                )

    # Stratum 5 strict-locale vs unfiltered cross-lingual comparison
    strict_locale_path = os.path.join(args.runs_dir, "raw_responses_stratum5_strict_locale.jsonl")
    stratum5_strict_summary = []
    if os.path.exists(strict_locale_path):
        with open(strict_locale_path, "r", encoding="utf-8") as sf:
            for line in sf:
                if not line.strip():
                    continue
                row = json.loads(line)
                qid = row["query_id"]
                if qid not in per_config_per_query_combined.get("C5", {}):
                    continue
                results = ((row.get("response_body") or {}).get("results") or [])[:10]
                med_cnt = sum(1 for r in results if str(r.get("id", "")).startswith("med-"))
                prod_cnt = sum(1 for r in results if str(r.get("id", "")).startswith("prod-"))
                stratum5_strict_summary.append(
                    {
                        "query_id": qid,
                        "query": row["query"],
                        "strict_locale": row.get("strict_locale"),
                        "total_returned": len(results),
                        "real_catalog_med_count": med_cnt,
                        "demo_seed_prod_count": prod_cnt,
                        "unfiltered_c5_ndcg10": round(per_config_per_query_combined["C5"][qid]["ndcg_at_10"], 4),
                    }
                )

    report = {
        "generated_at_utc": datetime.now(timezone.utc).isoformat(),
        "queries_file": args.queries,
        "qrels_file": args.qrels,
        "evaluation_engine": "pytrec_eval 0.5.11 + ir_measures 0.4.3",
        "bootstrap_resamples": 10000,
        "revision_comparison_note": (
            "Option 5B: 2026-10-14 revision psearch-serving-00029-7ss (commit d02047) runs locally on 127.0.0.1:18090 "
            "against today's Spanner psearch-db (20,535 items), isolating the serving code changes between 00029-7ss and 00043-p87."
        ),
        "configurations": per_config_summary,
        "paired_comparisons": paired_comparisons,
        "revision_top10_jaccard": {
            "C5_00043_vs_C6_00029": compute_top10_jaccard(raw_by_config["C5"], raw_by_config["C6"], pos_qids),
            "C4_00043_vs_C6_00029": compute_top10_jaccard(raw_by_config["C4"], raw_by_config["C6"], pos_qids),
            "C3_00043_vs_C6_00029": compute_top10_jaccard(raw_by_config["C3"], raw_by_config["C6"], pos_qids),
        },
        "sponsored_vs_organic_audit": {
            "dev_80_queries_with_sponsored_ads": len(dev_sponsored_impressions),
            "dev_80_sponsored_impressions": dev_sponsored_impressions,
            "combined_vs_organic_c5_delta_ndcg10": compute_paired_delta(
                per_config_per_query_combined["C5"], per_config_per_query_organic["C5"], pos_queries, "ndcg_at_10"
            )["mean_delta"],
            "auxiliary_en_us_ad_campaigns_slice": aux_ads_impressions,
        },
        "stratum5_strict_locale_vs_unfiltered": stratum5_strict_summary,
    }

    with open(out_path, "w", encoding="utf-8") as out_f:
        json.dump(report, out_f, indent=2, ensure_ascii=False)

    print(f"[compute_metrics] Saved full metrics report to {out_path}")
    for cfg_id, _, label in CONFIG_SPECS:
        if cfg_id not in per_config_summary:
            continue
        m = per_config_summary[cfg_id]["combined_serp_positive_queries"]
        neg = per_config_summary[cfg_id]["negative_control_diagnostics"]
        print(
            f"  {cfg_id}: nDCG@10={m['ndcg_at_10']['mean']:.4f} (±{m['ndcg_at_10']['half_width']:.4f}) | "
            f"nDCG@5={m['ndcg_at_5']['mean']:.4f} | MRR@10={m['mrr_at_10']['mean']:.4f} | "
            f"Recall@10={m['recall_at_10']['mean']:.4f} | NegZeroRate={neg['clean_zero_result_rate_pct']:.1f}%"
        )


if __name__ == "__main__":
    main()
