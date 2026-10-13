# `psearch` Software Factory — Verification & Review Log

## Baseline Import (`GoogleCloudPlatform/psearch` + Knowledge Plane ADRs #001–#006)
- **Date:** 2026-10-09
- **Status:** APPROVED
- **Tests:** `go test -v ./...` in `src/psearch/serving` — PASS (`TestHealthAndInfoEndpoints`, `TestHybridSearchAlphaParameterization`).
- **Backlog:** GitHub Issues `#1` through `#10` created on `duboc/scion-experiment`.

## #11: latest Gemini models via the Gen AI SDK (799e5e5)
- **Date:** 2026-10-10
- **Status:** PASS
- **Checks:**
    - **(1) Go Tests:** PASS
    - **(2) ADR-001 Compliance:** PASS (Re-verified with correct file paths. V1/V2 embedding columns, `ACTIVE_EMBEDDING_VERSION` toggle, additive migration, and lexical fallback on failure are all correctly implemented).
    - **(3) Old References:** PASS
    - **(4) Python Compilation:** PASS

### Follow-up: 6e4c0fb
- **Date:** 2026-10-12
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Search responses now include `embedding_version` and `retrieval_mode`.
    - Degraded v2 searches correctly return `lexical_only_degraded` and do not fall back to the in-memory engine.

### Follow-up: a876979
- **Date:** 2026-10-12
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Empty search results are now serialized as `[]` instead of `null`.

## #1 (alpha) & #3 (availability) (7b404be)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - ADR-001 (Alpha): PASS
    - ADR-002 (Availability): PASS
    - UI build: PASS

### Hotfix: 79b15a2
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Spanner SQL query fixed to avoid nested aggregations.
- **Process Gap:** The existing test suite only covers the in-memory search implementation, not the Spanner SQL. The new `cmd/sqlcheck` tool is intended to bridge this gap in pre-deployment validation.
