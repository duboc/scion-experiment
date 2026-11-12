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
import sys
import numpy as np
from scipy.stats import spearmanr


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


def compute_cohens_kappa(y1: np.ndarray, y2: np.ndarray, num_classes: int = 4, weighted: bool = False) -> float:
    n = len(y1)
    if n == 0:
        return 0.0
    conf = np.zeros((num_classes, num_classes), dtype=float)
    for a, b in zip(y1, y2):
        conf[int(a), int(b)] += 1.0
    conf /= n

    hist1 = np.sum(conf, axis=1)
    hist2 = np.sum(conf, axis=0)
    expected = np.outer(hist1, hist2)

    if not weighted:
        po = np.trace(conf)
        pe = np.trace(expected)
        return float((po - pe) / (1.0 - pe)) if (1.0 - pe) > 1e-12 else 1.0

    weights = np.zeros((num_classes, num_classes), dtype=float)
    for i in range(num_classes):
        for j in range(num_classes):
            weights[i, j] = ((i - j) ** 2) / ((num_classes - 1) ** 2)

    obs_dis = float(np.sum(weights * conf))
    exp_dis = float(np.sum(weights * expected))
    return float(1.0 - (obs_dis / exp_dis)) if exp_dis > 1e-12 else 1.0


def score_agreement(llm_key: dict, po_labels: dict) -> dict:
    matched_ids = sorted(sid for sid in llm_key.keys() if sid in po_labels)
    if not matched_ids:
        return {
            "status": "AWAITING_PO_LABELS",
            "matched_pairs": 0,
            "total_blind_samples": len(llm_key),
            "message": "No filled po_grade_0_to_3 values found yet in --po-labels file.",
        }

    llm_arr = np.array([int(llm_key[sid]["llm_grade"]) for sid in matched_ids], dtype=int)
    po_arr = np.array([int(po_labels[sid]) for sid in matched_ids], dtype=int)

    kappa_w = compute_cohens_kappa(po_arr, llm_arr, num_classes=4, weighted=True)
    kappa_unw = compute_cohens_kappa(po_arr, llm_arr, num_classes=4, weighted=False)
    rho, rho_pval = spearmanr(po_arr, llm_arr)
    exact_acc = float(np.mean(po_arr == llm_arr))
    bin_po = (po_arr >= 2).astype(int)
    bin_llm = (llm_arr >= 2).astype(int)
    binary_acc = float(np.mean(bin_po == bin_llm))

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
        "spearman_rho": round(float(rho), 4) if not np.isnan(rho) else 0.0,
        "spearman_pvalue": float(rho_pval) if not np.isnan(rho_pval) else 1.0,
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
