# Issue #1 — [Req 4.1.1] Busca Híbrida: parametrized `alpha` in RRF

**Status:** delivered and live · **ADR:** [ADR-001](../../knowledge-plane/adr/ADR-001-hybrid-and-vector-1024.md)

## Acceptance criteria → evidence
| Criterion | Evidence |
|---|---|
| RRF fusion weighted by `@alpha` in Spanner SQL (`0.0` = 100% text, `1.0` = 100% vector, default `0.5`) | `buildHybridSQL` in `src/psearch/serving/internal/services/spanner_service.go`: vector leg weight `@alpha`, text leg `1.0 - @alpha` |
| `alpha` exposed in `/api/search` (query param + JSON body) | `handlers.go` parses `?alpha=` and POST `{"alpha":…}`; clamped to `[0,1]` |
| `effective_alpha`, `vector_score`, `text_score` in the response | `SearchResponse.effective_alpha`; per result `score.{vector,text,hybrid,final_score,alpha}` |
| UI slider 0.0–1.0 | React `SearchInput.js` (MUI Slider, step 0.1) and storefront in `routes.go` |
| Given `q=running shoes`, alpha 0.0 vs 1.0 → rankings/`final_score` reflect pure text vs pure vector; `go test ./...` passes | `TestAlphaExtremesChangeRankingAndAreClamped` (asserts `final_score == text` at α=0, `== vector` at α=1, clamping); `go test -race ./...` green |

## Commits
- `7b404be` — `final_score`, alpha clamping, ranking tests; React slider (frontend)
- `79b15a2` — Spanner ORDER BY fix (nested aggregate) found at rollout, plus `cmd/sqlcheck` pre-deploy gate
- `23bf1f7` — deterministic tie-break (rrf → vector → text → product_id); alpha test strengthened to compare scores
- Reviews: `8894c6d`, `1189867` (REVIEW.md) · Deploys: `1baae5e` (rolled back), `31a8c9a` (live)

## Live verification (2026-10-13, `psearch-serving-00019-dsr`, `retrieval_mode=hybrid_v2`)
- `q=running shoes&alpha=0` → prod-001, prod-008, prod-015, prod-003, prod-002 (`effective_alpha=0`)
- `q=running shoes&alpha=1` → prod-002, prod-003, prod-001, prod-007, prod-015 (`effective_alpha=1`)
- `alpha=1.7` → clamped, `effective_alpha=1`
- `POST {"query":"running shoes","alpha":0.3}` → `effective_alpha=0.3`, per-result `vector`/`text` scores returned
- Storefront slider present (`id="alpha"`)
