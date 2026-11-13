#!/usr/bin/env python3
"""Query Cloud Monitoring in the Hub project for daily Scion/harness token and API call metrics.

Runs on your machine (not on the Hub VM). Reads HUB_PROJECT from FACTORY_ENV or
../config/factory.env unless --project is passed. Requires `gcloud auth print-access-token`.

Examples:
    python3 docs/scion/scripts/query-token-metrics.py --days 3
    python3 docs/scion/scripts/query-token-metrics.py --project my-hub-project --days 7
"""
from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
DEFAULT_ENV_PATH = os.path.normpath(os.path.join(HERE, "..", "config", "factory.env"))
METRIC_KEYWORDS = ("tokens", "api.calls", "usage", "cost", "session.count")


def load_env_file(path: str) -> dict[str, str]:
    values: dict[str, str] = {}
    if not os.path.exists(path):
        return values
    with open(path, encoding="utf-8") as fh:
        for raw in fh:
            line = raw.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, val = line.split("=", 1)
            key = key.strip()
            val = val.split("#", 1)[0].strip().strip("\"'")
            if key and val:
                values[key] = val
    return values


def resolve_hub_project(cli_project: str | None) -> str:
    if cli_project:
        return cli_project
    if os.environ.get("HUB_PROJECT"):
        return os.environ["HUB_PROJECT"]
    env_path = os.environ.get("FACTORY_ENV", DEFAULT_ENV_PATH)
    loaded = load_env_file(env_path)
    if loaded.get("HUB_PROJECT"):
        return loaded["HUB_PROJECT"]
    raise SystemExit(
        f"HUB_PROJECT is not set and {env_path} was not found or has no HUB_PROJECT. "
        "Pass --project <hub-project>, set HUB_PROJECT, or create docs/scion/config/factory.env from factory.env.example."
    )


def fetch_json(url: str, token: str) -> dict:
    req = urllib.request.Request(url, headers={"Authorization": f"Bearer {token}"})
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            return json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        return {"error": exc.code, "body": exc.read().decode("utf-8", errors="replace")[:400]}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--project", help="Hub GCP project ID (defaults to HUB_PROJECT in factory.env/env)")
    parser.add_argument("--days", type=int, default=7, help="Lookback window in UTC days (default: 7)")
    args = parser.parse_args(argv)

    project = resolve_hub_project(args.project)
    token_proc = subprocess.run(
        ["gcloud", "auth", "print-access-token"], capture_output=True, text=True, check=False
    )
    if token_proc.returncode != 0 or not token_proc.stdout.strip():
        sys.stderr.write(f"Failed to get gcloud access token: {token_proc.stderr.strip()}\n")
        return 1
    token = token_proc.stdout.strip()
    base = f"https://monitoring.googleapis.com/v3/projects/{project}"

    desc = fetch_json(
        f"{base}/metricDescriptors?"
        + urllib.parse.urlencode({"filter": 'metric.type = starts_with("workload.googleapis.com/")', "pageSize": 500}),
        token,
    )
    if "error" in desc:
        print("ERROR listing metricDescriptors:", desc)
        return 1

    types: dict[str, tuple[str | None, str | None, list[str]]] = {}
    for d in desc.get("metricDescriptors", []):
        types[d["type"]] = (d.get("metricKind"), d.get("valueType"), [l["key"] for l in d.get("labels", [])])

    end = dt.datetime.now(dt.timezone.utc).replace(microsecond=0, tzinfo=None)
    start = (end - dt.timedelta(days=args.days)).replace(hour=0, minute=0, second=0)
    interesting = [t for t in types if any(k in t for k in METRIC_KEYWORDS)]

    for mt in sorted(interesting):
        kind, vtype, _ = types[mt]
        hdr = fetch_json(
            f"{base}/timeSeries?"
            + urllib.parse.urlencode(
                {
                    "filter": f'metric.type = "{mt}"',
                    "interval.startTime": (end - dt.timedelta(days=2)).isoformat() + "Z",
                    "interval.endTime": end.isoformat() + "Z",
                    "view": "HEADERS",
                }
            ),
            token,
        )
        mkeys: set[str] = set()
        for ts in hdr.get("timeSeries", []):
            mkeys |= set(ts.get("metric", {}).get("labels", {}).keys())
        group = [f"metric.label.{k}" for k in sorted(mkeys) if any(s in k for s in ("agent", "model", "type", "harness", "project"))]
        if not group:
            group = [f"metric.label.{k}" for k in sorted(mkeys)][:4]
        aligner = "ALIGN_DELTA" if kind == "CUMULATIVE" else ("ALIGN_SUM" if kind == "DELTA" else "ALIGN_MEAN")
        params = [
            ("filter", f'metric.type = "{mt}"'),
            ("interval.startTime", start.isoformat() + "Z"),
            ("interval.endTime", end.isoformat() + "Z"),
            ("aggregation.alignmentPeriod", "86400s"),
            ("aggregation.perSeriesAligner", aligner),
            ("aggregation.crossSeriesReducer", "REDUCE_SUM"),
        ] + [("aggregation.groupByFields", g) for g in group]
        res = fetch_json(f"{base}/timeSeries?" + urllib.parse.urlencode(params), token)
        print(f"\n== {mt} ({kind}/{vtype}) grouped by {group}")
        if "error" in res:
            print("  ERROR", res)
            continue
        rows = []
        for ts in res.get("timeSeries", []):
            lab = ts.get("metric", {}).get("labels", {})
            for p in ts.get("points", []):
                v = p["value"]
                val = float(v.get("int64Value", v.get("doubleValue", 0)) or 0)
                day = p["interval"]["endTime"][:10]
                rows.append((day, json.dumps(lab, sort_keys=True), val))
        for day, lab, val in sorted(rows):
            if val:
                print(f"  {day}\t{val:,.0f}\t{lab}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
