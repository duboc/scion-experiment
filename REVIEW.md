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

### Follow-up: 780c681
- **Date:** 2026-10-13
- **Status:** PASS
- **Checks:**
    - `npm run build`: PASS
    - UI now has a smart query toggle and removable filter chips.

## Wave 5 batch: #19 UI, #21 UI, #21 fix (07c75d5, d1b602b, 8ba9d06, 4ab8403)
- **Date:** 2026-10-13
- **Status:** PASS (reviewer, private worktree; lead independently re-ran `go vet ./...` and `go test -race ./...` on `4ab8403`: ok)
- **Checks:**
    - `07c75d5` (#19 UI chips + smart-query toggle): `npm run build` PASS
    - `d1b602b` (#21 banners + explicit `parse_intent=false`): `npm run build` PASS
    - `8ba9d06` (#21 'Você quis dizer' banner): `npm run build` PASS
    - `4ab8403` (#21 entity-aware correction + handler regression test for `sansung`): `go vet` / `go test -race` PASS
- **Note:** an earlier entry for `780c681` was mislabeled as #21 UI; it is a DEPLOYMENT.md-only commit.

## #22 B2B contract pricing (372ad12)
- **Date:** 2026-10-13
- **Status:** PASS (reviewer, read-only worktree procedure)
- **Checks:** go vet PASS; go test -race PASS; ADR-008 compliance confirmed (interleaved product_prices + 019b seed, effective price via split-local lookups scoped by tenant, exclusive assortment, effective-price filters and price facet, sort_by after availability/pins/buries, LTR skipped for price sorts, 400 validation, bound params).

## #20 Session personalization + #22 UI (497fee6)
- **Date:** 2026-10-13
- **Status:** PASS (reviewer, read-only worktree procedure)
- **Checks:** go vet PASS; go test -race PASS; npm run build PASS (pre-existing eslint warnings only); ADR-007 session personalization compliant; #22 UI price-table/sort selectors and struck-through base price present.

### Follow-up: 1fad8cf (#22/#20)
- **Status:** PASS (reviewer, read-only worktree): go vet / go test -race PASS; hybrid SQL carries the optimizer-version statement hint; sqlcheck covers #22 price tables and #20 sessions.

## #20 UI (299f7ec)
- **Status:** PASS (reviewer, read-only worktree): npm run build PASS (pre-existing eslint warnings only), go build PASS; session panel, personas, click/purchase tracking and personalized badges present; storefront uses textContent.
