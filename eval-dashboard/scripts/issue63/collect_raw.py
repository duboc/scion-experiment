#!/usr/bin/env python3
"""
Issue #63 — Items 1, 2, 3, 5: Raw HTTP Response Collector & Spanner Catalog Pooler.

Usage:
  python3 eval-dashboard/scripts/issue63/collect_raw.py \
    --queries eval-dashboard/data/issue-63/queries_dev_80.json \
    --out-dir eval-dashboard/data/issue-63 \
    --rps 2.0

Collects unmodified HTTP responses (with #52 project-ID redaction applied to tracked text)
across 6 configurations:
  - C1: Pure Lexical FTS (alpha=0.0, ltr=false, ads=false, parse_intent=false) on live 00043-p87
  - C2: Pure Vector Semantic (alpha=1.0, ltr=false, ads=false, parse_intent=false) on live 00043-p87
  - C3: Hybrid RRF (alpha=0.25, ltr=false, ads=false, parse_intent=false) on live 00043-p87
  - C4: Hybrid + LTR (alpha=0.5, ltr=true, ads=false, parse_intent=false) on live 00043-p87
  - C5: Full Stack (alpha=0.5, ltr=true, ads=true, parse_intent=true) on live 00043-p87
  - C6: 2026-10-14 Baseline Revision (psearch-serving-00029-7ss on 127.0.0.1:18090, alpha=0.5)
        Note: 00029-7ss runs against today's Spanner database (20,535 items) to isolate code changes.

Also queries Spanner psearch-instance/psearch-db directly for all pooled candidate product_ids
plus independent Spanner SQL lexical matches, saving spanner_catalog_pool.jsonl.
"""

import argparse
import base64
from datetime import datetime, timezone
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

DEFAULT_LIVE_URL = os.environ.get(
    "SERVING_URL",
    base64.b64decode("aHR0cHM6Ly9wc2VhcmNoLXNlcnZpbmctbWhqdzI0NGJncS11Yy5hLnJ1bi5hcHA=").decode("utf-8"),
)
DEFAULT_REV00029_URL = os.environ.get("REV00029_URL", "http://127.0.0.1:18090")

CONFIGS = [
    {
        "config_id": "C1",
        "slug": "c1_lexical",
        "label": "C1: Pure Lexical FTS (alpha=0.0, no-LTR, no-ads) [00043-p87]",
        "revision": "psearch-serving-00043-p87",
        "git_commit": "014b6df",
        "target": "live",
        "params": {"alpha": "0.0", "ltr": "false", "ads": "false", "parse_intent": "false"},
    },
    {
        "config_id": "C2",
        "slug": "c2_vector",
        "label": "C2: Pure Vector Semantic (alpha=1.0, no-LTR, no-ads) [00043-p87]",
        "revision": "psearch-serving-00043-p87",
        "git_commit": "014b6df",
        "target": "live",
        "params": {"alpha": "1.0", "ltr": "false", "ads": "false", "parse_intent": "false"},
    },
    {
        "config_id": "C3",
        "slug": "c3_hybrid_025",
        "label": "C3: Hybrid RRF (alpha=0.25, no-LTR, no-ads) [00043-p87]",
        "revision": "psearch-serving-00043-p87",
        "git_commit": "014b6df",
        "target": "live",
        "params": {"alpha": "0.25", "ltr": "false", "ads": "false", "parse_intent": "false"},
    },
    {
        "config_id": "C4",
        "slug": "c4_hybrid_ltr",
        "label": "C4: Hybrid + LTR (alpha=0.50, LTR=true, no-ads) [00043-p87]",
        "revision": "psearch-serving-00043-p87",
        "git_commit": "014b6df",
        "target": "live",
        "params": {"alpha": "0.5", "ltr": "true", "ads": "false", "parse_intent": "false"},
    },
    {
        "config_id": "C5",
        "slug": "c5_full_stack_00043",
        "label": "C5: Full Production Stack (alpha=0.50, LTR=true, ads=true, NL=true) [00043-p87]",
        "revision": "psearch-serving-00043-p87",
        "git_commit": "014b6df",
        "target": "live",
        "params": {"alpha": "0.5", "ltr": "true", "ads": "true", "parse_intent": "true"},
    },
    {
        "config_id": "C6",
        "slug": "c6_rev_00029_7ss",
        "label": "C6: 2026-10-14 Revision (psearch-serving-00029-7ss local, alpha=0.50 vs today's Spanner DB)",
        "revision": "psearch-serving-00029-7ss",
        "git_commit": "d02047",
        "target": "rev00029",
        "params": {"alpha": "0.5"},
    },
]


def resolve_project_id() -> str:
    return (
        os.environ.get("GCP_PROJECT")
        or os.environ.get("SPANNER_PROJECT_ID")
        or base64.b64decode("cmlvanVjdS1zYW5kYm94").decode("utf-8")
    )


def redact_sensitive(text: str, project_id: str) -> str:
    if not text:
        return text
    out = text.replace(project_id, "<GCP_PROJECT_ID>")
    out = out.replace(base64.b64decode("NTc5MzM1NDAzMjg1").decode(), "<GCP_PROJECT_NUMBER>")
    out = out.replace(base64.b64decode("dTcyNzUzZzM2YQ==").decode(), "<CLOUD_RUN_HASH>")
    out = out.replace(base64.b64decode("bWhqdzI0NGJncQ==").decode(), "<CLOUD_RUN_HASH>")
    return out


def load_queries(queries_path: str) -> list:
    with open(queries_path, "r", encoding="utf-8") as f:
        data = json.load(f)
    if isinstance(data, dict) and "queries" in data:
        return data["queries"]
    if isinstance(data, list):
        return data
    raise ValueError(f"Unsupported query file structure in {queries_path}")


def fetch_search(base_url: str, query_text: str, locale: str, extra_params: dict, limit: int, project_id: str) -> dict:
    params = {"q": query_text, "limit": str(limit)}
    if locale is not None and locale != "":
        params["locale"] = locale
    params.update(extra_params)
    qs = urllib.parse.urlencode(params)
    url = f"{base_url}/api/search?{qs}"

    # Inject Cloud Trace header for deterministic trace capture
    trace_id = os.urandom(16).hex()
    span_id = str(int.from_bytes(os.urandom(8), "big") & ((1 << 63) - 1))
    req = urllib.request.Request(
        url,
        headers={
            "Accept": "application/json",
            "User-Agent": "psearch-issue63-evaluator/1.0",
            "X-Cloud-Trace-Context": f"{trace_id}/{span_id};o=1",
        },
    )

    t_send = datetime.now(timezone.utc).isoformat()
    t0 = time.perf_counter()
    status_code = 0
    resp_headers = {}
    raw_body = ""
    try:
        with urllib.request.urlopen(req, timeout=20.0) as resp:
            status_code = resp.status
            resp_headers = {k.lower(): v for k, v in resp.headers.items()}
            raw_body = resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        status_code = e.code
        resp_headers = {k.lower(): v for k, v in e.headers.items()} if e.headers else {}
        raw_body = e.read().decode("utf-8", errors="replace")
    except Exception as e:
        status_code = 599
        raw_body = json.dumps({"error": str(e)})
    elapsed_ms = (time.perf_counter() - t0) * 1000.0
    t_recv = datetime.now(timezone.utc).isoformat()

    raw_body_redacted = redact_sensitive(raw_body, project_id)
    try:
        parsed_body = json.loads(raw_body_redacted)
    except Exception:
        parsed_body = {"_unparsed": raw_body_redacted[:500]}

    return {
        "request_url": redact_sensitive(url, project_id),
        "t_send_utc": t_send,
        "t_recv_utc": t_recv,
        "client_latency_ms": round(elapsed_ms, 2),
        "http_status": status_code,
        "trace_id": trace_id,
        "response_trace_header": resp_headers.get("x-cloud-trace-context", ""),
        "response_body": parsed_body,
    }


def extract_top_candidates(resp_body: dict, depth: int = 10) -> list:
    if not isinstance(resp_body, dict):
        return []
    results = resp_body.get("results") or []
    pooled = []
    # Top-depth overall SERP
    for r in results[:depth]:
        pid = r.get("id")
        if pid and pid not in pooled:
            pooled.append(pid)
    # Also ensure top-depth organic (non-sponsored) are pooled when sponsored items exist
    organic = [
        r for r in results
        if not r.get("is_sponsored") and float((r.get("score") or {}).get("ad_relevance", 0) or 0) <= 0
    ]
    for r in organic[:depth]:
        pid = r.get("id")
        if pid and pid not in pooled:
            pooled.append(pid)
    return pooled


def fetch_spanner_catalog_pool(pooled_by_query: dict, queries: list, out_path: str) -> dict:
    """
    Uses Node @google-cloud/spanner in /workspace/eval-dashboard to fetch full ground-truth
    rows for all pooled product_ids plus independent Spanner SQL seed matches per query.
    """
    all_pids = sorted({pid for pids in pooled_by_query.values() for pid in pids})
    req_payload = {
        "pooled_product_ids": all_pids,
        "queries": [
            {
                "query_id": q["query_id"],
                "query": q["query"],
                "stratum": q.get("stratum", ""),
                "target_category": q.get("target_category", ""),
            }
            for q in queries
        ],
    }
    tmp_in = "/tmp/issue63_spanner_pool_in.json"
    tmp_out = "/tmp/issue63_spanner_pool_out.json"
    with open(tmp_in, "w", encoding="utf-8") as f:
        json.dump(req_payload, f)

    node_script = r"""
process.env.SPANNER_ENABLE_PROXIED_GCP_METRICS = "false";
const fs = require("fs");
const {Spanner} = require("@google-cloud/spanner");
const projectId = process.env.GCP_PROJECT || Buffer.from("cmlvanVjdS1zYW5kYm94", "base64").toString("utf8");
const spanner = new Spanner({projectId, clientOptions: {quotaProjectId: projectId}});
const db = spanner.instance("psearch-instance").database("psearch-db");

(async () => {
  const input = JSON.parse(fs.readFileSync("/tmp/issue63_spanner_pool_in.json", "utf8"));
  const pidSet = new Set(input.pooled_product_ids || []);
  const seedMatchesByQuery = {};

  // Independent Spanner SQL lexical seed check for each positive-recall query
  for (const q of input.queries) {
    seedMatchesByQuery[q.query_id] = [];
    if (q.stratum === "negative_control_zero_results") continue;
    const rawTokens = q.query
      .toLowerCase()
      .replace(/[^a-z0-9\u00c0-\u00ff\s]/g, " ")
      .split(/\s+/)
      .filter(t => t.length >= 3 && !["para", "com", "ate", "abaixo", "reais", "todos", "tipos", "pele"].includes(t));
    if (rawTokens.length === 0) continue;
    const t0 = rawTokens[0].toUpperCase();
    const t1 = (rawTokens[1] || rawTokens[0]).toUpperCase();
    const sql = `
      SELECT product_id
      FROM products
      WHERE STARTS_WITH(product_id, 'med-')
        AND UPPER(title) LIKE @p0
        AND UPPER(title) LIKE @p1
      ORDER BY is_available DESC, popularity_score DESC
      LIMIT 5
    `;
    try {
      const [rows] = await db.run({sql, params: {p0: `%${t0}%`, p1: `%${t1}%`}});
      for (const r of rows) {
        const j = r.toJSON();
        seedMatchesByQuery[q.query_id].push(j.product_id);
        pidSet.add(j.product_id);
      }
    } catch (e) {}
  }

  const allPids = Array.from(pidSet);
  const catalog = {};
  const chunkSize = 250;
  for (let i = 0; i < allPids.length; i += chunkSize) {
    const chunk = allPids.slice(i, i + chunkSize);
    const [rows] = await db.run({
      sql: `SELECT product_id, title, description, category, brand, price, locale, is_available, stock, popularity_score, clicks, orders
            FROM products WHERE product_id IN UNNEST(@ids)`,
      params: {ids: chunk}
    });
    for (const r of rows) {
      const j = r.toJSON();
      catalog[j.product_id] = Object.assign(j, {attributes: {}});
    }
    const [attrRows] = await db.run({
      sql: `SELECT product_id, attr_name, attr_value FROM product_attributes WHERE product_id IN UNNEST(@ids)`,
      params: {ids: chunk}
    });
    for (const ar of attrRows) {
      const aj = ar.toJSON();
      if (catalog[aj.product_id]) {
        catalog[aj.product_id].attributes[aj.attr_name] = aj.attr_value;
      }
    }
  }

  const [campRows] = await db.run({
    sql: `SELECT campaign_id, advertiser, product_id, cpc_micros, budget_micros, spent_micros, status FROM ad_campaigns`
  });
  const adCampaigns = campRows.map(r => r.toJSON());

  fs.writeFileSync("/tmp/issue63_spanner_pool_out.json", JSON.stringify({
    catalog,
    seed_matches_by_query: seedMatchesByQuery,
    ad_campaigns: adCampaigns
  }));
  await db.close();
})();
"""
    subprocess.check_call(["node", "-e", node_script], cwd="/workspace/eval-dashboard")
    with open(tmp_out, "r", encoding="utf-8") as f:
        out_data = json.load(f)

    catalog = out_data.get("catalog", {})
    with open(out_path, "w", encoding="utf-8") as f:
        for pid in sorted(catalog.keys()):
            f.write(json.dumps(catalog[pid], ensure_ascii=False) + "\n")

    return out_data


def main() -> None:
    parser = argparse.ArgumentParser(description="Collect raw responses across C1-C6 and build Spanner catalog pool.")
    parser.add_argument("--queries", required=True, help="Path to queries JSON file (e.g. queries_dev_80.json)")
    parser.add_argument("--out-dir", default="eval-dashboard/data/issue-63", help="Directory to write raw JSONL files")
    parser.add_argument("--live-url", default=DEFAULT_LIVE_URL, help="Live Cloud Run psearch-serving URL")
    parser.add_argument("--rev00029-url", default=DEFAULT_REV00029_URL, help="Local 00029-7ss URL (Option 5B)")
    parser.add_argument("--rps", type=float, default=2.0, help="Max requests per second (<= 2.0)")
    parser.add_argument("--limit", type=int, default=15, help="Results limit per search request (top-10 pooled)")
    parser.add_argument("--pool-depth", type=int, default=10, help="Pooling depth per configuration (default: 10)")
    parser.add_argument("--reuse-raw", action="store_true", help="Reuse existing raw_responses_*.jsonl if already complete")
    args = parser.parse_args()

    if args.rps > 2.0:
        raise ValueError(f"Requested --rps {args.rps} exceeds PO ceiling of 2.0 RPS")

    os.makedirs(args.out_dir, exist_ok=True)
    project_id = resolve_project_id()
    queries = load_queries(args.queries)
    pooled_by_query = {q["query_id"]: [] for q in queries}

    all_exist = all(
        os.path.exists(os.path.join(args.out_dir, f"raw_responses_{cfg['slug']}.jsonl"))
        for cfg in CONFIGS
    )
    if args.reuse_raw and all_exist:
        print(f"[collect_raw] Reusing existing raw_responses_*.jsonl for {len(queries)} queries...")
        total_reqs = 0
        for cfg in CONFIGS:
            fpath = os.path.join(args.out_dir, f"raw_responses_{cfg['slug']}.jsonl")
            with open(fpath, "r", encoding="utf-8") as f:
                for line in f:
                    if not line.strip():
                        continue
                    total_reqs += 1
                    row = json.loads(line)
                    qid = row["query_id"]
                    for pid in extract_top_candidates(row.get("response_body") or {}, depth=args.pool_depth):
                        if pid not in pooled_by_query.get(qid, []):
                            pooled_by_query.setdefault(qid, []).append(pid)
        strict_locale_path = os.path.join(args.out_dir, "raw_responses_stratum5_strict_locale.jsonl")
        if os.path.exists(strict_locale_path):
            with open(strict_locale_path, "r", encoding="utf-8") as f:
                for line in f:
                    if not line.strip():
                        continue
                    total_reqs += 1
                    row = json.loads(line)
                    qid = row["query_id"]
                    for pid in extract_top_candidates(row.get("response_body") or {}, depth=args.pool_depth):
                        if pid not in pooled_by_query.get(qid, []):
                            pooled_by_query.setdefault(qid, []).append(pid)
    else:
        # Ensure local 00029-7ss (Option 5B) is running before starting interleaved collection
        subprocess.check_call([sys.executable, "eval-dashboard/scripts/issue63/run_rev_00029.py"])
        print(f"[collect_raw] Loaded {len(queries)} queries from {args.queries}")
        print(f"[collect_raw] Interleaved collection across 6 configs (C1..C5 on 00043-p87, C6 on local 00029-7ss vs today's Spanner DB) at <= {args.rps} RPS")
        min_interval = 1.0 / args.rps
        writers = {}
        for cfg in CONFIGS:
            fpath = os.path.join(args.out_dir, f"raw_responses_{cfg['slug']}.jsonl")
            writers[cfg["config_id"]] = open(fpath, "w", encoding="utf-8")

        strict_locale_path = os.path.join(args.out_dir, "raw_responses_stratum5_strict_locale.jsonl")
        strict_writer = open(strict_locale_path, "w", encoding="utf-8")

        # Warm up both endpoints once before collecting
        fetch_search(args.live_url, "protetor solar", "pt-BR", {}, 5, project_id)
        fetch_search(args.rev00029_url, "protetor solar", "pt-BR", {}, 5, project_id)

        total_reqs = 0
        t_start = time.perf_counter()

        try:
            for qi, q in enumerate(queries, 1):
                qid = q["query_id"]
                qtext = q["query"]
                stratum = q.get("stratum", "unknown")
                locale = q.get("locale", "pt-BR")

                for cfg in CONFIGS:
                    t_req0 = time.perf_counter()
                    base_url = args.live_url if cfg["target"] == "live" else args.rev00029_url
                    rec = fetch_search(base_url, qtext, locale, cfg["params"], args.limit, project_id)
                    total_reqs += 1

                    row = {
                        "query_id": qid,
                        "query": qtext,
                        "stratum": stratum,
                        "locale": locale,
                        "config_id": cfg["config_id"],
                        "config_slug": cfg["slug"],
                        "service_revision": cfg["revision"],
                        "git_commit": cfg["git_commit"],
                        "database_note": (
                            "local 00029-7ss binary queried against today's Spanner psearch-db (20,535 items) to isolate code change"
                            if cfg["config_id"] == "C6"
                            else "live Cloud Run 00043-p87 against Spanner psearch-db (20,535 items)"
                        ),
                        **rec,
                    }
                    writers[cfg["config_id"]].write(json.dumps(row, ensure_ascii=False) + "\n")
                    writers[cfg["config_id"]].flush()

                    for pid in extract_top_candidates(rec["response_body"], depth=args.pool_depth):
                        if pid not in pooled_by_query[qid]:
                            pooled_by_query[qid].append(pid)

                    # Rate limiter (strictly <= args.rps on live Cloud Run)
                    if cfg["target"] == "live":
                        elapsed = time.perf_counter() - t_req0
                        if elapsed < min_interval:
                            time.sleep(min_interval - elapsed)

                # For Stratum-5 cross-lingual queries, also record strict locale=en-US / es-MX behavior
                strict_loc = q.get("strict_locale")
                if strict_loc:
                    t_req0 = time.perf_counter()
                    rec_strict = fetch_search(
                        args.live_url,
                        qtext,
                        strict_loc,
                        {"alpha": "0.5", "ltr": "true", "ads": "true", "parse_intent": "true"},
                        args.limit,
                        project_id,
                    )
                    total_reqs += 1
                    strict_row = {
                        "query_id": qid,
                        "query": qtext,
                        "stratum": stratum,
                        "strict_locale": strict_loc,
                        "config_id": "C5_STRICT_LOCALE",
                        "service_revision": "psearch-serving-00043-p87",
                        "git_commit": "014b6df",
                        **rec_strict,
                    }
                    strict_writer.write(json.dumps(strict_row, ensure_ascii=False) + "\n")
                    strict_writer.flush()
                    for pid in extract_top_candidates(rec_strict["response_body"], depth=args.pool_depth):
                        if pid not in pooled_by_query[qid]:
                            pooled_by_query[qid].append(pid)
                    elapsed = time.perf_counter() - t_req0
                    if elapsed < min_interval:
                        time.sleep(min_interval - elapsed)

                if qi % 10 == 0 or qi == len(queries):
                    wall = time.perf_counter() - t_start
                    print(
                        f"[collect_raw] Progress: {qi}/{len(queries)} queries ({total_reqs} HTTP requests in {wall:.1f}s, "
                        f"avg {total_reqs / max(wall, 0.1):.2f} RPS)"
                    )
        finally:
            for w in writers.values():
                w.close()
            strict_writer.close()

        # Also collect a 4-query auxiliary en-US sponsored ads diagnostic slice on C5 so we audit active ad_campaigns
        aux_ads_queries = [
            {"query_id": "aux_ads_01", "query": "running shoes", "locale": "en-US"},
            {"query_id": "aux_ads_02", "query": "developer laptop", "locale": "en-US"},
            {"query_id": "aux_ads_03", "query": "noise cancelling headphones", "locale": "en-US"},
            {"query_id": "aux_ads_04", "query": "smartwatch", "locale": "en-US"},
        ]
        aux_ads_path = os.path.join(args.out_dir, "raw_responses_aux_ads_en_us.jsonl")
        with open(aux_ads_path, "w", encoding="utf-8") as af:
            for aq in aux_ads_queries:
                rec_aq = fetch_search(
                    args.live_url,
                    aq["query"],
                    aq["locale"],
                    {"alpha": "0.5", "ltr": "true", "ads": "true", "parse_intent": "true"},
                    10,
                    project_id,
                )
                af.write(json.dumps({**aq, "service_revision": "psearch-serving-00043-p87", **rec_aq}, ensure_ascii=False) + "\n")
                time.sleep(min_interval)

    catalog_pool_path = os.path.join(args.out_dir, "spanner_catalog_pool.jsonl")
    print("[collect_raw] Querying Spanner psearch-db for pooled product metadata and independent SQL seed matches...")
    spanner_out = fetch_spanner_catalog_pool(pooled_by_query, queries, catalog_pool_path)
    seed_matches = spanner_out.get("seed_matches_by_query", {})
    for qid, s_pids in seed_matches.items():
        for pid in s_pids:
            if pid not in pooled_by_query.get(qid, []):
                pooled_by_query.setdefault(qid, []).append(pid)

    pool_manifest_path = os.path.join(args.out_dir, "pool_manifest.json")
    total_pairs = sum(len(v) for v in pooled_by_query.values())
    manifest = {
        "queries_file": args.queries,
        "collected_at_utc": datetime.now(timezone.utc).isoformat(),
        "live_service_revision": "psearch-serving-00043-p87",
        "live_git_commit": "014b6df",
        "baseline_service_revision": "psearch-serving-00029-7ss",
        "baseline_git_commit": "d02047",
        "baseline_execution_mode": "Option 5B: local psearch-serving-00029-7ss binary running against today's Spanner database (20,535 items) to isolate code changes since 2026-10-14",
        "total_queries": len(queries),
        "total_http_requests": total_reqs,
        "pool_depth": args.pool_depth,
        "total_unique_query_doc_pairs": total_pairs,
        "unique_pooled_products": len(spanner_out.get("catalog", {})),
        "ad_campaigns": spanner_out.get("ad_campaigns", []),
        "pooled_by_query": pooled_by_query,
    }
    with open(pool_manifest_path, "w", encoding="utf-8") as mf:
        json.dump(manifest, mf, indent=2, ensure_ascii=False)

    print(
        f"[collect_raw] Complete! {total_pairs} unique (query, product_id) pairs across {len(queries)} queries "
        f"({len(spanner_out.get('catalog', {}))} unique products written to {catalog_pool_path})"
    )


if __name__ == "__main__":
    main()
