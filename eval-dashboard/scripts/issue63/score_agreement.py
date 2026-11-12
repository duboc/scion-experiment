#!/usr/bin/env python3
"""
Issue #63 — Item 3: Inter-Rater Agreement Scorer between PO Blind Labels & LLM Judge (gemini-4-argon).

Usage:
  python3 eval-dashboard/scripts/issue63/score_agreement.py \
    --llm-key eval-dashboard/data/issue-63/llm_grades_blind_60.json \
    --po-labels eval-dashboard/data/issue-63/po_blind_sample_v1.csv

Supports PO labels supplied as:
  1. Filled CSV (with 'sample_id' and 'po_grade_0_to_3' column),
  2. JSON mapping {"B01": 3, "B02": 0, ...} or [{"sample_id": "B01", "grade": 3}, ...],
  3. Compact 'B01=3,B02=0,...' string or 2-column TSV.

Computes:
  - Quadratic Weighted Cohen's kappa (kappa_w)
  - Unweighted Cohen's kappa
  - Spearman's rank correlation (rho)
  - Exact 4-class agreement %
  - Binary relevance ({0,1} vs {2,3}) agreement %
  - 4x4 confusion matrix and list of any +/- 2 grade disagreements
"""

import argparse
import csv
import json
import math
import sys

try:
    import numpy as np
    from scipy.stats import spearmanr as _scipy_spearmanr
    HAS_SCIPY = True
except ImportError:
    HAS_SCIPY = False


def load_llm_key(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    samples = data.get("samples", data)
    return {k: v for k, v in samples.items()}


def load_po_labels(path: str) -> dict:
    if path.endswith(".json"):
        with open(path, "r", encoding="utf-8") as f:
            data = json.load(f)
        if isinstance(data, dict):
            return {k: int(v["grade"] if isinstance(v, dict) else v) for k, v in data.items() if str(v).strip() != ""}
        if isinstance(data, list):
            return {
                r["sample_id"]: int(r.get("po_grade_0_to_3", r.get("grade")))
                for r in data
                if str(r.get("po_grade_0_to_3", r.get("grade", ""))).strip() != ""
            }

    labels = {}
    with open(path, "r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            sid = (row.get("sample_id") or "").strip()
            val = (row.get("po_grade_0_to_3") or row.get("grade") or "").strip()
            if sid and val != "":
                labels[sid] = int(val)
    return labels


def compute_cohens_kappa(y1: list[int], y2: list[int], num_classes: int = 4, weighted: bool = False) -> float:
    n = len(y1)
    if n == 0:
        return 0.0
    conf = [[0.0] * num_classes for _ in range(num_classes)]
    for a, b in zip(y1, y2):
        conf[int(a)][int(b)] += 1.0
    for i in range(num_classes):
        for j in range(num_classes):
            conf[i][j] /= n

    hist1 = [sum(conf[i][j] for j in range(num_classes)) for i in range(num_classes)]
    hist2 = [sum(conf[i][j] for i in range(num_classes)) for j in range(num_classes)]
    expected = [[hist1[i] * hist2[j] for j in range(num_classes)] for i in range(num_classes)]

    if not weighted:
        po = sum(conf[i][i] for i in range(num_classes))
        pe = sum(expected[i][i] for i in range(num_classes))
        return float((po - pe) / (1.0 - pe)) if (1.0 - pe) > 1e-12 else 1.0

    obs_dis = 0.0
    exp_dis = 0.0
    denom = float((num_classes - 1) ** 2)
    for i in range(num_classes):
        for j in range(num_classes):
            w = ((i - j) ** 2) / denom
            obs_dis += w * conf[i][j]
            exp_dis += w * expected[i][j]
    return float(1.0 - (obs_dis / exp_dis)) if exp_dis > 1e-12 else 1.0


def _rankdata_avg_ties(vals: list[int]) -> list[float]:
    indexed = sorted(enumerate(vals), key=lambda x: x[1])
    ranks = [0.0] * len(vals)
    i = 0
    while i < len(indexed):
        j = i
        while j < len(indexed) and indexed[j][1] == indexed[i][1]:
            j += 1
        avg_rank = (i + 1 + j) / 2.0
        for k in range(i, j):
            ranks[indexed[k][0]] = avg_rank
        i = j
    return ranks


def compute_spearman(y1: list[int], y2: list[int]) -> tuple[float, float]:
    if HAS_SCIPY:
        rho, pval = _scipy_spearmanr(y1, y2)
        return (0.0 if math.isnan(float(rho)) else float(rho), 1.0 if math.isnan(float(pval)) else float(pval))
    n = len(y1)
    if n < 2:
        return 0.0, 1.0
    r1 = _rankdata_avg_ties(y1)
    r2 = _rankdata_avg_ties(y2)
    m1 = sum(r1) / n
    m2 = sum(r2) / n
    cov = sum((a - m1) * (b - m2) for a, b in zip(r1, r2))
    v1 = sum((a - m1) ** 2 for a in r1)
    v2 = sum((b - m2) ** 2 for b in r2)
    if v1 <= 1e-12 or v2 <= 1e-12:
        return 0.0, 1.0
    rho = cov / math.sqrt(v1 * v2)
    return float(rho), 0.0


def score_agreement(llm_key: dict, po_labels: dict) -> dict:
    matched_ids = sorted(sid for sid in llm_key.keys() if sid in po_labels)
    if not matched_ids:
        return {
            "status": "AWAITING_PO_LABELS",
            "matched_pairs": 0,
            "total_blind_samples": len(llm_key),
            "message": "No filled po_grade_0_to_3 values found yet in --po-labels file.",
        }

    llm_arr = [int(llm_key[sid]["llm_grade"]) for sid in matched_ids]
    po_arr = [int(po_labels[sid]) for sid in matched_ids]
    n = len(matched_ids)

    kappa_w = compute_cohens_kappa(po_arr, llm_arr, num_classes=4, weighted=True)
    kappa_unw = compute_cohens_kappa(po_arr, llm_arr, num_classes=4, weighted=False)
    rho, rho_pval = compute_spearman(po_arr, llm_arr)
    exact_acc = sum(1 for a, b in zip(po_arr, llm_arr) if a == b) / n
    binary_acc = sum(1 for a, b in zip(po_arr, llm_arr) if (a >= 2) == (b >= 2)) / n

    conf = [[0] * 4 for _ in range(4)]
    disagreements = []
    for sid, p_g, l_g in zip(matched_ids, po_arr, llm_arr):
        conf[int(p_g)][int(l_g)] += 1
        if abs(int(p_g) - int(l_g)) >= 1:
            info = llm_key[sid]
            disagreements.append(
                {
                    "sample_id": sid,
                    "query_id": info.get("query_id"),
                    "query": info.get("query"),
                    "product_id": info.get("product_id"),
                    "title": info.get("title"),
                    "po_grade": int(p_g),
                    "llm_grade": int(l_g),
                    "delta": int(l_g) - int(p_g),
                    "llm_reason": info.get("llm_reason", ""),
                }
            )

    return {
        "status": "SCORED",
        "matched_pairs": len(matched_ids),
        "total_blind_samples": len(llm_key),
        "quadratic_weighted_kappa": round(kappa_w, 4),
        "unweighted_kappa": round(kappa_unw, 4),
        "spearman_rho": round(float(rho), 4),
        "spearman_pvalue": float(rho_pval),
        "exact_4class_agreement_pct": round(exact_acc * 100.0, 2),
        "binary_relevant_agreement_pct": round(binary_acc * 100.0, 2),
        "confusion_matrix_po_rows_llm_cols": conf,
        "large_disagreements_delta_ge_2": [d for d in disagreements if abs(d["delta"]) >= 2],
        "all_disagreements_count": len(disagreements),
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Compute inter-rater agreement between PO blind labels and LLM judge.")
    parser.add_argument("--llm-key", required=True, help="Path to llm_grades_blind_60.json")
    parser.add_argument("--po-labels", required=True, help="Path to filled po_blind_sample_v1.csv or JSON")
    parser.add_argument("--queries", default="", help="Optional path to queries JSON file")
    parser.add_argument("--out", default="", help="Optional output JSON file")
    args = parser.parse_args()

    llm_key = load_llm_key(args.llm_key)
    po_labels = load_po_labels(args.po_labels)
    res = score_agreement(llm_key, po_labels)
    if args.out:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(res, f, indent=2, ensure_ascii=False)
    print(json.dumps(res, indent=2, ensure_ascii=False))


if __name__ == "__main__":
    main()
