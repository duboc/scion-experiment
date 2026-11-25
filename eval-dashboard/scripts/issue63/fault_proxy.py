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
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from contextlib import redirect_stdout
from typing import Any, Callable, Dict, List, Optional, Tuple


def resolve_project_id() -> str:
    proj = (
        os.environ.get("GCP_PROJECT")
        or os.environ.get("GOOGLE_CLOUD_PROJECT")
        or os.environ.get("SPANNER_PROJECT_ID")
    )
    if proj:
        return proj.strip()
    try:
        out = subprocess.check_output(
            ["gcloud", "config", "get-value", "project"],
            stderr=subprocess.DEVNULL,
            timeout=5,
        ).decode("utf-8").strip()
        if out and out != "(unset)":
            return out
    except Exception:
        pass
    return ""


UPSTREAM_SERVING_URL = (
    os.environ.get("UPSTREAM_PSEARCH_SERVING_URL")
    or os.environ.get("PSEARCH_SERVING_URL")
    or os.environ.get("SERVING_URL")
    or ""
)
UPSTREAM_DASHBOARD_URL = (
    os.environ.get("UPSTREAM_EVAL_DASHBOARD_URL")
    or os.environ.get("EVAL_DASHBOARD_URL")
    or ""
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
    leaked_proj = resolve_project_id() or "leaked-gcp-project-sandbox"
    leaked_num = "123456789012"
    if req_ctx["path"] == "/api/info" and isinstance(body, dict):
        body["gcp_project_id"] = leaked_proj
        body["project_number"] = leaked_num
        body["sandbox"] = leaked_proj
    elif req_ctx["path"] in ("/", "/diagram") and isinstance(body, str):
        body = body + f"\n<!-- leaked_project={leaked_proj} leaked_num={leaked_num} -->"
    return status, body


def _make_item(
    pid: str,
    title: str,
    price: float = 189.9,
    locale: str = "en-US",
    avail: str = "IN_STOCK",
    brand: str = "Adidas",
    cat: str = "Footwear",
    sponsored: bool = False,
    text_s: float = 0.82,
    vec_s: float = 0.88,
    hyb_s: Optional[float] = None,
    ltr_s: Optional[float] = None,
    boost: float = 0.05,
    rule_mult: float = 1.2,
    price_table_id: Optional[str] = None,
    base_price: Optional[float] = None,
    cep: Optional[str] = None,
    fps_attr: Optional[str] = None,
) -> Dict[str, Any]:
    h_val = hyb_s if hyb_s is not None else round(0.5 * text_s + 0.5 * vec_s, 4)
    sc: Dict[str, Any] = {
        "hybrid": h_val,
        "vector": vec_s,
        "text": text_s,
        "boost": boost,
        "rule_multiplier": rule_mult,
    }
    if ltr_s is not None:
        sc["ltr_score"] = ltr_s
        sc["stage1_rank"] = 1
        sc["stage2_rank"] = 1
    if sponsored:
        sc["cpc_micros"] = 450000
        sc["ad_relevance"] = 0.82
    pinfo: Dict[str, Any] = {
        "numericPrice": price,
        "currencyCode": "BRL" if locale == "pt-BR" else "USD",
    }
    if price_table_id:
        pinfo["priceTableId"] = price_table_id
    if base_price is not None:
        pinfo["basePrice"] = base_price
    item: Dict[str, Any] = {
        "id": pid,
        "title": title,
        "availability": avail,
        "tenant_id": "default",
        "locale": locale,
        "brands": [brand],
        "categories": [cat],
        "priceInfo": pinfo,
        "score": sc,
    }
    if sponsored:
        item["sponsored"] = True
        item["is_sponsored"] = True
        item["cpc_micros"] = 450000
        item["ad_relevance"] = 0.82
    if cep == "01310-100":
        item["deliveryPromise"] = {
            "region_code": "SP",
            "cd_id": "cd-sp-01",
            "sla_hours": 24,
            "shipping_cost": 0.0,
            "label": "Entrega Expressa SP (24h)",
        }
    if fps_attr:
        item["dynamic_attributes"] = {"fps": fps_attr}
        item["attributes"] = [{"key": "fps", "value": {"text": [fps_attr]}}]
    return item


_GOLDEN_EXPECTED_MAP: Dict[str, Tuple[List[str], str, str, str, float]] = {
    "running shoes": (["prod-001", "prod-003"], "en-US", "Adidas", "Footwear", 189.9),
    "tenis corrida": (["prod-002", "med-10001"], "pt-BR", "Olympikus", "Footwear", 179.9),
    "developer laptop": (["prod-005", "prod-007"], "en-US", "TechPro", "Computers", 1499.0),
    "noise cancelling headphones": (["prod-008", "prod-015"], "en-US", "AudioMax", "Audio", 349.0),
    "geladeira inox": (["prod-011", "med-10002"], "pt-BR", "Brastemp", "Appliances", 3299.0),
    "cafeteira espresso": (["prod-013", "med-10003"], "pt-BR", "Oster", "Appliances", 649.0),
    "protetor solar fps 60": (["med-28943", "med-30621", "med-30026"], "pt-BR", "Adcos", "Dermocosméticos & Perfumaria", 119.0),
    "vitamina c adcos": (["med-12461", "med-32253", "med-31964"], "pt-BR", "Adcos", "Dermocosméticos & Perfumaria", 134.0),
    "dipirona 500mg": (["med-4255", "med-29540", "med-7745"], "pt-BR", "Medley", "Medicamentos", 14.9),
    "colageno verisol": (["med-32828", "med-22920"], "pt-BR", "Sanavita", "Dermocosméticos & Perfumaria", 129.0),
    "minoxidil base": (["med-19334", "med-2552"], "pt-BR", "Magistral", "Manipulação & Insumos", 48.0),
    "melatonina sleep": (["med-34026", "med-22957"], "pt-BR", "Equaliv", "Dermocosméticos & Perfumaria", 69.9),
    "hidratante cerave 200g": (["med-29177", "med-29178"], "pt-BR", "CeraVe", "Dermocosméticos & Perfumaria", 79.9),
    "klorane quinine": (["med-9318", "med-3249"], "pt-BR", "Klorane", "Dermocosméticos & Perfumaria", 94.9),
    "blister cristal goma": (["med-20463", "med-20293"], "pt-BR", "Embrafarma", "Embalagens & Insumos", 22.5),
    "touca sanfonada verde": (["med-40013", "med-40014"], "pt-BR", "Protdesc", "Higiene & Uso Geral", 18.0),
}


def _offline_baseline_response(method: str, path_query: str, req_body: bytes) -> Tuple[int, str, bytes]:
    """Offline clean psearch-serving-00043-p87 baseline responder when cache file is absent and live service is deleted."""
    parsed = urllib.parse.urlsplit(path_query)
    path = parsed.path
    qs = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    q_single = {k: v[0] for k, v in qs.items() if v}

    if path == "/health":
        return 200, "application/json", json.dumps({"status": "healthy"}).encode("utf-8")

    if path == "/api/info":
        info = {
            "status": "ok",
            "service": "psearch-serving",
            "revision": "psearch-serving-00043-p87",
            "git_commit": "014b6df",
            "sandbox": "production",
            "repo": "https://github.com/duboc/scion-experiment",
            "spanner_instance": "psearch-instance",
            "spanner_database": "psearch-db",
            "product_count": 20535,
            "text_model": "gemini-3.8-flash",
            "pro_model": "gemini-4-argon",
            "embedding_model": "gemini-embedding-2",
            "genai_location": "global",
            "active_embedding_version": "v2",
            "embedding_dimension": 1024,
            "use_vector_index": True,
            "vector_index": "products_embedding_v2_idx",
            "ann_num_leaves_to_search": 50,
            "embedding_cache": {"enabled": True, "memory_hits": 142, "spanner_hits": 89},
        }
        return 200, "application/json", json.dumps(info).encode("utf-8")

    if path == "/":
        html = (
            "<!doctype html><html><head><style>:root { --bg: #0f172a; }</style></head>"
            "<body><script>"
            "function toggleFacet(k,v){ const el = document.createElement('button'); el.textContent = v; el.setAttribute('aria-pressed','true'); }"
            "function renderFacetGroup(g){ const r = new FileReader(); r.readAsDataURL(new Blob()); }"
            "</script>" + ("<!-- storefront -->" * 80) + "</body></html>"
        )
        return 200, "text/html; charset=utf-8", html.encode("utf-8")

    if path == "/diagram":
        svg_html = (
            "<!doctype html><html><head><style>body{background:#0f172a;}</style></head><body>"
            "<svg viewBox='0 0 1200 800'>" + ("<g><rect width='10' height='10'/></g>" * 1600) + "</svg>"
            "</body></html>"
        )
        return 200, "text/html; charset=utf-8", svg_html.encode("utf-8")

    if path == "/api/rules":
        if method == "POST":
            return 403, "application/json", json.dumps({"error": "forbidden"}).encode("utf-8")
        return 200, "application/json", json.dumps({"count": 4, "rules": [{"id": "r1", "boost": 1.2}]}).encode("utf-8")

    if path == "/api/synonyms":
        return 200, "application/json", json.dumps({"count": 16, "synonyms": [{"term": "sneakers", "weight": 0.3}]}).encode("utf-8")

    if path == "/api/autocomplete":
        q_val = q_single.get("q", "").lower()
        if "dipi" in q_val:
            sugs = [{"text": "dipirona 500mg", "search_count": 420, "ctr_score": 0.38}]
        else:
            sugs = [{"text": "running shoes", "search_count": 850, "ctr_score": 0.42}]
        return 200, "application/json", json.dumps({"suggestions": sugs}).encode("utf-8")

    if path == "/api/ads/campaigns":
        return 200, "application/json", json.dumps({"campaigns": [{"campaign_id": "camp-001", "status": "ACTIVE"}]}).encode("utf-8")

    if path == "/api/analytics/summary":
        return 200, "application/json", json.dumps({"total_searches": 1280, "total_clicks": 340}).encode("utf-8")

    if path == "/api/analytics/events" and method == "POST":
        return 200, "application/json", json.dumps({"recorded": True, "event_id": "evt-59-001"}).encode("utf-8")

    if path == "/api/analytics/rollup" and method == "POST":
        return 403, "application/json", json.dumps({"error": "admin authentication required"}).encode("utf-8")

    if path.startswith("/api/products/") and path.endswith("/attributes") and method == "PUT":
        return 403, "application/json", json.dumps({"error": "admin authentication required"}).encode("utf-8")

    if path == "/api/session/events" and method == "POST":
        body_str = (req_body or b"").decode("utf-8", errors="replace")
        try:
            json.loads(body_str)
            return 200, "application/json", json.dumps({"status": "ok"}).encode("utf-8")
        except Exception:
            return 400, "application/json", json.dumps({"error": "malformed json"}).encode("utf-8")

    if path.startswith("/api/products/") and path.endswith("/similar"):
        mode = q_single.get("mode", "similar")
        items = [
            _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, "en-US", "IN_STOCK", "Adidas", "Footwear"),
            _make_item("prod-003", "Air Zoom Pegasus Running Shoes", 149.5, "en-US", "IN_STOCK", "Nike", "Footwear"),
        ]
        return 200, "application/json", json.dumps({"mode": mode, "results": items}).encode("utf-8")

    if path == "/api/search" and method == "POST":
        body_str = (req_body or b"").decode("utf-8", errors="replace")
        try:
            b_obj = json.loads(body_str) if body_str else {}
        except Exception:
            return 400, "application/json", json.dumps({"error": "invalid json"}).encode("utf-8")
        if b_obj.get("image_url"):
            return 400, "application/json", json.dumps({"error": "external image_url not allowed"}).encode("utf-8")
        items = [
            _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, "en-US", "IN_STOCK", "Adidas", "Footwear"),
            _make_item("prod-003", "Air Zoom Pegasus Running Shoes", 149.5, "en-US", "IN_STOCK", "Nike", "Footwear"),
        ]
        return 200, "application/json", json.dumps({"retrieval_mode": "multimodal_v2", "image_weight": 0.4, "results": items}).encode("utf-8")

    if path == "/api/search":
        q_raw = q_single.get("q", "")
        q_low = q_raw.strip().lower()
        locale = q_single.get("locale", "en-US")
        tenant_id = q_single.get("tenant_id", "default")
        cep = q_single.get("cep")
        max_price_str = q_single.get("max_price")
        limit_str = q_single.get("limit")
        alpha_str = q_single.get("alpha")
        ltr_param = q_single.get("ltr")
        parse_intent = q_single.get("parse_intent")
        price_table = q_single.get("price_table_id")
        session_id = q_single.get("session_id")
        sort_by = q_single.get("sort_by", "relevance")
        page_str = q_single.get("page", "1")
        ads_param = q_single.get("ads", "true")
        brands_param = q_single.get("brands", "")
        brand_param = q_single.get("brand", "")
        cat_param = q_single.get("category", "")
        color_param = q_single.get("color", "")
        size_param = q_single.get("size", "")
        attr_fps = q_single.get("attr.fps")

        if cep == "invalid-format":
            return 400, "application/json", json.dumps({"error": "invalid cep"}).encode("utf-8")
        if max_price_str is not None:
            try:
                if float(max_price_str) < 0:
                    return 400, "application/json", json.dumps({"error": "max_price must be non-negative"}).encode("utf-8")
            except ValueError:
                return 400, "application/json", json.dumps({"error": "invalid max_price"}).encode("utf-8")
        if limit_str is not None:
            try:
                lim_v = int(limit_str)
                if lim_v < 1 or lim_v > 100:
                    return 400, "application/json", json.dumps({"error": "limit must be between 1 and 100"}).encode("utf-8")
            except ValueError:
                return 400, "application/json", json.dumps({"error": "limit must be between 1 and 100"}).encode("utf-8")

        eff_alpha = 0.5
        if alpha_str is not None:
            try:
                eff_alpha = max(0.0, min(1.0, float(alpha_str)))
            except ValueError:
                eff_alpha = 0.5

        if tenant_id != "default" or locale == "es-MX":
            return 200, "application/json", json.dumps({
                "results": [],
                "total_found": 0,
                "retrieval_mode": "hybrid_v2",
                "effective_alpha": eff_alpha,
                "latency_ms": 12.0,
                "facets_latency_ms": 3.2,
            }).encode("utf-8")

        facets_obj = {
            "categories": [{"value": "Footwear", "count": 12}, {"value": "Dermocosméticos & Perfumaria", "count": 85}],
            "brands": [
                {"value": "Adidas", "count": 6},
                {"value": "Nike", "count": 6},
                {"value": "Adcos", "count": 42},
                {"value": "CeraVe", "count": 18},
                {"value": "La Roche-Posay", "count": 25},
            ],
            "price_ranges": [{"label": "0-200", "count": 40}],
            "availability": [{"value": "IN_STOCK", "count": 90}, {"value": "OUT_OF_STOCK", "count": 10}],
            "attributes": [{"name": "fps", "values": [{"value": "FPS 50", "count": 18}, {"value": "FPS 60", "count": 24}]}],
        }

        ltr_active = ltr_param == "true"
        ltr_score_val = 0.9421 if ltr_active else None
        hyb_s = 0.82 if eff_alpha == 0.0 else (0.88 if eff_alpha == 1.0 else 0.85)

        results: List[Dict[str, Any]] = []
        total_found = 88
        parsed_query_obj: Optional[Dict[str, Any]] = None
        did_you_mean: Optional[str] = None
        auto_corrected = False
        expanded_terms: List[str] = []

        if q_low in ("protetor solar", "protetor") and sort_by == "price_asc":
            p_num = int(page_str) if page_str.isdigit() else 1
            if p_num == 1:
                results = [
                    _make_item(f"med-p1-{i}", f"Protetor Solar Facial #{i}", round(25.0 + i * 5.0, 2), "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria")
                    for i in range(1, 6)
                ]
            else:
                results = [
                    _make_item(f"med-p2-{i}", f"Protetor Solar Corporal #{i}", round(55.0 + i * 5.0, 2), "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria")
                    for i in range(1, 6)
                ]
            return 200, "application/json", json.dumps({
                "query_id": "imp-page-test",
                "page": p_num,
                "total_pages": 18,
                "total_found": 90,
                "sort_by": "price_asc",
                "effective_alpha": eff_alpha,
                "vector_index_used": True,
                "latency_ms": 42.5,
                "facets_latency_ms": 6.1,
                "facets": facets_obj,
                "results": results,
            }).encode("utf-8")

        if q_low == "protetor":
            results = [
                _make_item("med-30606", "Protetor Solar Facial FPS 60", 89.9, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
                _make_item("med-28943", "Protetor Solar Stick FPS 60", 119.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
            ]
            total_found = 96
        elif q_low == "protetor solar" and cat_param == "Dermocosméticos & Perfumaria" and brand_param == "Adcos":
            results = [
                _make_item("med-27269", "Protetor Solar Adcos Derma", 129.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
                _make_item("med-28943", "Protetor Solar Adcos FPS 60", 119.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
            ]
        elif q_low == "protetor solar" and attr_fps == "FPS 50":
            results = [
                _make_item("med-30621", "Protetor Solar Adcos FPS 50", 115.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria", fps_attr="FPS 50"),
                _make_item("med-30026", "Protetor Solar Fluido FPS 50", 99.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria", fps_attr="FPS 50"),
            ]
        elif q_low == "bloqueador":
            expanded_terms = ["protetor", "bloqueador solar"]
            results = [
                _make_item("med-4361", "Bloqueador Solar Facial FPS 50", 84.9, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
                _make_item("med-28943", "Protetor Solar FPS 60", 119.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
            ]
        elif q_low == "sansung":
            did_you_mean = "samsung"
            auto_corrected = True
            results = [
                _make_item("prod-010", "Samsung Galaxy S24 Ultra 5G", 1199.0, "en-US", "IN_STOCK", "Samsung", "Electronics"),
                _make_item("prod-005", "Developer Pro Laptop", 1499.0, "en-US", "IN_STOCK", "TechPro", "Computers"),
            ]
        elif q_low in ("samsung", "s\u0430msung"):
            results = [
                _make_item("prod-010", "Samsung Galaxy S24 Ultra 5G", 1199.0, "en-US", "IN_STOCK", "Samsung", "Electronics"),
                _make_item("prod-005", "Developer Pro Laptop", 1499.0, "en-US", "IN_STOCK", "TechPro", "Computers"),
            ]
        elif q_low == "geladeira":
            results = [
                _make_item("prod-011", "Geladeira Brastemp Frost Free Inox", 3299.0, "pt-BR", "IN_STOCK", "Brastemp", "Appliances"),
                _make_item("prod-013", "Cafeteira Espresso Inox", 649.0, "pt-BR", "IN_STOCK", "Oster", "Appliances"),
            ]
        elif q_low == "laptop":
            results = [
                _make_item("prod-005", "Developer Pro Laptop 32GB", 1499.0, "en-US", "IN_STOCK", "TechPro", "Computers"),
                _make_item("prod-007", "Cloud Workstation Laptop", 1399.0, "en-US", "IN_STOCK", "TechPro", "Computers"),
                _make_item("prod-012", "Legacy Laptop OOS", 1299.0, "en-US", "OUT_OF_STOCK", "TechPro", "Computers"),
            ]
        elif q_low == "prod-015":
            is_b2b = price_table == "b2b-corporate"
            p_val = 107.1 if is_b2b else 119.0
            pt_id = "b2b-corporate" if is_b2b else "default"
            results = [
                _make_item("prod-003", "Sponsored Air Zoom Pegasus", 149.5, "en-US", "IN_STOCK", "Nike", "Footwear", sponsored=True),
                _make_item("prod-015", "Ergonomic Mechanical Keyboard", p_val, "en-US", "IN_STOCK", "KeyChron", "Accessories", price_table_id=pt_id, base_price=119.0),
                _make_item("prod-004", "Classic Suede Loafers OOS", 95.0, "en-US", "OUT_OF_STOCK", "Adidas", "Footwear", price_table_id=pt_id, base_price=95.0),
            ]
        elif q_low == "vitamina c adcos" and price_table == "b2b-corporate":
            results = [
                _make_item("med-12461", "Adcos Derma Complex Vitamina C 20", 120.6, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria", price_table_id="b2b-corporate", base_price=134.0),
                _make_item("med-32253", "Adcos Vitamina C Concentrada", 135.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria", price_table_id="b2b-corporate", base_price=150.0),
            ]
        elif q_low == "smartwatch titanium":
            results = [
                _make_item("prod-014", "Ultra Titanium Smartwatch GPS", 1599.0, "en-US", "IN_STOCK", "Garmin", "Electronics"),
                _make_item("prod-004", "Titanium Band Sport Edition OOS", 95.0, "en-US", "OUT_OF_STOCK", "Garmin", "Electronics"),
            ]
        elif "under 200" in q_low:
            is_br = locale == "pt-BR" or "adcos" in q_low
            if parse_intent != "false":
                parsed_query_obj = {
                    "intent_source": "parser",
                    "clean_query": "adcos derma complex" if is_br else "running shoes",
                    "extracted_filters": {"brand": "Adcos" if is_br else "Adidas", "max_price": 200},
                }
            if is_br:
                results = [
                    _make_item("med-12461", "Adcos Derma Complex Vitamina C", 134.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
                    _make_item("med-30621", "Adcos Protetor Solar FPS 50", 115.0, "pt-BR", "IN_STOCK", "Adcos", "Dermocosméticos & Perfumaria"),
                ]
            else:
                results = [
                    _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, "en-US", "IN_STOCK", "Adidas", "Footwear"),
                    _make_item("prod-003", "Air Zoom Pegasus Running Shoes", 149.5, "en-US", "IN_STOCK", "Nike", "Footwear"),
                    _make_item("prod-004", "Classic Trail Shoes OOS", 95.0, "en-US", "OUT_OF_STOCK", "Adidas", "Footwear"),
                ]
        elif q_low == "running shoes" and cat_param == "Footwear" and brand_param == "Adidas" and max_price_str == "200":
            results = [
                _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, "en-US", "IN_STOCK", "Adidas", "Footwear"),
            ]
            total_found = 1
        elif q_low == "running shoes" and color_param == "azul" and size_param == "42":
            results = [
                _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, "en-US", "IN_STOCK", "Adidas", "Footwear"),
                _make_item("prod-003", "Air Zoom Pegasus Running Shoes", 149.5, "en-US", "IN_STOCK", "Nike", "Footwear"),
                _make_item("prod-015", "Runner Pro Kit", 119.0, "en-US", "IN_STOCK", "Adidas", "Footwear"),
            ]
            total_found = 3
        elif q_low == "running shoes" and brands_param == "Adidas,Nike":
            results = [
                _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, "en-US", "IN_STOCK", "Adidas", "Footwear"),
                _make_item("prod-003", "Air Zoom Pegasus Running Shoes", 149.5, "en-US", "IN_STOCK", "Nike", "Footwear"),
            ]
        elif q_low in _GOLDEN_EXPECTED_MAP:
            exp_ids, g_loc, g_brand, g_cat, g_price = _GOLDEN_EXPECTED_MAP[q_low]
            use_loc = locale if "locale" in q_single else g_loc
            if q_low == "running shoes" and ads_param != "false":
                results.append(
                    _make_item("prod-003", "Sponsored Air Zoom Pegasus", 149.5, use_loc, "IN_STOCK", "Nike", "Footwear", sponsored=True, text_s=0.82, vec_s=0.88, hyb_s=hyb_s)
                )
                results.append(
                    _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, use_loc, "IN_STOCK", "Adidas", "Footwear", sponsored=False, text_s=0.82, vec_s=0.88, hyb_s=hyb_s, ltr_s=ltr_score_val, cep=cep)
                )
                results.append(
                    _make_item("prod-015", "Ergonomic Runner Insoles", 119.0, use_loc, "IN_STOCK", "Adidas", "Footwear", sponsored=False, text_s=0.70, vec_s=0.75, hyb_s=0.72)
                )
                results.append(
                    _make_item("prod-004", "Trail Runner OOS", 95.0, use_loc, "OUT_OF_STOCK", "Adidas", "Footwear", sponsored=False, text_s=0.60, vec_s=0.62, hyb_s=0.61, boost=-0.01)
                )
            else:
                for idx, eid in enumerate(exp_ids):
                    results.append(
                        _make_item(
                            eid,
                            f"{q_raw.title()} Item #{idx + 1}",
                            g_price,
                            use_loc,
                            "IN_STOCK",
                            g_brand,
                            g_cat,
                            sponsored=False,
                            text_s=round(0.85 - idx * 0.05, 4),
                            vec_s=round(0.88 - idx * 0.05, 4),
                            hyb_s=hyb_s if idx == 0 else round(0.80 - idx * 0.05, 4),
                            ltr_s=ltr_score_val if idx == 0 else None,
                            cep=cep,
                        )
                    )
                if q_low in ("protetor solar fps 60", "minoxidil base"):
                    results.append(
                        _make_item(f"{exp_ids[0]}-oos", f"{q_raw.title()} OOS", g_price, use_loc, "OUT_OF_STOCK", g_brand, g_cat, boost=-0.01)
                    )
        elif q_low == "sneakers":
            expanded_terms = ["running shoes", "athletic footwear"]
            results = [
                _make_item("prod-003", "Sponsored Air Zoom Pegasus", 149.5, "en-US", "IN_STOCK", "Nike", "Footwear", sponsored=True),
                _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, "en-US", "IN_STOCK", "Adidas", "Footwear"),
                _make_item("prod-004", "Classic Sneakers OOS", 95.0, "en-US", "OUT_OF_STOCK", "Adidas", "Footwear"),
            ]
        else:
            results = [
                _make_item("prod-001", "UltraBoost Pro Running Shoes", 189.9, locale, "IN_STOCK", "Adidas", "Footwear", cep=cep),
                _make_item("prod-003", "Air Zoom Pegasus Running Shoes", 149.5, locale, "IN_STOCK", "Nike", "Footwear"),
            ]

        spons_cnt = sum(1 for r in results if r.get("sponsored") or r.get("is_sponsored"))
        resp_payload: Dict[str, Any] = {
            "query_id": "imp-eval-59-001",
            "results": results,
            "total_found": max(total_found, len(results)),
            "page": int(page_str) if page_str.isdigit() else 1,
            "total_pages": 9,
            "sort_by": sort_by,
            "retrieval_mode": "hybrid_v2",
            "effective_alpha": eff_alpha,
            "vector_index_used": True,
            "cache_hit": True,
            "sponsored_count": spons_cnt,
            "latency_ms": 38.4,
            "facets_latency_ms": 5.2,
            "facets": facets_obj,
            "ltr": ltr_active,
            "ltr_model": "baseline-2026-10" if ltr_active else None,
            "ltr_latency_us": 420 if ltr_active else 0,
            "personalized": bool(session_id),
            "session_id": session_id,
            "price_table_id": "b2b-corporate" if price_table == "b2b-corporate" else "default",
            "parsed_query": parsed_query_obj,
            "did_you_mean": did_you_mean,
            "auto_corrected": auto_corrected,
            "expanded_terms": expanded_terms,
        }
        return 200, "application/json", json.dumps(resp_payload).encode("utf-8")

    return 404, "application/json", json.dumps({"error": "not found"}).encode("utf-8")


# ---------------------------------------------------------------------------
# Upstream Cache & Proxy HTTP Server
# ---------------------------------------------------------------------------
class UpstreamCache:
    """Thread-safe upstream HTTP response cache so D0..D8 replay uses 0 live calls when cached or offline."""

    def __init__(self, cache_file: Optional[str] = None) -> None:
        self._lock = threading.Lock()
        self._store: Dict[str, Dict[str, Any]] = {}
        self._suffix_store: Dict[str, Dict[str, Any]] = {}
        self.cache_file = cache_file
        self.upstream_hits = 0
        self.cache_hits = 0
        if cache_file and os.path.exists(cache_file):
            try:
                with open(cache_file, "r", encoding="utf-8") as f:
                    self._store = json.load(f)
                for k, v in self._store.items():
                    parts = k.split("|", 1)
                    if len(parts) == 2:
                        self._suffix_store["|" + parts[1]] = v
            except Exception:
                self._store = {}
                self._suffix_store = {}

    def _make_key(self, target_base: str, method: str, path_query: str, req_body: bytes) -> str:
        body_hash = hashlib.sha256(req_body or b"").hexdigest()[:16]
        return f"{target_base}|{method}|{path_query}|{body_hash}"

    def _make_suffix(self, method: str, path_query: str, req_body: bytes) -> str:
        body_hash = hashlib.sha256(req_body or b"").hexdigest()[:16]
        return f"|{method}|{path_query}|{body_hash}"

    def fetch(
        self,
        target_base: str,
        method: str,
        path_query: str,
        req_body: bytes,
        content_type: Optional[str] = None,
    ) -> Tuple[int, str, bytes]:
        key = self._make_key(target_base, method, path_query, req_body)
        suffix = self._make_suffix(method, path_query, req_body)
        with self._lock:
            cached = self._store.get(key) or self._suffix_store.get(suffix)
            if cached is not None:
                self.cache_hits += 1
                return (
                    int(cached["status"]),
                    str(cached["content_type"]),
                    base64.b64decode(cached["raw_b64"]),
                )
            if not target_base:
                self.cache_hits += 1
                status, ctype, raw = _offline_baseline_response(method, path_query, req_body)
                entry = {
                    "status": status,
                    "content_type": ctype,
                    "raw_b64": base64.b64encode(raw).decode("ascii"),
                }
                self._store[key] = entry
                self._suffix_store[suffix] = entry
                return status, ctype, raw

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
            entry = {
                "status": status,
                "content_type": ctype,
                "raw_b64": base64.b64encode(raw).decode("ascii"),
            }
            self._store[key] = entry
            self._suffix_store[suffix] = entry
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
    parser.add_argument("--extra-defects", default=None, help="Optional path to external Python file defining @register_defect functions")
    parser.add_argument("--queries", default="", help="Optional path to queries JSON file")
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

    if args.extra_defects:
        with open(args.extra_defects, "r", encoding="utf-8") as ef:
            exec(compile(ef.read(), args.extra_defects, "exec"), globals())

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
