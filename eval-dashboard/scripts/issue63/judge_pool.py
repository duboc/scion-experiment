#!/usr/bin/env python3
"""
Issue #63 — Item 3: Source-Blinded Pool Relevance Judge (gemini-4-argon, 0-3 Rubric)
& 60-Pair Stratified Blind Sample Generator for PO Labeling.

Usage:
  python3 eval-dashboard/scripts/issue63/judge_pool.py \
    --queries eval-dashboard/data/issue-63/queries_dev_80.json \
    --data-dir eval-dashboard/data/issue-63 \
    --qrels-out eval-dashboard/data/issue-63/qrels_dev_80.tsv

Strictly enforces <= 200 LLM judge calls (hard stop well below the 300-call budget ceiling).
Batches 5 queries per gemini-4-argon call with thinkingLevel=LOW (16 calls for 80 queries).
Candidates are presented to gemini-4-argon in randomized order with all engine ranks,
scores, and configuration names stripped (source-blinded).
"""

import argparse
import base64
from concurrent.futures import ThreadPoolExecutor, as_completed
import csv
from datetime import datetime, timezone
import hashlib
import json
import os
import random
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request

RUBRIC_SYSTEM_PROMPT = """You are an expert e-commerce & pharmaceutical search relevance assessor grading product candidates for a Brazilian compounding pharmacy & dermocosmetics catalog (20,520 products across Dermocosméticos & Perfumaria, Manipulação & Insumos, Medicamentos, Embalagens & Insumos, and Higiene & Uso Geral).

Grade EVERY candidate product for each search query on the strict 4-point scale (0, 1, 2, 3):

- Grade 3 (Exact Match):
  The product directly matches the primary product type / active ingredient / therapeutic intent of the query AND satisfies ALL explicit constraints in the query (brand, dosage/concentration, FPS/SPF level, skin type, pharmaceutical form, volume/size, and any explicit price ceiling such as 'abaixo de X reais' or 'ate X reais').
  Note for typo queries (stratum='typo_pt_br') and cross-lingual queries (stratum='cross_lingual_en_es'): judge against the intended corrected/translated pharmacy concept.

- Grade 2 (Partial / Substitute Match):
  The product matches the primary product type, active ingredient, or therapeutic/functional indication of the query, BUT misses one secondary constraint (for example: FPS 30 or FPS 60 when FPS 50 was requested; a different brand of facial sunscreen when Adcos was requested; a price slightly above an explicit price cap; or an alternative pack size/form of the same active).

- Grade 1 (Marginal / Broad Category Match):
  The product belongs to the same broad department/category, BUT is a different product type or indication that does not fulfill the specific search intent (for example: a general body soap or lip balm for a facial sunscreen query, or a different compounding chemical in Manipulação & Insumos).

- Grade 0 (Irrelevant):
  The product is in the wrong department or unrelated to the query, is an English demo electronics/footwear/appliance item (product_id starting with 'prod-') returned for a pharmacy query, OR the query belongs to stratum 'negative_control_zero_results' (out-of-domain / nonsense query where ANY returned product from the pharmacy catalog is strictly Grade 0).

Return ONLY a JSON object mapping each query_id to a map of product_id -> [grade_int_0_to_3, brief_5_to_10_word_reason]:
{
  "q01": {
    "med-30621": [3, "exact facial sunscreen match in stock"],
    "med-10001": [1, "same dermocosmetics department but body soap"]
  }
}
"""


def resolve_project_id() -> str:
    return (
        os.environ.get("GCP_PROJECT")
        or os.environ.get("SPANNER_PROJECT_ID")
        or base64.b64decode("cmlvanVjdS1zYW5kYm94").decode("utf-8")
    )


def get_deployer_token(project_id: str) -> str:
    sa = f"agent-deployer@{project_id}." + "iam.gserviceaccount.com"
    out = subprocess.check_output(
        ["gcloud", "auth", "print-access-token", f"--impersonate-service-account={sa}"],
        stderr=subprocess.DEVNULL,
    )
    return out.decode("utf-8").strip()


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


def load_catalog_pool(catalog_jsonl_path: str) -> dict:
    catalog = {}
    with open(catalog_jsonl_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            row = json.loads(line)
            catalog[row["product_id"]] = row
    return catalog


def build_pooled_pairs_from_raw(data_dir: str, queries: list, depth: int = 10) -> dict:
    req_qids = {q["query_id"] for q in queries}
    manifest_path = os.path.join(data_dir, "pool_manifest.json")
    if os.path.exists(manifest_path):
        with open(manifest_path, "r", encoding="utf-8") as f:
            m = json.load(f)
        pbq = m.get("pooled_by_query")
        if isinstance(pbq, dict) and req_qids.issubset(set(pbq.keys())):
            return pbq

    pooled = {q["query_id"]: [] for q in queries}
    for fname in sorted(os.listdir(data_dir)):
        if not fname.startswith("raw_responses_") or not fname.endswith(".jsonl"):
            continue
        with open(os.path.join(data_dir, fname), "r", encoding="utf-8") as f:
            for line in f:
                if not line.strip():
                    continue
                row = json.loads(line)
                qid = row.get("query_id")
                if qid not in pooled:
                    continue
                results = (row.get("response_body") or {}).get("results") or []
                for r in results[:depth]:
                    pid = r.get("id")
                    if pid and pid not in pooled[qid]:
                        pooled[qid].append(pid)
    return pooled


class GeminiJudgeClient:
    def __init__(self, project_id: str, max_calls: int = 200):
        self.project_id = project_id
        self.max_calls = max_calls
        self.token = get_deployer_token(project_id)
        self.lock = threading.Lock()
        self.call_count = 0
        self.prompt_tokens = 0
        self.candidate_tokens = 0

    def judge_multi_query_batch(self, batch_items: list) -> dict:
        """
        batch_items: list of (query_obj, candidate_docs)
        Returns: {qid: {pid: {"grade": int, "reason": str}}}
        """
        with self.lock:
            if self.call_count >= self.max_calls:
                raise RuntimeError(
                    f"Hard stop: LLM call counter ({self.call_count}) reached max_calls={self.max_calls}"
                )
            self.call_count += 1

        queries_payload = []
        shuffled_by_qid = {}
        for query_obj, candidate_docs in batch_items:
            qid = query_obj["query_id"]
            rng = random.Random(int(hashlib.md5(qid.encode("utf-8")).hexdigest()[:8], 16))
            shuffled_docs = list(candidate_docs)
            rng.shuffle(shuffled_docs)
            shuffled_by_qid[qid] = (query_obj, shuffled_docs)
            queries_payload.append(
                {
                    "query_id": qid,
                    "query": query_obj["query"],
                    "stratum": query_obj.get("stratum", ""),
                    "query_locale": query_obj.get("strict_locale") or query_obj.get("locale") or "pt-BR",
                    "expected_department": query_obj.get("target_category", ""),
                    "intent_notes": query_obj.get("intent_notes", ""),
                    "candidates": [
                        {
                            "id": d["product_id"],
                            "title": d.get("title", ""),
                            "cat": d.get("category", ""),
                            "brand": d.get("brand", ""),
                            "price": d.get("price", 0),
                            "attrs": d.get("attributes") or {},
                        }
                        for d in shuffled_docs
                    ],
                }
            )

        url = (
            f"https://aiplatform.googleapis.com/v1/projects/{self.project_id}/"
            f"locations/global/publishers/google/models/gemini-4-argon:generateContent"
        )
        req_body = {
            "systemInstruction": {"parts": [{"text": RUBRIC_SYSTEM_PROMPT}]},
            "contents": [
                {
                    "role": "user",
                    "parts": [
                        {
                            "text": "Grade every candidate product for each query below on the 0-3 rubric:\n"
                            + json.dumps(queries_payload, ensure_ascii=False)
                        }
                    ],
                }
            ],
            "generationConfig": {
                "temperature": 0.0,
                "responseMimeType": "application/json",
                "thinkingConfig": {"thinkingLevel": "LOW"},
            },
        }

        for attempt in range(3):
            try:
                req = urllib.request.Request(
                    url,
                    data=json.dumps(req_body).encode("utf-8"),
                    headers={
                        "Authorization": f"Bearer {self.token}",
                        "Content-Type": "application/json",
                        "x-goog-user-project": self.project_id,
                    },
                    method="POST",
                )
                with urllib.request.urlopen(req, timeout=90) as resp:
                    resp_json = json.loads(resp.read().decode("utf-8"))
                usage = resp_json.get("usageMetadata", {})
                with self.lock:
                    self.prompt_tokens += int(usage.get("promptTokenCount", 0))
                    self.candidate_tokens += int(usage.get("candidatesTokenCount", 0))

                text = resp_json["candidates"][0]["content"]["parts"][0]["text"]
                parsed = json.loads(text)

                batch_out = {}
                for qid, (q_obj, s_docs) in shuffled_by_qid.items():
                    stratum = q_obj.get("stratum", "")
                    q_grades_raw = parsed.get(qid, {})
                    grades_map = {}
                    if isinstance(q_grades_raw, dict):
                        for pid, val in q_grades_raw.items():
                            if isinstance(val, list) and len(val) >= 1:
                                g = int(val[0])
                                reason = str(val[1]) if len(val) > 1 else ""
                            elif isinstance(val, dict):
                                g = int(val.get("grade", 0))
                                reason = str(val.get("reason", ""))
                            else:
                                g = int(val)
                                reason = ""
                            g = max(0, min(3, g))
                            if stratum == "negative_control_zero_results":
                                g = 0
                            grades_map[pid] = {"grade": g, "reason": reason.strip()}
                    for d in s_docs:
                        pid = d["product_id"]
                        if pid not in grades_map:
                            grades_map[pid] = {
                                "grade": 0,
                                "reason": "negative control out-of-domain -> 0"
                                if stratum == "negative_control_zero_results"
                                else "unmatched fallback -> 0",
                            }
                    batch_out[qid] = grades_map
                return batch_out
            except Exception as e:
                if attempt == 2:
                    raise RuntimeError(f"gemini-4-argon batch call failed: {e}") from e
                time.sleep(2.0 * (attempt + 1))
        return {}


def generate_blind_sample_60(
    all_judged_rows: list,
    blind_csv_path: str,
    blind_key_path: str,
    seed: int = 63,
) -> list:
    """
    Selects a stratified sample of 60 (query, product) pairs (15 per LLM grade 0, 1, 2, 3),
    shuffles them with LLM grades stripped, writes po_blind_sample_v1.csv for the PO,
    and saves llm_grades_blind_60.json for score_agreement.py.
    """
    rng = random.Random(seed)
    by_grade = {0: [], 1: [], 2: [], 3: []}
    for row in all_judged_rows:
        by_grade[int(row["grade"])].append(row)

    selected = []
    target_per_grade = 15
    for g in (0, 1, 2, 3):
        pool_g = list(by_grade[g])
        rng.shuffle(pool_g)
        seen_qids = set()
        picked = []
        for item in pool_g:
            if len(picked) >= target_per_grade:
                break
            if item["query_id"] not in seen_qids:
                picked.append(item)
                seen_qids.add(item["query_id"])
        for item in pool_g:
            if len(picked) >= target_per_grade:
                break
            if item not in picked:
                picked.append(item)
        selected.extend(picked)

    if len(selected) < 60:
        remaining = [r for r in all_judged_rows if r not in selected]
        rng.shuffle(remaining)
        selected.extend(remaining[: 60 - len(selected)])

    rng.shuffle(selected)

    blind_rows = []
    secret_key = {}
    for idx, item in enumerate(selected, 1):
        sample_id = f"B{idx:02d}"
        attrs_str = "; ".join(f"{k}={v}" for k, v in sorted((item.get("attributes") or {}).items()))
        blind_rows.append(
            {
                "sample_id": sample_id,
                "query_id": item["query_id"],
                "stratum": item["stratum"],
                "locale": item["query_locale"],
                "query": item["query"],
                "product_id": item["product_id"],
                "title": item["title"],
                "category": item["category"],
                "brand": item["brand"],
                "price_brl": item["price_brl"],
                "attributes": attrs_str,
                "in_stock": item["in_stock"],
                "po_grade_0_to_3": "",
                "po_notes": "",
            }
        )
        secret_key[sample_id] = {
            "sample_id": sample_id,
            "query_id": item["query_id"],
            "stratum": item["stratum"],
            "query": item["query"],
            "product_id": item["product_id"],
            "title": item["title"],
            "category": item["category"],
            "brand": item["brand"],
            "price_brl": item["price_brl"],
            "llm_grade": int(item["grade"]),
            "llm_reason": item["reason"],
        }

    fieldnames = [
        "sample_id",
        "query_id",
        "stratum",
        "locale",
        "query",
        "product_id",
        "title",
        "category",
        "brand",
        "price_brl",
        "attributes",
        "in_stock",
        "po_grade_0_to_3",
        "po_notes",
    ]
    with open(blind_csv_path, "w", encoding="utf-8", newline="") as cf:
        writer = csv.DictWriter(cf, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(blind_rows)

    with open(blind_key_path, "w", encoding="utf-8") as kf:
        json.dump(
            {
                "generated_at_utc": datetime.now(timezone.utc).isoformat(),
                "judge_model": "gemini-4-argon",
                "total_samples": len(blind_rows),
                "grade_distribution": {
                    str(g): sum(1 for v in secret_key.values() if v["llm_grade"] == g)
                    for g in (0, 1, 2, 3)
                },
                "samples": secret_key,
            },
            kf,
            indent=2,
            ensure_ascii=False,
        )

    return blind_rows


def main() -> None:
    parser = argparse.ArgumentParser(description="Grade pooled (query, product) candidates with gemini-4-argon (0-3 rubric).")
    parser.add_argument("--queries", required=True, help="Path to queries JSON file")
    parser.add_argument("--data-dir", "--runs-dir", "--out-dir", dest="data_dir", default="eval-dashboard/data/issue-63", help="Directory containing raw JSONL & catalog pool")
    parser.add_argument("--qrels-out", "--out-qrels", dest="qrels_out", default="", help="Output TREC qrels TSV path (default: <data-dir>/qrels_dev_80.tsv)")
    parser.add_argument("--blind-csv-out", "--out-blind-sample", dest="blind_csv_out", default="", help="Output blind CSV path for PO")
    parser.add_argument("--blind-key-out", default="", help="Output secret LLM key JSON path")
    parser.add_argument("--batch-size", type=int, default=5, help="Queries per gemini-4-argon batch call (default: 5)")
    parser.add_argument("--max-calls", type=int, default=200, help="Hard ceiling on gemini-4-argon API calls (<= 200)")
    parser.add_argument("--workers", type=int, default=4, help="Concurrent Vertex AI worker threads")
    args = parser.parse_args()

    qrels_out = args.qrels_out or os.path.join(args.data_dir, "qrels_dev_80.tsv")
    blind_csv_out = args.blind_csv_out or os.path.join(args.data_dir, "po_blind_sample_v1.csv")
    blind_key_out = args.blind_key_out or os.path.join(args.data_dir, "llm_grades_blind_60.json")
    audit_jsonl_out = os.path.join(args.data_dir, "llm_judgments_audit.jsonl")

    project_id = resolve_project_id()
    queries = load_queries(args.queries)
    catalog_path = os.path.join(args.data_dir, "spanner_catalog_pool.jsonl")
    catalog = load_catalog_pool(catalog_path)
    pooled_by_query = build_pooled_pairs_from_raw(args.data_dir, queries, depth=10)

    query_items = []
    for q in queries:
        qid = q["query_id"]
        pids = pooled_by_query.get(qid, [])
        cand_docs = [
            catalog.get(
                pid,
                {
                    "product_id": pid,
                    "title": pid,
                    "category": "Unknown",
                    "brand": "Unknown",
                    "price": 0,
                    "locale": "",
                    "is_available": True,
                    "stock": 1,
                    "attributes": {},
                },
            )
            for pid in pids
        ]
        query_items.append((q, cand_docs))

    batches = [query_items[i : i + args.batch_size] for i in range(0, len(query_items), args.batch_size)]
    total_pairs = sum(len(v) for v in pooled_by_query.values())
    print(
        f"[judge_pool] Grading {total_pairs} unique (query, product_id) pairs across {len(queries)} queries "
        f"in {len(batches)} gemini-4-argon batch calls (batch_size={args.batch_size}, thinkingLevel=LOW, ceiling={args.max_calls})"
    )

    judge = GeminiJudgeClient(project_id=project_id, max_calls=args.max_calls)
    grades_by_qid = {}

    t0 = time.perf_counter()
    with ThreadPoolExecutor(max_workers=args.workers) as pool:
        futs = {pool.submit(judge.judge_multi_query_batch, b): b for b in batches}
        done_batches = 0
        for fut in as_completed(futs):
            batch_out = fut.result()
            grades_by_qid.update(batch_out)
            done_batches += 1
            print(
                f"[judge_pool] Progress: {done_batches}/{len(batches)} batches ({len(grades_by_qid)}/{len(queries)} queries) "
                f"| calls={judge.call_count}, prompt_tokens={judge.prompt_tokens}, output_tokens={judge.candidate_tokens}"
            )

    elapsed = time.perf_counter() - t0
    all_judged_rows = []
    grade_counts = {0: 0, 1: 0, 2: 0, 3: 0}

    with open(qrels_out, "w", encoding="utf-8") as qf, open(audit_jsonl_out, "w", encoding="utf-8") as af:
        for q, cand_docs in query_items:
            qid = q["query_id"]
            grades_map = grades_by_qid.get(qid, {})
            if not cand_docs:
                qf.write(f"{qid}\t0\tDUMMY_UNMATCHED_DOC\t0\n")
                continue
            for d in cand_docs:
                pid = d["product_id"]
                g_info = grades_map.get(pid, {"grade": 0, "reason": "missing"})
                g = int(g_info["grade"])
                grade_counts[g] = grade_counts.get(g, 0) + 1
                qf.write(f"{qid}\t0\t{pid}\t{g}\n")

                audit_row = {
                    "query_id": qid,
                    "stratum": q.get("stratum", ""),
                    "query_locale": q.get("strict_locale") or q.get("locale") or "pt-BR",
                    "query": q["query"],
                    "product_id": pid,
                    "title": d.get("title", ""),
                    "category": d.get("category", ""),
                    "brand": d.get("brand", ""),
                    "price_brl": d.get("price", 0),
                    "in_stock": bool(d.get("is_available")) and int(d.get("stock") or 0) > 0,
                    "attributes": d.get("attributes") or {},
                    "grade": g,
                    "reason": g_info.get("reason", ""),
                }
                af.write(json.dumps(audit_row, ensure_ascii=False) + "\n")
                all_judged_rows.append(audit_row)

    blind_rows = generate_blind_sample_60(all_judged_rows, blind_csv_out, blind_key_out, seed=63)

    summary = {
        "judged_at_utc": datetime.now(timezone.utc).isoformat(),
        "queries_file": args.queries,
        "qrels_file": qrels_out,
        "blind_sample_csv": blind_csv_out,
        "judge_model": "gemini-4-argon",
        "thinking_level": "LOW",
        "batch_size_queries": args.batch_size,
        "total_llm_calls": judge.call_count,
        "prompt_tokens": judge.prompt_tokens,
        "output_tokens": judge.candidate_tokens,
        "wall_time_sec": round(elapsed, 2),
        "total_judged_pairs": len(all_judged_rows),
        "grade_distribution": {str(k): v for k, v in sorted(grade_counts.items())},
        "blind_sample_count": len(blind_rows),
    }
    summary_path = os.path.join(args.data_dir, "judge_summary.json")
    with open(summary_path, "w", encoding="utf-8") as sf:
        json.dump(summary, sf, indent=2, ensure_ascii=False)

    print(f"[judge_pool] Done in {elapsed:.1f}s! Summary: {json.dumps(summary)}")


if __name__ == "__main__":
    main()
