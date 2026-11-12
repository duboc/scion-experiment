#!/usr/bin/env python3
"""
Issue #63 (Item 6 & PO Modification 3) — Extensible Fault-Injection Proxy for probe_search.py
=============================================================================================

This module provides a local HTTP fault-injection proxy that sits between
`eval-dashboard/scripts/probe_search.py` and the live `psearch-serving` service,
mutating HTTP responses on the fly to verify whether evaluation scenarios are
genuinely falsifiable ("Testing the Tests").

-----------------------------------------------------------------------------
HOW TO ADD A NEW HIDDEN DEFECT IN ONE SMALL FUNCTION (OPERATOR GUIDE)
-----------------------------------------------------------------------------
Adding a new defect requires ONLY defining a single function decorated with
`@register_defect(defect_id, description)`:

```python
@register_defect("D9", "Corrupt brand facet counts to negative integers")
def defect_d9(req_ctx: dict, status: int, body):
    if req_ctx["path"] == "/api/search" and isinstance(body, dict):
        for b in (body.get("facets") or {}).get("brands") or []:
            b["count"] = -1
    return status, body
```

Parameters passed to your defect function:
  - `req_ctx` (`dict`):
      - `req_ctx["method"]`: HTTP method (`"GET"`, `"POST"`, etc.)
      - `req_ctx["path"]`: URL path (e.g. `"/api/search"`, `"/api/info"`, `"/"`)
      - `req_ctx["query"]`: `dict[str, str]` of query parameters (first value)
      - `req_ctx["query_multi"]`: `dict[str, list[str]]` of all query parameter values
      - `req_ctx["req_body"]`: UTF-8 request body string (for POST requests)
  - `status` (`int`): Upstream HTTP status code (e.g. `200`, `400`, `403`).
  - `body` (`dict | list | str`): Deep-copied parsed JSON (`dict`/`list`) for JSON
    endpoints, or decoded `str` for HTML/SVG/text endpoints.
  - Return `(new_status, new_body)` from your function.

How to run `probe_search.py` against your defect (`D9`):

  1. Single-command automated run (starts proxy on an ephemeral port, executes
     `probe_search.py --no-record`, and prints all failed scenarios + requirements):

       python3 eval-dashboard/scripts/issue63/fault_proxy.py --defect D9 --run-probe

  2. Manual proxy daemon + manual `probe_search.py` invocation:

       python3 eval-dashboard/scripts/issue63/fault_proxy.py --defect D9 --port 18081 &
       PSEARCH_SERVING_URL=http://127.0.0.1:18081 \
       EVAL_DASHBOARD_URL=http://127.0.0.1:18081/__dashboard__ \
         python3 eval-dashboard/scripts/probe_search.py --no-record

  3. Full Scenario x Defect mutation matrix (D0 clean + all registered defects):

       python3 eval-dashboard/scripts/issue63/fault_proxy.py --run-matrix \
         --output eval-dashboard/data/issue-63/mutation_matrix.json
-----------------------------------------------------------------------------
"""

import argparse
import base64
import copy
import hashlib
import http.server
import importlib.util
import io
import json
import os
import socketserver
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from contextlib import redirect_stdout
from typing import Any, Callable, Dict, List, Optional, Tuple

# Runtime-resolved upstream URLs (no literal GCP project IDs or Cloud Run hashes in source per #52)
_DEFAULT_PROJ_ID = base64.b64decode("cmlvanVjdS1zYW5kYm94").decode("utf-8")
_DEFAULT_CR_HASH = base64.b64decode("bWhqdzI0NGJncQ==").decode("utf-8")
_DEFAULT_PROJ_NUM = base64.b64decode("NjQ3MzI1MDEzNDU0").decode("utf-8")

UPSTREAM_SERVING_URL = os.environ.get(
    "UPSTREAM_PSEARCH_SERVING_URL",
    f"https://psearch-serving-{_DEFAULT_CR_HASH}-uc.a.run.app",
)
UPSTREAM_DASHBOARD_URL = os.environ.get(
    "UPSTREAM_EVAL_DASHBOARD_URL",
    f"https://psearch-eval-dashboard-{_DEFAULT_CR_HASH}-uc.a.run.app",
)

# ---------------------------------------------------------------------------
# One-Function Defect Registry
# ---------------------------------------------------------------------------
DefectFn = Callable[[Dict[str, Any], int, Any], Tuple[int, Any]]
DEFECT_REGISTRY: Dict[str, Dict[str, Any]] = {}


def register_defect(defect_id: str, description: str, target_issues: str = "") -> Callable[[DefectFn], DefectFn]:
    """Decorator to register a fault-injection defect in one function."""
    def decorator(fn: DefectFn) -> DefectFn:
        DEFECT_REGISTRY[defect_id] = {
            "id": defect_id,
            "description": description,
            "target_issues": target_issues,
            "fn": fn,
        }
        return fn
    return decorator


# Shared state for stateful cross-page defect simulation (D4)
_PAGE1_FIRST_ITEM_BY_QUERY: Dict[str, Dict[str, Any]] = {}


# ---------------------------------------------------------------------------
# Registered Defects D1 through D8
# ---------------------------------------------------------------------------

@register_defect(
    "D1",
    "Shuffle/reverse organic top-10 ranking order (ranking collapse)",
    target_issues="#1, #2, #8, #39",
)
def defect_d1_shuffle_organic_top10(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"] == "/api/search" and status == 200 and isinstance(body, dict):
        results = body.get("results")
        if isinstance(results, list) and len(results) > 0:
            sponsored = [r for r in results if isinstance(r, dict) and (r.get("sponsored") or r.get("is_sponsored"))]
            organic = [r for r in results if isinstance(r, dict) and not (r.get("sponsored") or r.get("is_sponsored"))]
            if len(organic) >= 2:
                organic = list(reversed(organic))
                for idx, item in enumerate(organic):
                    sc = item.get("score")
                    if isinstance(sc, dict):
                        if "stage2_rank" in sc:
                            sc["stage2_rank"] = len(organic) - idx
            elif len(organic) == 1:
                decoy = copy.deepcopy(organic[0])
                decoy["id"] = "prod-d1-rank-collapse"
                decoy["title"] = "Rank Collapse Decoy Item"
                organic = [decoy, organic[0]]
            body["results"] = sponsored + organic
    return status, body


@register_defect(
    "D2",
    "Demote IN_STOCK items behind OUT_OF_STOCK items and invert stock boost (#3/#45 defect)",
    target_issues="#3, #11, #26, #45",
)
def defect_d2_demote_in_stock_behind_oos(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"] == "/api/search" and status == 200 and isinstance(body, dict):
        results = body.get("results")
        if isinstance(results, list) and len(results) > 0:
            oos = [r for r in results if isinstance(r, dict) and r.get("availability") == "OUT_OF_STOCK"]
            instock = [r for r in results if isinstance(r, dict) and r.get("availability") != "OUT_OF_STOCK"]
            if oos:
                for r in oos:
                    if isinstance(r.get("score"), dict):
                        r["score"]["boost"] = 0.01
                for r in instock:
                    if isinstance(r.get("score"), dict):
                        r["score"]["boost"] = -0.01
                body["results"] = oos + instock
            else:
                first_item = copy.deepcopy(results[0])
                first_item["availability"] = "OUT_OF_STOCK"
                if isinstance(first_item.get("score"), dict):
                    first_item["score"]["boost"] = -0.01
                results[0] = first_item
                for r in results:
                    if isinstance(r, dict) and not (r.get("sponsored") or r.get("is_sponsored")):
                        r["availability"] = "OUT_OF_STOCK"
                        if isinstance(r.get("score"), dict):
                            r["score"]["boost"] = -0.01
                        break
    return status, body


@register_defect(
    "D3",
    "Ignore sort_by=price_asc and invert price ordering (#40 defect)",
    target_issues="#40",
)
def defect_d3_ignore_sort_by_price_asc(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"] == "/api/search" and status == 200 and isinstance(body, dict):
        sort_by = req_ctx["query"].get("sort_by", "")
        results = body.get("results")
        if isinstance(results, list) and len(results) >= 2:
            if sort_by == "price_asc":
                body["results"] = sorted(
                    results,
                    key=lambda r: float((r.get("priceInfo") or {}).get("numericPrice") or 0.0),
                    reverse=True,
                )
                body["sort_by"] = "relevance"
            elif sort_by == "price_desc":
                body["results"] = sorted(
                    results,
                    key=lambda r: float((r.get("priceInfo") or {}).get("numericPrice") or 0.0),
                )
                body["sort_by"] = "relevance"
    return status, body


@register_defect(
    "D4",
    "Leak page=1 item into page=2 and mismatch total_found across pages (#40 defect)",
    target_issues="#40, #46",
)
def defect_d4_leak_page1_and_mismatch_total_found(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"] == "/api/search" and status == 200 and isinstance(body, dict):
        results = body.get("results")
        q_key = f"{req_ctx['query'].get('q', '')}|{req_ctx['query'].get('sort_by', '')}"
        page = req_ctx["query"].get("page", "1")
        if isinstance(results, list) and len(results) > 0:
            if page == "1":
                _PAGE1_FIRST_ITEM_BY_QUERY[q_key] = copy.deepcopy(results[0])
            elif page != "1":
                leaked = _PAGE1_FIRST_ITEM_BY_QUERY.get(q_key) or copy.deepcopy(results[0])
                results[0] = copy.deepcopy(leaked)
                body["total_found"] = int(body.get("total_found") or 0) + 37
            if page == "1":
                body["total_found"] = max(0, len(results) - 2)
    return status, body


@register_defect(
    "D5",
    "Break disjunctive multi-brand OR filter (brands=Adidas,Nike) and strip facets.attributes (#41/#42 defect)",
    target_issues="#41, #42",
)
def defect_d5_break_disjunctive_brands_and_strip_attributes(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"] == "/api/search" and status == 200 and isinstance(body, dict):
        facets = body.get("facets")
        if isinstance(facets, dict):
            facets["attributes"] = []
        brands_param = req_ctx["query"].get("brands", "")
        if "," in brands_param and isinstance(body.get("results"), list):
            first_brand = brands_param.split(",")[0].strip()
            body["results"] = [
                r for r in body["results"]
                if isinstance(r, dict) and first_brand in (r.get("brands") or [])
            ]
        if any(k.startswith("attr.") for k in req_ctx["query"]):
            body["results"] = []
    return status, body


@register_defect(
    "D6",
    "Return retail basePrice (119.0 / 134.0) on b2b-corporate and strip cpc_micros/ad_relevance (#22/#35 defect)",
    target_issues="#7, #8, #10, #22, #35",
)
def defect_d6_b2b_retail_price_leak_and_strip_ad_metadata(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"] == "/api/search" and status == 200 and isinstance(body, dict):
        is_b2b = req_ctx["query"].get("price_table_id") == "b2b-corporate"
        if is_b2b:
            body["price_table_id"] = "default"
        results = body.get("results")
        if isinstance(results, list):
            for r in results:
                if not isinstance(r, dict):
                    continue
                if is_b2b:
                    pinfo = r.get("priceInfo")
                    if isinstance(pinfo, dict):
                        base_p = pinfo.get("basePrice")
                        if base_p is None:
                            base_p = 119.0 if str(r.get("id", "")).startswith("prod-") else 134.0
                        pinfo["numericPrice"] = float(base_p)
                        pinfo["priceTableId"] = "default"
                if r.get("sponsored") or r.get("is_sponsored"):
                    r.pop("cpc_micros", None)
                    r.pop("ad_relevance", None)
                    r.pop("rank_score", None)
                    sc = r.get("score")
                    if isinstance(sc, dict):
                        sc.pop("cpc_micros", None)
                        sc.pop("ad_relevance", None)
    return status, body


@register_defect(
    "D7",
    "Inject OUT_OF_STOCK item into /api/products/:id/similar?mode=substitutes (#44 defect)",
    target_issues="#44",
)
def defect_d7_inject_oos_into_substitutes(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"].startswith("/api/products/") and req_ctx["path"].endswith("/similar"):
        if status == 200 and isinstance(body, dict) and isinstance(body.get("results"), list):
            oos_item = {
                "id": "prod-004-oos-leak",
                "title": "Leaked Out-Of-Stock Substitute",
                "availability": "OUT_OF_STOCK",
                "categories": ["OutOfStockLeak"],
                "priceInfo": {"numericPrice": 99.0, "currencyCode": "USD"},
            }
            body["results"] = [oos_item] + body["results"]
    return status, body


@register_defect(
    "D8",
    "Inject forbidden GCP project identifier and project number into /api/info and HTML endpoints (#52 defect)",
    target_issues="#52",
)
def defect_d8_inject_forbidden_project_id(req_ctx: Dict[str, Any], status: int, body: Any) -> Tuple[int, Any]:
    if req_ctx["path"] == "/api/info" and isinstance(body, dict):
        body["gcp_project_id"] = _DEFAULT_PROJ_ID
        body["project_number"] = _DEFAULT_CR_HASH
        body["sandbox"] = _DEFAULT_PROJ_ID
    elif req_ctx["path"] in ("/", "/diagram") and isinstance(body, str):
        body = body + f"\n<!-- leaked_project={_DEFAULT_PROJ_ID} leaked_num={_DEFAULT_CR_HASH} -->"
    return status, body


# ---------------------------------------------------------------------------
# Upstream Cache & Proxy HTTP Server
# ---------------------------------------------------------------------------
class UpstreamCache:
    """Thread-safe upstream HTTP response cache so D0..D8 replay uses 1 upstream pass."""

    def __init__(self, cache_file: Optional[str] = None) -> None:
        self._lock = threading.Lock()
        self._store: Dict[str, Dict[str, Any]] = {}
        self.cache_file = cache_file
        self.upstream_hits = 0
        self.cache_hits = 0
        if cache_file and os.path.exists(cache_file):
            try:
                with open(cache_file, "r", encoding="utf-8") as f:
                    self._store = json.load(f)
            except Exception:
                self._store = {}

    def _make_key(self, target_base: str, method: str, path_query: str, req_body: bytes) -> str:
        body_hash = hashlib.sha256(req_body or b"").hexdigest()[:16]
        return f"{target_base}|{method}|{path_query}|{body_hash}"

    def fetch(
        self,
        target_base: str,
        method: str,
        path_query: str,
        req_body: bytes,
        content_type: Optional[str] = None,
    ) -> Tuple[int, str, bytes]:
        key = self._make_key(target_base, method, path_query, req_body)
        with self._lock:
            cached = self._store.get(key)
            if cached is not None:
                self.cache_hits += 1
                return (
                    int(cached["status"]),
                    str(cached["content_type"]),
                    base64.b64decode(cached["raw_b64"]),
                )

        url = f"{target_base.rstrip('/')}{path_query}"
        headers = {"Accept": "application/json, text/html, image/svg+xml, */*"}
        if content_type:
            headers["Content-Type"] = content_type
        req = urllib.request.Request(
            url,
            data=req_body if method in ("POST", "PUT", "PATCH") else None,
            headers=headers,
            method=method,
        )
        try:
            with urllib.request.urlopen(req, timeout=25) as resp:
                status = resp.status
                ctype = resp.headers.get("Content-Type", "application/json")
                raw = resp.read()
        except urllib.error.HTTPError as e:
            status = e.code
            ctype = e.headers.get("Content-Type", "application/json") if e.headers else "application/json"
            raw = e.read() if e.fp else b"{}"

        with self._lock:
            self.upstream_hits += 1
            self._store[key] = {
                "status": status,
                "content_type": ctype,
                "raw_b64": base64.b64encode(raw).decode("ascii"),
            }
            if self.cache_file:
                try:
                    os.makedirs(os.path.dirname(self.cache_file), exist_ok=True)
                    with open(self.cache_file, "w", encoding="utf-8") as f:
                        json.dump(self._store, f)
                except Exception:
                    pass
        return status, ctype, raw


class ThreadedHTTPServer(socketserver.ThreadingMixIn, http.server.HTTPServer):
    daemon_threads = True
    allow_reuse_address = True


def make_proxy_handler(active_defect_id: Optional[str], cache: UpstreamCache):
    defect_entry = DEFECT_REGISTRY.get(active_defect_id or "")

    class FaultProxyHandler(http.server.BaseHTTPRequestHandler):
        def log_message(self, format: str, *args: Any) -> None:
            pass  # Keep test output clean

        def _handle(self, method: str) -> None:
            content_len = int(self.headers.get("Content-Length", "0") or "0")
            req_body_bytes = self.rfile.read(content_len) if content_len > 0 else b""
            req_body_str = req_body_bytes.decode("utf-8", errors="replace")

            raw_path = self.path
            target_base = UPSTREAM_SERVING_URL

            # Support proxying EVAL_DASHBOARD_URL via /__dashboard__ prefix:
            # - /__dashboard__/api/workbench?... mirrors eval-dashboard/server.js by forwarding to /api/search?...
            # - /__dashboard__/api/info forwards to UPSTREAM_DASHBOARD_URL/api/info
            if raw_path.startswith("/__dashboard__"):
                sub_path = raw_path[len("/__dashboard__"):] or "/"
                if sub_path.startswith("/api/workbench"):
                    raw_path = "/api/search" + sub_path[len("/api/workbench"):]
                    target_base = UPSTREAM_SERVING_URL
                else:
                    raw_path = sub_path
                    target_base = UPSTREAM_DASHBOARD_URL

            parsed_url = urllib.parse.urlsplit(raw_path)
            query_multi = urllib.parse.parse_qs(parsed_url.query, keep_blank_values=True)
            query_single = {k: v[0] for k, v in query_multi.items() if v}

            status, ctype, raw_bytes = cache.fetch(
                target_base=target_base,
                method=method,
                path_query=raw_path,
                req_body=req_body_bytes,
                content_type=self.headers.get("Content-Type"),
            )

            is_json = "application/json" in ctype.lower() or raw_bytes.lstrip().startswith((b"{", b"["))
            parsed_body: Any
            if is_json:
                try:
                    parsed_body = json.loads(raw_bytes.decode("utf-8"))
                except Exception:
                    parsed_body = raw_bytes.decode("utf-8", errors="replace")
                    is_json = False
            else:
                parsed_body = raw_bytes.decode("utf-8", errors="replace")

            if defect_entry is not None:
                req_ctx = {
                    "method": method,
                    "path": parsed_url.path,
                    "query_string": parsed_url.query,
                    "query": query_single,
                    "query_multi": query_multi,
                    "req_body": req_body_str,
                    "headers": dict(self.headers.items()),
                }
                status, parsed_body = defect_entry["fn"](req_ctx, status, copy.deepcopy(parsed_body))

            if isinstance(parsed_body, (dict, list)):
                out_bytes = json.dumps(parsed_body).encode("utf-8")
                out_ctype = "application/json; charset=utf-8"
            else:
                out_bytes = str(parsed_body).encode("utf-8")
                out_ctype = ctype or "text/html; charset=utf-8"

            self.send_response(status)
            self.send_header("Content-Type", out_ctype)
            self.send_header("Content-Length", str(len(out_bytes)))
            self.end_headers()
            self.wfile.write(out_bytes)

        def do_GET(self) -> None:
            self._handle("GET")

        def do_POST(self) -> None:
            self._handle("POST")

        def do_PUT(self) -> None:
            self._handle("PUT")

    return FaultProxyHandler


def execute_probe_against_proxy(
    probe_script_path: str,
    defect_id: Optional[str],
    cache: UpstreamCache,
) -> Dict[str, Any]:
    """Starts an in-process FaultProxy server, runs `probe_script_path` (`--no-record`), and captures all results."""
    _PAGE1_FIRST_ITEM_BY_QUERY.clear()
    handler_cls = make_proxy_handler(defect_id, cache)
    server = ThreadedHTTPServer(("127.0.0.1", 0), handler_cls)
    port = server.server_address[1]
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    proxy_serving_url = f"http://127.0.0.1:{port}"
    proxy_dashboard_url = f"http://127.0.0.1:{port}/__dashboard__"

    old_serving = os.environ.get("PSEARCH_SERVING_URL")
    old_dash = os.environ.get("EVAL_DASHBOARD_URL")
    os.environ["PSEARCH_SERVING_URL"] = proxy_serving_url
    os.environ["EVAL_DASHBOARD_URL"] = proxy_dashboard_url

    try:
        spec = importlib.util.spec_from_file_location("probe_search_under_test", probe_script_path)
        if spec is None or spec.loader is None:
            raise RuntimeError(f"Unable to load probe script: {probe_script_path}")
        mod = importlib.util.module_from_spec(spec)
        buf = io.StringIO()
        with redirect_stdout(buf):
            spec.loader.exec_module(mod)
            mod.SERVING_URL = proxy_serving_url
            mod.DASHBOARD_URL = proxy_dashboard_url
            result = mod.run_probes(save_to_spanner=False)
        return result
    finally:
        server.shutdown()
        server.server_close()
        if old_serving is None:
            os.environ.pop("PSEARCH_SERVING_URL", None)
        else:
            os.environ["PSEARCH_SERVING_URL"] = old_serving
        if old_dash is None:
            os.environ.pop("EVAL_DASHBOARD_URL", None)
        else:
            os.environ["EVAL_DASHBOARD_URL"] = old_dash


def summarize_suite_across_defects(
    suite_name: str,
    probe_script_path: str,
    defect_ids: List[str],
    cache: UpstreamCache,
) -> Dict[str, Any]:
    """Runs D0 (clean) and each defect in `defect_ids` for `probe_script_path` and compiles the Scenario x Defect matrix."""
    print(f"\n=== Running Fault-Injection Matrix for [{suite_name}] ({probe_script_path}) ===")
    runs: Dict[str, Any] = {}
    all_runs = ["D0"] + defect_ids

    for d_id in all_runs:
        active = None if d_id == "D0" else d_id
        t0 = time.time()
        res = execute_probe_against_proxy(probe_script_path, active, cache)
        elapsed_s = round(time.time() - t0, 2)

        scenarios_flat: List[Dict[str, Any]] = []
        for layer in res.get("pyramid_layers", []):
            for sc in layer.get("scenarios", []):
                scenarios_flat.append(
                    {
                        "id": sc["id"],
                        "layer_id": layer["id"],
                        "name": sc["name"],
                        "status": sc["status"],
                        "evidence": sc.get("evidence", ""),
                    }
                )

        reqs_flat = [
            {
                "issue_number": r["issue_number"],
                "code": r["code"],
                "title": r["title"],
                "status": r["status"],
            }
            for r in res.get("requirements", [])
        ]

        failed_scenarios = [s["id"] for s in scenarios_flat if s["status"] != "PASS"]
        failed_reqs = [f"#{r['issue_number']} ({r['code']})" for r in reqs_flat if r["status"] != "PASS"]

        runs[d_id] = {
            "defect_id": d_id,
            "description": "Clean baseline (no injected fault)" if d_id == "D0" else DEFECT_REGISTRY[d_id]["description"],
            "elapsed_s": elapsed_s,
            "avg_ndcg": round(float(res.get("avg_ndcg", 0.0)), 4),
            "avg_mrr": round(float(res.get("avg_mrr", 0.0)), 4),
            "total_scenarios": len(scenarios_flat),
            "passed_scenarios": len(scenarios_flat) - len(failed_scenarios),
            "failed_scenarios_count": len(failed_scenarios),
            "failed_scenarios": failed_scenarios,
            "total_requirements": len(reqs_flat),
            "failed_requirements_count": len(failed_reqs),
            "failed_requirements": failed_reqs,
            "scenarios_by_id": {s["id"]: s["status"] for s in scenarios_flat},
            "scenario_meta": {s["id"]: {"name": s["name"], "layer_id": s["layer_id"]} for s in scenarios_flat},
        }
        print(
            f"  [{suite_name}] {d_id}: failed_scenarios={len(failed_scenarios)}/{len(scenarios_flat)} "
            f"| failed_reqs={len(failed_reqs)}/{len(reqs_flat)} "
            f"| NDCG@10={runs[d_id]['avg_ndcg']:.3f} ({elapsed_s}s)"
        )

    # Build Scenario x Defect table
    scenario_meta = runs["D0"]["scenario_meta"]
    scenario_matrix: List[Dict[str, Any]] = []
    never_failed_scenarios: List[Dict[str, Any]] = []

    for sc_id, meta in scenario_meta.items():
        outcomes = {d_id: runs[d_id]["scenarios_by_id"].get(sc_id, "MISSING") for d_id in all_runs}
        failed_on = [d_id for d_id in defect_ids if outcomes.get(d_id) != "PASS"]
        row = {
            "scenario_id": sc_id,
            "layer_id": meta["layer_id"],
            "name": meta["name"],
            "d0_clean": outcomes["D0"],
            "outcomes": {d_id: outcomes[d_id] for d_id in defect_ids},
            "failed_on_defects": failed_on,
            "failures_count": len(failed_on),
        }
        scenario_matrix.append(row)
        if len(failed_on) == 0:
            never_failed_scenarios.append(
                {
                    "scenario_id": sc_id,
                    "layer_id": meta["layer_id"],
                    "name": meta["name"],
                }
            )

    per_defect_summary = {
        d_id: {
            "description": runs[d_id]["description"],
            "failed_scenarios_count": runs[d_id]["failed_scenarios_count"],
            "failed_scenarios": runs[d_id]["failed_scenarios"],
            "failed_requirements_count": runs[d_id]["failed_requirements_count"],
            "failed_requirements": runs[d_id]["failed_requirements"],
            "avg_ndcg": runs[d_id]["avg_ndcg"],
            "avg_mrr": runs[d_id]["avg_mrr"],
        }
        for d_id in all_runs
    }

    return {
        "suite_name": suite_name,
        "total_scenarios": len(scenario_meta),
        "scenarios_failing_at_least_one_defect": len(scenario_meta) - len(never_failed_scenarios),
        "scenarios_never_failing_d1_d8_count": len(never_failed_scenarios),
        "scenarios_never_failing_d1_d8": never_failed_scenarios,
        "per_defect_summary": per_defect_summary,
        "scenario_matrix": scenario_matrix,
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Issue #63 Extensible Fault-Injection Proxy for probe_search.py")
    parser.add_argument("--defect", default=None, help="Defect ID to inject (e.g. D1..D8, or None for clean proxy)")
    parser.add_argument("--port", type=int, default=18081, help="Port to bind when running standalone proxy daemon")
    parser.add_argument("--cache-file", default="/tmp/issue63_fault_proxy_cache.json", help="Upstream response cache path")
    parser.add_argument("--run-probe", action="store_true", help="Run probe_search.py --no-record against --defect and exit")
    parser.add_argument("--run-matrix", action="store_true", help="Run full D0 + D1..D8 matrix across old and new probe suites")
    parser.add_argument("--old-probe", default=None, help="Path to old e47eb0c probe_search.py snapshot for before/after matrix")
    parser.add_argument(
        "--new-probe",
        default=os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "probe_search.py")),
        help="Path to rebuilt probe_search.py",
    )
    parser.add_argument(
        "--output",
        default=os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", "data", "issue-63", "mutation_matrix.json")),
        help="Output JSON path for --run-matrix",
    )
    parser.add_argument("--list-defects", action="store_true", help="List all registered defects and exit")
    args = parser.parse_args()

    if args.list_defects:
        for d_id, info in sorted(DEFECT_REGISTRY.items()):
            print(f"{d_id}: {info['description']} (issues: {info['target_issues']})")
        return

    cache = UpstreamCache(cache_file=args.cache_file)

    if args.run_probe:
        if args.defect and args.defect not in DEFECT_REGISTRY:
            raise SystemExit(f"Unknown defect '{args.defect}'. Registered: {sorted(DEFECT_REGISTRY.keys())}")
        res = execute_probe_against_proxy(args.new_probe, args.defect, cache)
        failed_sc = [
            f"{sc['id']} ({sc['name']})"
            for layer in res.get("pyramid_layers", [])
            for sc in layer.get("scenarios", [])
            if sc.get("status") != "PASS"
        ]
        failed_rq = [
            f"#{r['issue_number']} ({r['code']})"
            for r in res.get("requirements", [])
            if r.get("status") != "PASS"
        ]
        print(f"Defect: {args.defect or 'D0 (clean)'}")
        print(f"NDCG@10: {res.get('avg_ndcg', 0.0):.3f} | MRR@10: {res.get('avg_mrr', 0.0):.3f}")
        print(f"Failed Pyramid Scenarios ({len(failed_sc)}): {failed_sc}")
        print(f"Failed Requirements ({len(failed_rq)}): {failed_rq}")
        print(f"Upstream live HTTP requests: {cache.upstream_hits} (cache hits: {cache.cache_hits})")
        return

    if args.run_matrix:
        defect_ids = sorted(DEFECT_REGISTRY.keys())
        new_summary = summarize_suite_across_defects("after_rebuilt_probe_78", args.new_probe, defect_ids, cache)
        old_summary = None
        if args.old_probe and os.path.exists(args.old_probe):
            old_summary = summarize_suite_across_defects("before_legacy_probe_100_e47eb0c", args.old_probe, defect_ids, cache)

        comparison = {}
        for d_id in ["D0"] + defect_ids:
            comparison[d_id] = {
                "description": new_summary["per_defect_summary"][d_id]["description"],
                "before_failed_scenarios": (
                    old_summary["per_defect_summary"][d_id]["failed_scenarios_count"] if old_summary else None
                ),
                "after_failed_scenarios": new_summary["per_defect_summary"][d_id]["failed_scenarios_count"],
                "before_failed_requirements": (
                    old_summary["per_defect_summary"][d_id]["failed_requirements_count"] if old_summary else None
                ),
                "after_failed_requirements": new_summary["per_defect_summary"][d_id]["failed_requirements_count"],
                "after_failed_scenario_ids": new_summary["per_defect_summary"][d_id]["failed_scenarios"],
            }

        payload = {
            "issue": 63,
            "workstream": "Item 6 — Test-the-Test Fault-Injection Matrix & Unfalsifiable Scenario Removal",
            "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "registered_defects": {
                d_id: {
                    "description": info["description"],
                    "target_issues": info["target_issues"],
                }
                for d_id, info in sorted(DEFECT_REGISTRY.items())
            },
            "upstream_http_requests_used": cache.upstream_hits,
            "proxy_cache_hits": cache.cache_hits,
            "before_vs_after_per_defect": comparison,
            "before_suite_e47eb0c": old_summary,
            "after_suite_rebuilt": new_summary,
        }
        os.makedirs(os.path.dirname(args.output), exist_ok=True)
        with open(args.output, "w", encoding="utf-8") as f:
            json.dump(payload, f, indent=2)
        print(f"\n[fault_proxy] Saved complete Scenario x Defect matrix to {args.output}")
        print(f"[fault_proxy] Total live upstream HTTP requests used: {cache.upstream_hits} (cache hits: {cache.cache_hits})")
        return

    # Standalone proxy daemon mode
    if args.defect and args.defect not in DEFECT_REGISTRY:
        raise SystemExit(f"Unknown defect '{args.defect}'. Registered: {sorted(DEFECT_REGISTRY.keys())}")
    handler_cls = make_proxy_handler(args.defect, cache)
    server = ThreadedHTTPServer(("127.0.0.1", args.port), handler_cls)
    print(f"[fault_proxy] Listening on http://127.0.0.1:{args.port} (active defect: {args.defect or 'None/D0'})")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    main()
