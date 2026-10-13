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
    - Search responses now include `embedding_version` and `retrieal_mode`.
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

## #2: Embedding Cache & Version Endpoint (23bf1f7)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-001 (Cache): PASS
    - Security: PASS (Constant-time token comparison, no token leakage)

## #4 (multilanguage) & #5 (facets) (254c8f7, 405d454)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - `npm run build`: PASS
    - ADR-003 (Facets): PASS
    - ADR-002 (Multilanguage): PASS
    - Security: PASS (Filters are bound parameters, facet expressions are constants)

### Hotfix: abf6432
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test`: PASS
    - Empty Spanner results are now authoritative and do not fall back to the in-memory engine.

## #7: Synonyms (6d62e7e)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-003 (Synonyms): PASS
    - Security: PASS (Admin endpoint secured, inputs are bound parameters)

## #6: Merchandising (0b54d52)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - `npm run build`: PASS
    - ADR-004 (Merchandising): PASS
    - Security: PASS (Admin endpoints secured, inputs are bound parameters)

### Follow-up: 2586e94, f07d668
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test` and `npm run build`: PASS
    - Rule multipliers are clamped in the backend.
    - UI uses `/api/rules` and sessionStorage for the admin token.

## #8: Delivery Promises (dea13c8)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-005 (Delivery Promises): PASS
    - Security: PASS (Bound parameters used for CEP and SLA)

### Follow-up: f00acb7
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test`: PASS
    - `shipping_cost` is now 0 when `free_shipping` is true.
    - Seed check now retries and logs non-fatally.

## #9: Autocomplete (c717604)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-006 (Autocomplete): PASS
    - #8 UI Changes: PASS (CEP input and validation, SLA toggle, and badges are all implemented)

### Follow-up: 3e9e0dd
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `npm run build`: PASS
    - UI now has debounced typeahead with keyboard navigation.

### Follow-up: f3fc754
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test`: PASS
    - Autocomplete DDL and BigQuery export now use a plain `suggestion_folded` column.

### Hotfix: 69bdc21
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test`: PASS
    - Autocomplete SQL query no longer uses `OR STARTS_WITH`.

## #10: LTR (75fee13)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-006 (LTR): PASS
- **Process Gap:** NDCG@10 comparison on the psearch-eval dashboard is not implemented.

### Follow-up: 8a7017d
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `npm run build`: PASS
    - UI now has LTR toggle and rank-change badges.

## #19: Conversational Query (b35ce14)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-007 (Conversational Query): PASS

### Follow-up: 612d939
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go test -race`: PASS
    - `parse_intent` is now on by default.

## #21: Zero-results Recovery (d674656)
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `go vet` and `go test -race`: PASS
    - ADR-008 (Zero-results Recovery): PASS
