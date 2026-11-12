#!/usr/bin/env bash
# =============================================================================
# Issue #63 — Item 8: Reproducible Proof Package & Operator Runner
# =============================================================================
# Usage:
#   1) Offline reproduction of all Issue #63 metrics & checksums from committed raw JSONL/TSV:
#      bash eval-dashboard/scripts/issue63/reproduce_issue_63.sh
#
#   2) Operator Holdout Execution (runs collection, pooling, 0-3 LLM judge, and pytrec_eval/ir_measures
#      metrics on the operator's external 40-query holdout set):
#      bash eval-dashboard/scripts/issue63/reproduce_issue_63.sh \
#        --mode holdout \
#        --queries /path/to/holdout_40.json \
#        --out-dir eval-dashboard/data/issue-63-holdout
#
#   3) Score PO Blind Sample Labels (once po_grade_0_to_3 column is filled in CSV or JSON):
#      python3 eval-dashboard/scripts/issue63/score_agreement.py \
#        --llm-key eval-dashboard/data/issue-63/llm_grades_blind_60.json \
#        --po-labels eval-dashboard/data/issue-63/po_blind_sample_v1.csv
#
#   4) Operator Hidden Defect Execution (runs rebuilt 78-scenario probe_search.py against a new defect):
#      python3 eval-dashboard/scripts/issue63/fault_proxy.py \
#        --defect D9 \
#        --extra-defects /path/to/operator_hidden_defects.py \
#        --run-probe
# =============================================================================
set -euo pipefail

MODE="offline"
QUERIES="eval-dashboard/data/issue-63/queries_dev_80.json"
OUT_DIR="eval-dashboard/data/issue-63"
DEFECT="D1"
EXTRA_DEFECTS=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      MODE="$2"
      shift 2
      ;;
    --queries)
      QUERIES="$2"
      shift 2
      ;;
    --out-dir)
      OUT_DIR="$2"
      shift 2
      ;;
    --defect)
      DEFECT="$2"
      shift 2
      ;;
    --extra-defects)
      EXTRA_DEFECTS="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

echo "============================================================================="
echo "Issue #63 Reproducible Proof Runner (mode=${MODE}, queries=${QUERIES})"
echo "============================================================================="

if [[ "${MODE}" == "holdout" ]]; then
  mkdir -p "${OUT_DIR}"
  echo "[1/3] Collecting raw HTTP responses across C1..C6 and Spanner catalog pool..."
  python3 eval-dashboard/scripts/issue63/collect_raw.py \
    --queries "${QUERIES}" \
    --out-dir "${OUT_DIR}" \
    --rps 2.0

  echo "[2/3] Grading pooled (query, product_id) pairs on 0-3 rubric via gemini-4-argon..."
  python3 eval-dashboard/scripts/issue63/judge_pool.py \
    --queries "${QUERIES}" \
    --data-dir "${OUT_DIR}" \
    --qrels-out "${OUT_DIR}/qrels_holdout.tsv"

  echo "[3/3] Computing pytrec_eval / ir_measures metrics & 10,000-resample paired bootstrap CIs..."
  python3 eval-dashboard/scripts/issue63/compute_metrics.py \
    --queries "${QUERIES}" \
    --qrels "${OUT_DIR}/qrels_holdout.tsv" \
    --runs-dir "${OUT_DIR}" \
    --out "${OUT_DIR}/metrics_report_holdout.json"
  exit 0
fi

if [[ "${MODE}" == "defect" ]]; then
  if [[ -n "${EXTRA_DEFECTS}" ]]; then
    python3 eval-dashboard/scripts/issue63/fault_proxy.py \
      --defect "${DEFECT}" \
      --extra-defects "${EXTRA_DEFECTS}" \
      --run-probe
  else
    python3 eval-dashboard/scripts/issue63/fault_proxy.py \
      --defect "${DEFECT}" \
      --run-probe
  fi
  exit 0
fi

echo ""
echo "--- [Step 1/5] Recomputing pytrec_eval + ir_measures metrics & 10,000-resample paired bootstrap CIs ---"
python3 eval-dashboard/scripts/issue63/compute_metrics.py \
  --queries "${QUERIES}" \
  --qrels "${OUT_DIR}/qrels_dev_80.tsv" \
  --runs-dir "${OUT_DIR}" \
  --out "${OUT_DIR}/metrics_report_dev_80.json"

echo ""
echo "--- [Step 2/5] Checking PO Blind Sample Agreement Scorer Status ---"
python3 eval-dashboard/scripts/issue63/score_agreement.py \
  --queries "${QUERIES}" \
  --llm-key "${OUT_DIR}/llm_grades_blind_60.json" \
  --po-labels "${OUT_DIR}/po_blind_sample_v1.csv"

echo ""
echo "--- [Step 3/5] Verifying Registered Defects & Scenario x Defect Mutation Matrix ---"
python3 eval-dashboard/scripts/issue63/fault_proxy.py --list-defects
python3 -c '
import json
with open("eval-dashboard/data/issue-63/mutation_matrix.json") as f:
    m = json.load(f)
old_s = m["before_suite_e47eb0c"]
new_s = m["after_suite_rebuilt"]
print("Old Suite (e47eb0c): %d/%d scenarios failed across D1..D8" % (old_s["scenarios_failing_at_least_one_defect"], old_s["total_scenarios"]))
print("Rebuilt Suite (#63): %d/%d scenarios failed across D1..D8" % (new_s["scenarios_failing_at_least_one_defect"], new_s["total_scenarios"]))
for d_id, row in m["before_vs_after_per_defect"].items():
    print("  %s: old_failed=%s/100 -> rebuilt_failed=%s/78 (%s)" % (d_id, row["before_failed_scenarios"], row["after_failed_scenarios"], row["description"]))
'

echo ""
echo "--- [Step 4/5] Verifying Fixed-Rate Open-Loop Latency & Stage Attribution Summary ---"
python3 -c '
import json
with open("eval-dashboard/data/issue-63/latency_load_report.json") as f:
    r = json.load(f)
print("Total HTTP requests executed: %d" % r["total_http_requests_executed"])
for rev in ["psearch-serving-00043-p87", "psearch-serving-00029-7ss"]:
    for rate in ["rps_2_0_60s", "rps_5_0_60s"]:
        blk = r["open_loop_load_tests"][rev][rate]
        srv = blk["server_latency_ms"]
        cli = blk["client_latency_ms"]
        print("  %s @ %s: ok=%d/%d | server p50=%sms p95=%sms p99=%sms | client p50=%sms p95=%sms" % (
            rev, rate, blk["http_200_count"], blk["total_requests"],
            srv["p50"], srv["p95"], srv["p99"], cli["p50"], cli["p95"]
        ))
'

echo ""
echo "--- [Step 5/5] Raw Artifact SHA-256 Checksums ---"
sha256sum "${OUT_DIR}"/*.json "${OUT_DIR}"/*.jsonl "${OUT_DIR}"/*.tsv "${OUT_DIR}"/*.csv "${OUT_DIR}"/*.txt
