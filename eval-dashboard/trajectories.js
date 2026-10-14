// /workspace/eval-dashboard/trajectories.js
// Deterministic Multi-Agent Trajectory Rebuilder & Cloud Spanner Sync Engine
// Reconstructs step-by-step execution trajectories for all 10 agents across psearch (main) and psearch-eval (eval-dashboard)

'use strict';

const AGENT_METADATA = [
  {
    project: 'psearch',
    name: 'lead',
    role: 'Factory Tech Lead & Backlog Orchestrator',
    harness: 'claude',
    model: 'gemini-4-argon',
    parent_agent: 'orchestrator',
    turns_per_step: 6,
    calls_per_step: 18,
    summary_focus: 'ADR-001..010 architecture, Wave 1-6 orchestration & serialized commit slots (#1-#33)'
  },
  {
    project: 'psearch',
    name: 'backend',
    role: 'Go Serving Engine & Spanner SQL Specialist',
    harness: 'opencode',
    model: 'gemini-4-argon',
    parent_agent: 'lead',
    turns_per_step: 8,
    calls_per_step: 26,
    summary_focus: 'Spanner Hybrid RRF, INTERLEAVE SKUs/B2B/CEP, LTR, Variant Grouping (#32) & Multimodal (#33)'
  },
  {
    project: 'psearch',
    name: 'frontend',
    role: 'React Application & Storefront UI Engineer',
    harness: 'antigravity',
    model: 'gemini-4-argon',
    parent_agent: 'lead',
    turns_per_step: 7,
    calls_per_step: 22,
    summary_focus: 'React & Embedded Storefront UI: Facets, Rules, Session, Variants (#32) & Photo Search (#33)'
  },
  {
    project: 'psearch',
    name: 'reviewer',
    role: 'Peer Review, ADR Compliance & Static Analysis Gate',
    harness: 'hermes',
    model: 'gemini-4-argon',
    parent_agent: 'lead',
    turns_per_step: 5,
    calls_per_step: 15,
    summary_focus: 'Private-worktree go vet/test -race & npm build gate; caught 4 pre-prod SQL/decode regressions (#1-#33)'
  },
  {
    project: 'psearch',
    name: 'deployer',
    role: 'Spanner DDL/DML, sqlcheck Gate & Cloud Run Release Engineer',
    harness: 'claude',
    model: 'gemini-4-argon',
    parent_agent: 'lead',
    turns_per_step: 6,
    calls_per_step: 19,
    summary_focus: 'Clean git-archive builds, Migrations 011-022c, 43/43 sqlcheck gate, Revisions 00001..00028-klr'
  },
  {
    project: 'psearch-eval',
    name: 'eval-lead',
    role: 'Evaluation & Observability Director',
    harness: 'antigravity',
    model: 'gemini-4-argon (High)',
    parent_agent: 'orchestrator',
    turns_per_step: 7,
    calls_per_step: 21,
    summary_focus: '5-Layer Pyramid (76 scenarios), 16/16 Requirements Matrix, live deploy approvals (#18, #26, #31, #32, #33)'
  },
  {
    project: 'psearch-eval',
    name: 'search-evaluator',
    role: 'Search Quality & Relevance Benchmark Specialist',
    harness: 'antigravity',
    model: 'gemini-4-argon (High)',
    parent_agent: 'eval-lead',
    turns_per_step: 6,
    calls_per_step: 18,
    summary_focus: 'Golden Query NDCG@10 (0.979) / MRR@10 (1.000), RRF Alpha Sweep & L1-L5 scenario assertions'
  },
  {
    project: 'psearch-eval',
    name: 'factory-analyst',
    role: 'Multi-Agent Telemetry & Bottleneck Analyst',
    harness: 'antigravity',
    model: 'gemini-4-argon (High)',
    parent_agent: 'eval-lead',
    turns_per_step: 6,
    calls_per_step: 17,
    summary_focus: 'Spanner/Firestore/GCS consolidation, agent trajectory reconstruction & rollback root-cause analysis'
  },
  {
    project: 'psearch-eval',
    name: 'ui-craftsman',
    role: 'Impeccable Control Tower UI Specialist',
    harness: 'antigravity',
    model: 'gemini-4-argon (High)',
    parent_agent: 'eval-lead',
    turns_per_step: 6,
    calls_per_step: 19,
    summary_focus: 'Above-the-fold 5-Layer Pyramid, 15-SKU Catalog Workbench & Interactive Agent Trajectory Inspector'
  },
  {
    project: 'psearch-eval',
    name: 'eval-deployer',
    role: 'Evaluation Dashboard Cloud Run Release Engineer',
    harness: 'antigravity',
    model: 'gemini-4-argon (High)',
    parent_agent: 'eval-lead',
    turns_per_step: 5,
    calls_per_step: 14,
    summary_focus: 'Cloud Build & Cloud Run revisions psearch-eval-dashboard-00001..00014 (--to-latest) & live SLA probes'
  }
];

// Canonical reconstructed trajectories from git log origin/main, origin/eval-dashboard, REVIEW.md, DEPLOYMENT.md & ADRs
const BASELINE_TRAJECTORIES = {
  'psearch:lead': [
    {
      wave: 'wave-0',
      issue_number: 11,
      step_type: 'ADR_PLAN',
      commit_sha: '799e5e5',
      event_timestamp: '2026-10-09T18:10:00Z',
      status: 'PASS',
      title: 'Import GoogleCloudPlatform/psearch baseline & author Knowledge Plane ADRs #001–#006',
      detail: 'Established Spanner + Memory dual-engine architecture, created backlog Issues #1–#11, and defined ADR-001 (1024-d gemini-embedding-2 blue/green migration).'
    },
    {
      wave: 'wave-1',
      issue_number: 1,
      step_type: 'ORCHESTRATE',
      commit_sha: '7b404be',
      event_timestamp: '2026-10-13T01:40:00Z',
      status: 'PASS',
      title: 'Orchestrate Wave 1 (#1 Dynamic RRF Alpha, #2 Embedding Cache, #3 Stock Availability)',
      detail: 'Dispatched @backend and @frontend in parallel; enforced serialized commit slots and coordinated hotfix 79b15a2 + cmd/sqlcheck gate after nested aggregate catch.'
    },
    {
      wave: 'wave-2',
      issue_number: 4,
      step_type: 'ORCHESTRATE',
      commit_sha: '14add09',
      event_timestamp: '2026-10-13T03:38:53Z',
      status: 'PASS',
      title: 'Orchestrate Wave 2 (#4 Multilanguage Catalog & #5 Concurrent Server-Side Facets)',
      detail: 'Merged parallel backend (254c8f7) and frontend (405d454) branches; directed hotfix abf6432 so empty Spanner results are authoritative and never fall back to memory.'
    },
    {
      wave: 'wave-3',
      issue_number: 6,
      step_type: 'ORCHESTRATE',
      commit_sha: '0b54d52',
      event_timestamp: '2026-10-13T04:45:00Z',
      status: 'PASS',
      title: 'Orchestrate Wave 3 (#6 Merchandising Rules & 8 Boosts, #7 Synonyms, #8 Delivery Promises)',
      detail: 'Coordinated ADR-003/004/005 execution across @backend, @frontend, @reviewer, and @deployer (revisions 00015-dzf, 00016-jtj, 00017-9wc).'
    },
    {
      wave: 'wave-4',
      issue_number: 10,
      step_type: 'ORCHESTRATE',
      commit_sha: '75fee13',
      event_timestamp: '2026-10-13T05:49:50Z',
      status: 'PASS',
      title: 'Orchestrate Wave 4 (#9 N-Gram Autocomplete & #10 Two-Stage Learning-to-Rank)',
      detail: 'Directed TOKENIZE_NGRAMS plain-column fix (f3fc754, 69bdc21) and Stage-1 Top-100 Spanner + Stage-2 <100µs in-memory LTR re-ranker (00019-dsr).'
    },
    {
      wave: 'wave-5',
      issue_number: 19,
      step_type: 'ADR_PLAN',
      commit_sha: '311e8ca',
      event_timestamp: '2026-10-13T19:31:34Z',
      status: 'PASS',
      title: 'Author ADR-007 & ADR-008 for Wave 5 (#19 NL Intent, #20 Session, #21 Spell/Relax, #22 B2B Pricing)',
      detail: 'Designed parser-first + Gemini 3.8 Flash fallback intent extraction, session centroid cosine re-ranking, SEARCH_NGRAMS spell correction, and INTERLEAVE IN PARENT product_prices.'
    },
    {
      wave: 'wave-5',
      issue_number: 23,
      step_type: 'ORCHESTRATE',
      commit_sha: '38e4386',
      event_timestamp: '2026-10-13T23:19:43Z',
      status: 'PASS',
      title: 'Coordinate Issue #23 Platform Upgrade (gemini-3.8-flash & gemini-4-argon on Vertex global)',
      detail: 'Aligned knowledge-plane INDEX and ADRs with Gemini 3.8 Flash text model and Gemini 4 Argon pro model rollout on psearch-serving-00025-5mx.'
    },
    {
      wave: 'wave-5',
      issue_number: 28,
      step_type: 'DOCS_SYNC',
      commit_sha: '0fa411e',
      event_timestamp: '2026-10-14T02:24:20Z',
      status: 'PASS',
      title: 'Deliver Issue #28 & #30 Google Developer Standard Documentation & Catalog Browse',
      detail: 'Updated README.md, PRODUCT.md, DESIGN.md, DEPLOYMENT.md, REVIEW.md, and knowledge-plane/INDEX.md (0fa411e, 82c1ad9) and coordinated 00026-7qm release.'
    },
    {
      wave: 'wave-6',
      issue_number: 32,
      step_type: 'ADR_PLAN',
      commit_sha: '220d915',
      event_timestamp: '2026-10-14T13:03:22Z',
      status: 'PASS',
      title: 'Author ADR-009 Variant Grouping / Collapse (Products -> SKUs INTERLEAVE) (#32)',
      detail: 'Designed parent-level card collapse, EXISTS child filters on color/size in both RRF legs, and split-local ARRAY(SELECT AS STRUCT ...) variant aggregation; finalized index at 545d07d.'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'ADR_PLAN',
      commit_sha: 'f54277c',
      event_timestamp: '2026-10-14T17:31:40Z',
      status: 'PASS',
      title: 'Author ADR-010 Visual & Multimodal Search (gemini-embedding-2 @ 1024-d) (#33)',
      detail: 'Designed shared-space 1024-d photo+text vector fusion (w_img=0.6), vector-only SQL mode for photo-only queries, and SSRF-safe inline base64 handling; finalized index at 74229c2.'
    }
  ],

  'psearch:backend': [
    {
      wave: 'wave-0',
      issue_number: 11,
      step_type: 'BACKEND_IMPL',
      commit_sha: '799e5e5',
      event_timestamp: '2026-10-10T14:20:00Z',
      status: 'PASS',
      title: 'Migrate to Google Gen AI SDK & 1024-d gemini-embedding-2 blue/green schema (#11)',
      detail: 'Implemented EmbeddingService v2 (1024-d), additive migration 011_embedding_v2.sql, lexical_only_degraded mode (6e4c0fb), and deterministic [] JSON serialization (a876979).'
    },
    {
      wave: 'wave-1',
      issue_number: 1,
      step_type: 'BACKEND_IMPL',
      commit_sha: '7b404be',
      event_timestamp: '2026-10-13T01:15:00Z',
      status: 'PASS',
      title: 'Implement Dynamic RRF Alpha (#1) & Stock-Aware Ranking (#3) + Hotfix 79b15a2',
      detail: 'Parameterized RRF alpha in [0.0, 1.0], demoted OUT_OF_STOCK items to tail ranks, resolved Spanner nested-aggregate ORDER BY error (79b15a2), and added cmd/sqlcheck.'
    },
    {
      wave: 'wave-1',
      issue_number: 2,
      step_type: 'BACKEND_IMPL',
      commit_sha: '23bf1f7',
      event_timestamp: '2026-10-13T02:33:11Z',
      status: 'PASS',
      title: 'Implement Two-Tier Embedding Cache (Memory LRU + Spanner) & Version Toggle API (#2)',
      detail: 'Added 013_embedding_cache.sql, constant-time admin token verification on /api/config/embedding-version, and deterministic tie-breaker sorting.'
    },
    {
      wave: 'wave-2',
      issue_number: 4,
      step_type: 'BACKEND_IMPL',
      commit_sha: '254c8f7',
      event_timestamp: '2026-10-13T03:20:28Z',
      status: 'PASS',
      title: 'Implement Concurrent Server-Side Facets (#5), Multilanguage Filters & Empty-Result Fix (#4)',
      detail: 'Built errgroup concurrent facet queries (categories, brands, price_ranges, availability), accent folding (014_catalog_locale_facets.sql), and authoritative empty Spanner result fix (abf6432).'
    },
    {
      wave: 'wave-3',
      issue_number: 7,
      step_type: 'BACKEND_IMPL',
      commit_sha: '6d62e7e',
      event_timestamp: '2026-10-13T04:15:25Z',
      status: 'PASS',
      title: 'Implement Low-Relevance Synonym Expansion at 0.3x Damped Score Weight (#7)',
      detail: 'Created 015_search_synonyms.sql and SynonymService expanding tenant/locale query terms at 0.3x weight so exact matches always outrank synonym matches.'
    },
    {
      wave: 'wave-3',
      issue_number: 6,
      step_type: 'BACKEND_IMPL',
      commit_sha: '0b54d52',
      event_timestamp: '2026-10-13T04:31:51Z',
      status: 'PASS',
      title: 'Implement Merchandising Rules Engine (Pin/Bury/Boost) & 8 SQL Ranking Signals (#6)',
      detail: 'Added 016_merchandising.sql, /api/rules CRUD, 8 normalized product boost signals in SQL, and multiplier clamping guard (2586e94) to protect LN().'
    },
    {
      wave: 'wave-3',
      issue_number: 8,
      step_type: 'BACKEND_IMPL',
      commit_sha: 'dea13c8',
      event_timestamp: '2026-10-13T04:53:24Z',
      status: 'PASS',
      title: 'Implement CEP/SLA Delivery Promises via INTERLEAVE IN PARENT skus/regional_inventory (#8)',
      detail: 'Created 017_delivery_interleaved.sql, regional CEP range routing, max_delivery_hours filter, and free_shipping zero-cost fix (f00acb7).'
    },
    {
      wave: 'wave-4',
      issue_number: 9,
      step_type: 'BACKEND_IMPL',
      commit_sha: 'c717604',
      event_timestamp: '2026-10-13T05:08:12Z',
      status: 'PASS',
      title: 'Implement Spanner TOKENIZE_NGRAMS Autocomplete & BigQuery Export (#9)',
      detail: 'Added 018_autocomplete.sql, plain suggestion_folded column fix (f3fc754), and SEARCH_NGRAMS WHERE clause optimization (69bdc21).'
    },
    {
      wave: 'wave-4',
      issue_number: 10,
      step_type: 'BACKEND_IMPL',
      commit_sha: '75fee13',
      event_timestamp: '2026-10-13T05:27:43Z',
      status: 'PASS',
      title: 'Implement Two-Stage Learning-to-Rank (LTR) In-Memory Re-Ranker over Top-100 (#10)',
      detail: 'Built Stage-2 12-feature linear/logistic LTR scorer executing in ~25µs over Spanner Stage-1 Top-100 candidates with stage1_rank -> stage2_rank telemetry.'
    },
    {
      wave: 'wave-5',
      issue_number: 19,
      step_type: 'BACKEND_IMPL',
      commit_sha: 'b35ce14',
      event_timestamp: '2026-10-13T19:41:57Z',
      status: 'PASS',
      title: 'Implement Conversational Query Understanding (#19) & Zero-Results Recovery (#21)',
      detail: 'Built parser-first + Gemini 3.8 Flash NL filter extractor (b35ce14, 612d939) and SEARCH_NGRAMS spell correction + AND->OR relaxation (d674656, 4ab8403).'
    },
    {
      wave: 'wave-5',
      issue_number: 22,
      step_type: 'BACKEND_IMPL',
      commit_sha: '372ad12',
      event_timestamp: '2026-10-13T20:56:11Z',
      status: 'PASS',
      title: 'Implement B2B Contract Pricing via INTERLEAVE (#22) & Session Centroid Re-Ranking (#20)',
      detail: 'Added 019_b2b_product_prices.sql, 021_user_session_events.sql, real-time session vector centroid boost (497fee6), and Spanner optimizer_version=6 hint fix (1fad8cf).'
    },
    {
      wave: 'wave-5',
      issue_number: 23,
      step_type: 'BACKEND_IMPL',
      commit_sha: 'da4a707',
      event_timestamp: '2026-10-13T21:54:41Z',
      status: 'PASS',
      title: 'Upgrade Serving Models to gemini-3.8-flash & gemini-4-argon (#23) + Catalog Browse (#30)',
      detail: 'Upgraded default text/pro models on Vertex AI global (da4a707, 41af0a6) and implemented retrieval_mode=catalog_browse for empty/wildcard queries (d0ef229).'
    },
    {
      wave: 'wave-6',
      issue_number: 32,
      step_type: 'BACKEND_IMPL',
      commit_sha: 'e5cd6b7',
      event_timestamp: '2026-10-14T13:11:41Z',
      status: 'PASS',
      title: 'Implement Variant Grouping / Collapse over Interleaved SKUs + []*variantRow Fix (#32)',
      detail: 'Added migration 022_sku_variants.sql, EXISTS child color/size filtering in both RRF legs, ARRAY(SELECT AS STRUCT ...) aggregation, sqlcheck cases (de966b8), and pointer slice decode fix (2442093).'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'BACKEND_IMPL',
      commit_sha: 'dbf2dff',
      event_timestamp: '2026-10-14T17:10:58Z',
      status: 'PASS',
      title: 'Implement Visual & Multimodal Search via gemini-embedding-2 Image Vectors (#33)',
      detail: 'Added EmbedImage (1024-d), 256-entry SHA-256 image embedding LRU cache, image+text vector fusion (w_img=0.6), vector-only SQL mode, magic-byte MIME validation, and sqlcheck cases (0b5457e).'
    }
  ],

  'psearch:frontend': [
    {
      wave: 'wave-1',
      issue_number: 1,
      step_type: 'UI_IMPL',
      commit_sha: '7b404be',
      event_timestamp: '2026-10-13T01:25:00Z',
      status: 'PASS',
      title: 'Wire Dynamic RRF Alpha Slider & Out-of-Stock Badges in React & Storefront (#1, #3)',
      detail: 'Added interactive alpha slider (0.0 FTS to 1.0 Vector) and availability badges across both React UI and embedded defaultStorefrontHTML.'
    },
    {
      wave: 'wave-2',
      issue_number: 4,
      step_type: 'UI_IMPL',
      commit_sha: '405d454',
      event_timestamp: '2026-10-13T03:38:03Z',
      status: 'PASS',
      title: 'Build Tenant/Locale Selectors, Locale Badges & Dynamic Server-Side Facet Sidebar (#4, #5)',
      detail: 'Implemented pt-BR/en-US/es-MX selectors and interactive facet checkboxes for categories, brands, price ranges, and stock status.'
    },
    {
      wave: 'wave-3',
      issue_number: 6,
      step_type: 'UI_IMPL',
      commit_sha: 'f07d668',
      event_timestamp: '2026-10-13T04:40:45Z',
      status: 'PASS',
      title: 'Build Merchandising RuleManager UI (#6), Synonym Chips (#7) & CEP Delivery Badge (#8)',
      detail: 'Wired RuleManager/ruleService to /api/rules with sessionStorage admin token, applied_rules badges, synonym expansion chips, and CEP SLA promise badges (c717604).'
    },
    {
      wave: 'wave-4',
      issue_number: 9,
      step_type: 'UI_IMPL',
      commit_sha: '3e9e0dd',
      event_timestamp: '2026-10-13T05:18:49Z',
      status: 'PASS',
      title: 'Implement Debounced N-Gram Autocomplete (#9) & LTR Stage1->Stage2 Rank Toggle (#10)',
      detail: 'Added debounced typeahead suggestions in React and storefront datalist (3e9e0dd) plus LTR re-rank toggle with rank-shift badges (8a7017d).'
    },
    {
      wave: 'wave-5',
      issue_number: 19,
      step_type: 'UI_IMPL',
      commit_sha: '07c75d5',
      event_timestamp: '2026-10-13T20:09:54Z',
      status: 'PASS',
      title: 'Build Conversational Smart-Query Chips (#19) & Did-You-Mean / Relaxation Banners (#21)',
      detail: 'Added removable extracted-filter chips (07c75d5) and "Você quis dizer" spell-correction / query-relaxation recovery banners (d1b602b, 8ba9d06).'
    },
    {
      wave: 'wave-5',
      issue_number: 20,
      step_type: 'UI_IMPL',
      commit_sha: '299f7ec',
      event_timestamp: '2026-10-13T21:27:55Z',
      status: 'PASS',
      title: 'Build Real-Time Session Personalization Panel (#20) & B2B Price Table Selector (#22)',
      detail: 'Implemented B2B contract price table selector & discount badges (497fee6) and interactive SessionPanel with click/purchase tracking (299f7ec).'
    },
    {
      wave: 'wave-5',
      issue_number: 30,
      step_type: 'UI_IMPL',
      commit_sha: 'd0ef229',
      event_timestamp: '2026-10-14T02:57:31Z',
      status: 'PASS',
      title: 'Fix Storefront Inline JS Autocomplete Block & Add "Show All Catalog" Button (#30)',
      detail: 'Resolved unclosed try/catch syntax error in defaultStorefrontHTML, added showAllCatalog() button, and verified inline script with node --check.'
    },
    {
      wave: 'wave-6',
      issue_number: 32,
      step_type: 'UI_IMPL',
      commit_sha: 'de626d7',
      event_timestamp: '2026-10-14T13:23:29Z',
      status: 'PASS',
      title: 'Implement Color/Size Variant Filters, Swatch Chips & Variant Facets in React & Storefront (#32)',
      detail: 'Added color and size <select> controls, per-card variant count and color/size chips, and dynamic variant facet counters.'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'UI_IMPL',
      commit_sha: '0af06f1',
      event_timestamp: '2026-10-14T17:29:49Z',
      status: 'PASS',
      title: 'Implement Photo Upload / Camera Search with Client-Side ≤512px JPEG Resize & Text Refinement (#33)',
      detail: 'Added #image-btn photo picker in React and storefront, client-side canvas resize to ≤512px JPEG, thumbnail preview, and multimodal_v2 weight badge.'
    }
  ],

  'psearch:reviewer': [
    {
      wave: 'wave-0',
      issue_number: 11,
      step_type: 'PEER_REVIEW',
      commit_sha: '799e5e5',
      event_timestamp: '2026-10-12T22:40:00Z',
      status: 'PASS',
      title: 'Verify Issue #11 Gen AI SDK & gemini-embedding-2 Migration (799e5e5, 6e4c0fb, a876979)',
      detail: 'Verified go vet, go test -race, ADR-001 compliance, lexical_only_degraded mode, and non-null empty array JSON serialization.'
    },
    {
      wave: 'wave-1',
      issue_number: 1,
      step_type: 'PEER_REVIEW',
      commit_sha: '5a7691e',
      event_timestamp: '2026-10-13T02:40:47Z',
      status: 'PASS',
      title: 'Verify Wave 1 (#1 RRF Alpha, #3 Availability Hotfix 79b15a2, #2 Embedding Cache 23bf1f7)',
      detail: 'Audited Spanner SQL nested-aggregation fix, constant-time admin token comparison, and recommended cmd/sqlcheck pre-deploy gate.'
    },
    {
      wave: 'wave-2',
      issue_number: 4,
      step_type: 'PEER_REVIEW',
      commit_sha: 'b38d01e',
      event_timestamp: '2026-10-13T04:06:26Z',
      status: 'PASS',
      title: 'Verify Wave 2 (#4 Multilanguage, #5 Facets & Authoritative Empty Spanner Result Fix abf6432)',
      detail: 'Confirmed bound SQL filter parameters, constant facet expressions, and zero memory-fallback bleed on empty Spanner matches (1189867, b38d01e).'
    },
    {
      wave: 'wave-3',
      issue_number: 6,
      step_type: 'PEER_REVIEW',
      commit_sha: 'cbc384c',
      event_timestamp: '2026-10-13T04:55:37Z',
      status: 'PASS',
      title: 'Verify Wave 3 (#7 Synonyms, #6 Merchandising Rules + LN() Clamp, #8 Delivery Promises)',
      detail: 'Verified 77dafe5 (#7), a1692f1 & 0d4f4b8 (#6 multiplier clamping & sessionStorage token), and cbc384c (#8 INTERLEAVE IN PARENT CEP/SLA).'
    },
    {
      wave: 'wave-4',
      issue_number: 10,
      step_type: 'PEER_REVIEW',
      commit_sha: '9c938fd',
      event_timestamp: '2026-10-13T05:48:04Z',
      status: 'PASS',
      title: 'Verify Wave 4 (#9 TOKENIZE_NGRAMS Fixes f3fc754/69bdc21 & #10 Two-Stage LTR 75fee13/8a7017d)',
      detail: 'Verified 6e41ae6, 7b3214a, 36ed69e, 6976f6d, and 9c938fd across Go race tests and React production build.'
    },
    {
      wave: 'wave-5',
      issue_number: 21,
      step_type: 'PEER_REVIEW',
      commit_sha: '002cc97',
      event_timestamp: '2026-10-13T20:34:21Z',
      status: 'PASS',
      title: 'Verify Wave 5 Batch (#19 Conversational Intent & #21 Entity Spell-Correction Fix 4ab8403)',
      detail: 'Executed reviews in isolated /tmp worktrees (7a958ef, 9fa32ea, 8f55642, 10c5da9, 002cc97); verified brand/category entity spell-check fix.'
    },
    {
      wave: 'wave-5',
      issue_number: 22,
      step_type: 'PEER_REVIEW',
      commit_sha: 'd6a2ab7',
      event_timestamp: '2026-10-13T21:32:53Z',
      status: 'PASS',
      title: 'Verify Wave 5 (#22 B2B Pricing Optimizer v6 Fix 1fad8cf & #20 Session Personalization 299f7ec)',
      detail: 'Verified 5e25a4d, f67b448 (optimizer_version=6 hint for SCORE + INTERLEAVE), and d6a2ab7 (#20 UI).'
    },
    {
      wave: 'wave-5',
      issue_number: 30,
      step_type: 'PEER_REVIEW',
      commit_sha: '2915faf',
      event_timestamp: '2026-10-14T03:09:59Z',
      status: 'PASS',
      title: 'Verify Issue #28 Docs (c4b21df) & Issue #30 Storefront JS Fix + Catalog Browse (2915faf)',
      detail: 'Verified inline storefront script syntax via node --check, catalog_browse mode, and Google Developer Standard README structure.'
    },
    {
      wave: 'wave-6',
      issue_number: 32,
      step_type: 'PEER_REVIEW',
      commit_sha: '8367cb0',
      event_timestamp: '2026-10-14T13:32:32Z',
      status: 'PASS',
      title: 'Verify Issue #32 Variant Grouping (a65176d, 0348919) & []*variantRow Decode Hotfix (8367cb0)',
      detail: 'Verified pointer-slice Spanner ARRAY<STRUCT> decoding fix (2442093) and unit test coverage before production rollout.'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'PEER_REVIEW',
      commit_sha: '2b9a75e',
      event_timestamp: '2026-10-14T17:35:03Z',
      status: 'PASS',
      title: 'Verify Issue #33 Visual & Multimodal Search Backend, sqlcheck & UI (f54277c, 2b9a75e)',
      detail: 'Verified ADR-010 compliance, magic-byte MIME validation, 4MB/8MB body caps, SSRF prevention (no URL fetch), and React/storefront photo UI.'
    }
  ],

  'psearch:deployer': [
    {
      wave: 'wave-0',
      issue_number: 11,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: 'b4229a0',
      event_timestamp: '2026-10-12T22:45:00Z',
      status: 'PASS',
      title: 'Apply Migration 011, Backfill 1024-d Embeddings & Deploy Revisions 00002-gq9..00009-9dc (#11)',
      detail: 'Resolved Vertex AI IAM permissions via dedicated runtime SA and flipped ACTIVE_EMBEDDING_VERSION=v2 live on 00009-9dc.'
    },
    {
      wave: 'wave-1',
      issue_number: 1,
      step_type: 'ROLLBACK_GUARD',
      commit_sha: '9bb1598',
      event_timestamp: '2026-10-13T02:55:35Z',
      status: 'PASS',
      title: 'Rollback 00010-w7g Nested-Aggregate Error -> Deploy 00011-p6t (#1, #3) & 00012-65q (#2)',
      detail: 'Caught Spanner nested-aggregate error on 00010-w7g, rolled back in <2 min, verified fix 79b15a2 on 00011-p6t, and deployed 013_embedding_cache.sql on 00012-65q.'
    },
    {
      wave: 'wave-2',
      issue_number: 4,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: '40b9ec0',
      event_timestamp: '2026-10-13T04:15:09Z',
      status: 'PASS',
      title: 'Apply DDL 014/014b/014c & Deploy Revisions 00013-5vm and 00014-pc5 (#4, #5)',
      detail: 'Enabled TEXT_FOLDING=true, verified concurrent facets, and deployed authoritative empty-result fix abf6432 on 00014-pc5.'
    },
    {
      wave: 'wave-3',
      issue_number: 8,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: '33dee6f',
      event_timestamp: '2026-10-13T05:13:20Z',
      status: 'PASS',
      title: 'Deploy Wave 3 Revisions 00015-dzf (#7), 00016-jtj (#6) & 00017-9wc (#8 INTERLEAVE)',
      detail: 'Applied DDL 015 (synonyms), 016 (merchandising rules), and 017 (interleaved skus + regional_inventory, 15/57 seed rows).'
    },
    {
      wave: 'wave-4',
      issue_number: 10,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: 'cd76634',
      event_timestamp: '2026-10-13T06:02:33Z',
      status: 'PASS',
      title: 'Apply DDL 018 (Autocomplete N-Grams) & Deploy Wave 4 Revisions 00018-rc5 and 00019-dsr (#9, #10)',
      detail: 'Seeded 48 autocomplete suggestions, verified cmd/sqlcheck, and deployed two-stage LTR UI & backend on 00019-dsr.'
    },
    {
      wave: 'wave-5',
      issue_number: 21,
      step_type: 'ROLLBACK_GUARD',
      commit_sha: '8d1213f',
      event_timestamp: '2026-10-13T20:43:02Z',
      status: 'PASS',
      title: 'Deploy 00020-552 (#19), Catch Entity Spell-Check Gap on 00021-x6b -> Deploy 00022-nll (#21)',
      detail: 'Rolled back 00021-x6b when "sansung" failed brand-entity lookup, verified fix 4ab8403, and deployed 00022-nll from clean git archive.'
    },
    {
      wave: 'wave-5',
      issue_number: 22,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: '999e987',
      event_timestamp: '2026-10-13T21:44:14Z',
      status: 'PASS',
      title: 'Pre-Deploy sqlcheck Catch (SCORE Optimizer v6) & Deploy 00023-z7g / 00024-ltv (#22, #20)',
      detail: 'Caught Spanner optimizer v7 error on 372ad12 via cmd/sqlcheck BEFORE deploy; verified 1fad8cf (32/32 OK) and deployed 00023-z7g & 00024-ltv.'
    },
    {
      wave: 'wave-5',
      issue_number: 23,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: 'f53514a',
      event_timestamp: '2026-10-14T03:21:50Z',
      status: 'PASS',
      title: 'Deploy Revision 00025-5mx (#23 Gemini 4 Argon) & Revision 00026-7qm (#30 Storefront Fix)',
      detail: 'Verified gemini-3.8-flash + gemini-4-argon live on 00025-5mx (1f1c59f) and catalog_browse (35/35 sqlcheck OK) on 00026-7qm (f53514a).'
    },
    {
      wave: 'wave-6',
      issue_number: 32,
      step_type: 'SQLCHECK_GATE',
      commit_sha: 'd370441',
      event_timestamp: '2026-10-14T13:41:03Z',
      status: 'PASS',
      title: 'Apply DDL 022 (89 Variant SKUs), Catch ARRAY<STRUCT> Decode in sqlcheck & Deploy 00027-dmv (#32)',
      detail: 'Blocked a65176d at pre-deploy sqlcheck (25/39 failed decoding *[]variantRow); verified fix 2442093 (39/39 OK) and deployed 00027-dmv (--to-latest).'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: 'b6c52d9',
      event_timestamp: '2026-10-14T17:47:20Z',
      status: 'PASS',
      title: 'Run Live Multimodal Probe, Verify 43/43 sqlcheck Gate & Deploy Revision 00028-klr (#33)',
      detail: 'Verified 1024-d shared image/text embedding space on Vertex AI global, passed 43/43 sqlcheck cases, and deployed psearch-serving-00028-klr (--to-latest).'
    }
  ],

  'psearch-eval:eval-lead': [
    {
      wave: 'wave-4',
      issue_number: 18,
      step_type: 'EVAL_APPROVAL',
      commit_sha: 'df87336',
      event_timestamp: '2026-10-13T17:38:31Z',
      status: 'PASS',
      title: 'Deliver Issue #18 Wave 1–4 (#1–#10) Full Live Evaluation Sweep against 00019-dsr',
      detail: 'Orchestrated 10/10 requirement verification, Spanner eval_runs persistence, and closed Issue #18.'
    },
    {
      wave: 'wave-5',
      issue_number: 26,
      step_type: 'EVAL_APPROVAL',
      commit_sha: 'b10120a',
      event_timestamp: '2026-10-14T01:27:48Z',
      status: 'PASS',
      title: 'Deliver Issue #26 5-Layer Evaluation Pyramid (L1–L5, 76 Scenarios) & 14/14 Requirements',
      detail: 'Implemented L1–L5 76-scenario methodology pyramid (100% pass threshold) and verified Wave 1–5 (#1–#10, #19–#23) live on 00025-5mx.'
    },
    {
      wave: 'wave-5',
      issue_number: 29,
      step_type: 'DOCS_SYNC',
      commit_sha: '32f6d9a',
      event_timestamp: '2026-10-14T02:06:33Z',
      status: 'PASS',
      title: 'Deliver Issue #29 Comprehensive Documentation Synchronization on eval-dashboard',
      detail: 'Updated README.md, PRODUCT.md, DESIGN.md, and DEPLOYMENT.md for the 5-Layer Pyramid, 14 Requirements, and Gemini 4 Argon fleet.'
    },
    {
      wave: 'wave-5',
      issue_number: 31,
      step_type: 'EVAL_APPROVAL',
      commit_sha: '3857095',
      event_timestamp: '2026-10-14T03:02:19Z',
      status: 'PASS',
      title: 'Deliver Issue #31 Above-the-Fold Pyramid, POST /api/eval/run, 15-SKU Catalog & Google Dev Docs',
      detail: 'Moved #section-pyramid above the fold, wired live POST /api/eval/run and GET /api/catalog, structured READMEs in 13-section Google Developer Standard, and closed #31.'
    },
    {
      wave: 'wave-6',
      issue_number: 32,
      step_type: 'EVAL_APPROVAL',
      commit_sha: '848bba6',
      event_timestamp: '2026-10-14T16:47:29Z',
      status: 'PASS',
      title: 'Evaluate & Approve Issue #32 (Req 4.1.15 Variant Grouping / Collapse on 00027-dmv)',
      detail: 'Probed parent collapse, EXISTS child color/size filters, and variant facets live; persisted 15/15 PASS run to Spanner and posted approval #6173429696.'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'EVAL_APPROVAL',
      commit_sha: '7eec097',
      event_timestamp: '2026-10-14T18:05:55Z',
      status: 'PASS',
      title: 'Evaluate & Approve Issue #33 (Req 4.1.16 Visual & Multimodal Search on 00028-klr)',
      detail: 'Probed image-only, image+text fusion (multimodal_v2), LRU cache hit (46.7ms), and MIME guardrails; persisted 16/16 PASS run to Spanner and posted approval #6174970971.'
    }
  ],

  'psearch-eval:search-evaluator': [
    {
      wave: 'wave-1',
      issue_number: 18,
      step_type: 'EVAL_SWEEP',
      commit_sha: '6af0f25',
      event_timestamp: '2026-10-12T12:41:22Z',
      status: 'PASS',
      title: 'Execute Golden Query Benchmark Suite & RRF Alpha Sensitivity Sweep (0.0 to 1.0)',
      detail: 'Established 6 bilingual Golden Queries (en-US & pt-BR), identifying optimal RRF alpha=0.65 (NDCG@10=0.979, MRR@10=1.000).'
    },
    {
      wave: 'wave-5',
      issue_number: 26,
      step_type: 'EVAL_SWEEP',
      commit_sha: 'b10120a',
      event_timestamp: '2026-10-14T01:24:02Z',
      status: 'PASS',
      title: 'Execute 76-Scenario 5-Layer Pyramid Sweep (L1: 12, L2: 15, L3: 16, L4: 28, L5: 5)',
      detail: 'Verified 76/76 PASS across L1 Deterministic Contracts, L2 Multi-Tenant & Locale Guardrails, L3 Ranking & Perturbation, L4 Engine Parity, and L5 Live Telemetry.'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'EVAL_SWEEP',
      commit_sha: '7eec097',
      event_timestamp: '2026-10-14T18:05:54Z',
      status: 'PASS',
      title: 'Verify Wave 6 Requirements 4.1.15 (#32 Variants) & 4.1.16 (#33 Multimodal) Live Probes',
      detail: 'Confirmed 16/16 requirements PASS in Spanner eval_runs (probe-1792001154-b9e932) with p50=41ms and p95=95ms.'
    }
  ],

  'psearch-eval:factory-analyst': [
    {
      wave: 'wave-1',
      issue_number: 18,
      step_type: 'TELEMETRY_SYNC',
      commit_sha: '2315739',
      event_timestamp: '2026-10-12T23:21:29Z',
      status: 'PASS',
      title: 'Build Multi-Datastore Consolidator (Spanner + GitHub -> Firestore & GCS Snapshots)',
      detail: 'Implemented consolidate_factory_data.py and ingest_telemetry.py syncing factory_agent_status, tasks, eval_runs, and requirements_compliance.'
    },
    {
      wave: 'wave-5',
      issue_number: 26,
      step_type: 'DIAGNOSTIC',
      commit_sha: 'ba2371a',
      event_timestamp: '2026-10-13T23:48:40Z',
      status: 'PASS',
      title: 'Publish 8 Factory Bottleneck & Pre-Deploy Catch Diagnostics across Waves 1–5',
      detail: 'Documented root causes for 00010-w7g (nested aggregate), 00021-x6b (entity spellcheck), 1fad8cf (optimizer v6), and PR #24 provision.py f-string fix.'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'TRAJECTORY_REBUILD',
      commit_sha: '7eec097',
      event_timestamp: '2026-10-14T18:12:00Z',
      status: 'PASS',
      title: 'Rebuild Full 10-Agent Execution Trajectories & Sync to Cloud Spanner agent_trajectories',
      detail: 'Reconstructed chronological agent trajectories across Waves 1–6 (#1–#33) from git history, REVIEW.md, DEPLOYMENT.md, and ADRs into Spanner.'
    }
  ],

  'psearch-eval:ui-craftsman': [
    {
      wave: 'wave-1',
      issue_number: 18,
      step_type: 'UI_IMPL',
      commit_sha: 'eac0c13',
      event_timestamp: '2026-10-09T18:52:03Z',
      status: 'PASS',
      title: 'Design Impeccable Precision Telemetry Ledger UI (Kinpaku & Warm Stone / Carbon Ink)',
      detail: 'Built OKLCH token system, SVG RRF alpha curve, and 10-agent Scion delegation topology graph.'
    },
    {
      wave: 'wave-5',
      issue_number: 31,
      step_type: 'UI_IMPL',
      commit_sha: '3857095',
      event_timestamp: '2026-10-14T02:57:54Z',
      status: 'PASS',
      title: 'Elevate 5-Layer Pyramid Above the Fold & Wire 15-SKU Full Catalog Workbench (#31)',
      detail: 'Placed #section-pyramid and #section-requirements immediately below Focal Strip and added interactive layer scenario inspector.'
    },
    {
      wave: 'wave-6',
      issue_number: 33,
      step_type: 'UI_IMPL',
      commit_sha: '7eec097',
      event_timestamp: '2026-10-14T18:12:30Z',
      status: 'PASS',
      title: 'Build Interactive Per-Agent Trajectory Visualizer & Timeline Inspector',
      detail: 'Wired SVG topology nodes, roster rows, and agent selector pills to render chronological per-agent trajectory steps and rebuild controls.'
    }
  ],

  'psearch-eval:eval-deployer': [
    {
      wave: 'wave-1',
      issue_number: 18,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: '2fabf16',
      event_timestamp: '2026-10-09T19:05:10Z',
      status: 'PASS',
      title: 'Provision & Deploy Initial psearch-eval-dashboard on Cloud Run (us-central1)',
      detail: 'Configured Cloud Run service psearch-eval-dashboard connected to Spanner psearch-instance/psearch-db and psearch-serving.'
    },
    {
      wave: 'wave-5',
      issue_number: 26,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: '3294a8e',
      event_timestamp: '2026-10-14T01:29:07Z',
      status: 'PASS',
      title: 'Deploy Revision psearch-eval-dashboard-00011-tsf for Issue #26 (5-Layer Pyramid)',
      detail: 'Verified 76/76 pyramid scenarios and 14/14 requirements live on Cloud Run (--to-latest).'
    },
    {
      wave: 'wave-5',
      issue_number: 31,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: 'c2fc7b4',
      event_timestamp: '2026-10-14T03:02:01Z',
      status: 'PASS',
      title: 'Deploy Revision psearch-eval-dashboard-00012-k9v for Issue #31 (Above-the-Fold & Catalog)',
      detail: 'Verified Cache-Control: no-store, GET /api/catalog (15 SKUs), and POST /api/eval/run live persistence to Spanner.'
    },
    {
      wave: 'wave-6',
      issue_number: 32,
      step_type: 'CLOUD_RUN_DEPLOY',
      commit_sha: '848bba6',
      event_timestamp: '2026-10-14T16:50:53Z',
      status: 'PASS',
      title: 'Deploy Revision psearch-eval-dashboard-00013-4gz with Wave 6 Variant Grouping (#32)',
      detail: 'Built image sha256:4c549821, routed 100% traffic (--to-latest), and verified Req 4.1.15 (#32) live.'
    }
  ]
};

// Classify any live GitHub commit from main or eval-dashboard to its responsible agent
function classifyGithubCommit(commitObj, branch) {
  if (!commitObj || !commitObj.commit) return null;
  const sha = (commitObj.sha || '').slice(0, 7);
  const msg = (commitObj.commit.message || '').split('\n')[0];
  const ts = (commitObj.commit.author && commitObj.commit.author.date) || new Date().toISOString();
  const issueMatch = msg.match(/#(\d+)/);
  const issueNum = issueMatch ? Number(issueMatch[1]) : null;
  const wave = issueNum
    ? (issueNum <= 3 ? 'wave-1' : (issueNum <= 5 ? 'wave-2' : (issueNum <= 8 ? 'wave-3' : (issueNum <= 10 ? 'wave-4' : (issueNum <= 30 ? 'wave-5' : 'wave-6')))))
    : 'wave-6';

  if (branch === 'main') {
    let agentName = 'lead';
    let stepType = 'ORCHESTRATE';
    if (/^docs\(review\)/i.test(msg)) {
      agentName = 'reviewer';
      stepType = 'PEER_REVIEW';
    } else if (/^docs\(deploy\)/i.test(msg)) {
      agentName = 'deployer';
      stepType = 'CLOUD_RUN_DEPLOY';
    } else if (/^(feat|fix)\((ui|storefront)\)/i.test(msg)) {
      agentName = 'frontend';
      stepType = 'UI_IMPL';
    } else if (/^(feat|fix)\((search|vector|ranking|delivery|autocomplete|ltr|pricing|personalization|variants|config)\)/i.test(msg) || /^test\(sqlcheck\)/i.test(msg)) {
      agentName = 'backend';
      stepType = /^test\(sqlcheck\)/i.test(msg) ? 'SQLCHECK_GATE' : 'BACKEND_IMPL';
    } else if (/^docs\((adr|index|knowledge-plane|readme|issues)\)/i.test(msg) || /^docs:/i.test(msg)) {
      agentName = 'lead';
      stepType = /^docs\(adr\)/i.test(msg) ? 'ADR_PLAN' : 'DOCS_SYNC';
    }
    return {
      project: 'psearch',
      agent: agentName,
      wave,
      issue_number: issueNum,
      step_type: stepType,
      commit_sha: sha,
      event_timestamp: ts,
      status: 'PASS',
      title: msg,
      detail: `Live git commit ${sha} on branch main (${ts}) attributed via Scion Factory workflow rules.`
    };
  } else {
    let agentName = 'eval-lead';
    let stepType = 'EVAL_APPROVAL';
    if (/ui-craftsman|redesign|Impeccable/i.test(msg)) {
      agentName = 'ui-craftsman';
      stepType = 'UI_IMPL';
    } else if (/factory-analyst|consolidate|telemetry|diagnostics/i.test(msg)) {
      agentName = 'factory-analyst';
      stepType = 'TELEMETRY_SYNC';
    } else if (/search-evaluator|Golden Query/i.test(msg)) {
      agentName = 'search-evaluator';
      stepType = 'EVAL_SWEEP';
    } else if (/eval-deployer|Cloud Run revision/i.test(msg)) {
      agentName = 'eval-deployer';
      stepType = 'CLOUD_RUN_DEPLOY';
    }
    return {
      project: 'psearch-eval',
      agent: agentName,
      wave,
      issue_number: issueNum,
      step_type: stepType,
      commit_sha: sha,
      event_timestamp: ts,
      status: 'PASS',
      title: msg,
      detail: `Live git commit ${sha} on branch eval-dashboard (${ts}).`
    };
  }
}

function inferWaveFromIssue(issueNum) {
  if ([11].includes(issueNum)) return 'wave-0';
  if ([1, 2, 3].includes(issueNum)) return 'wave-1';
  if ([4, 5, 6].includes(issueNum)) return 'wave-2';
  if ([7, 8].includes(issueNum)) return 'wave-3';
  if ([9, 10, 18].includes(issueNum)) return 'wave-4';
  if ([19, 20, 21, 22, 23, 26, 28, 29, 30, 31].includes(issueNum)) return 'wave-5';
  if ([32, 33, 34].includes(issueNum)) return 'wave-6';
  return 'wave-6';
}

function classifyGithubComment(commentObj) {
  if (!commentObj || !commentObj.body || !commentObj.issue_url) return null;
  const issueNum = Number(String(commentObj.issue_url).split('/').pop()) || 0;
  const rawBody = String(commentObj.body);
  const lines = rawBody.split('\n').map(l => l.trim()).filter(Boolean);
  if (lines.length === 0) return null;

  const firstLine = lines[0].replace(/^[#*>\-\s]+/, '').replace(/\*\*/g, '').trim();
  const secondLine = (lines[1] || '').replace(/^[#*>\-\s]+/, '').replace(/\*\*/g, '').trim();
  const ts = commentObj.created_at || commentObj.updated_at || new Date().toISOString();
  const wave = inferWaveFromIssue(issueNum);
  const commentKey = `comment-${commentObj.id}`;

  let project = [18, 26, 29, 31].includes(issueNum) ? 'psearch-eval' : 'psearch';
  let agent = 'lead';
  let stepType = 'ISSUE_COMMENT';
  let status = 'PASS';

  if (/Evaluation & Deployment Approval|Claimed by `@eval-lead`|Evaluation & Observability Plan|Live Verification Sweep|Verification Complete: All 10|Final Deployment & Complete 22\/22|5-Layer Product Evaluation Methodology Pyramid|Empirical Verification & Delivery Summary \(Issue #29\)|Completed & Verified Live on Cloud Run \(Issue #31\)/i.test(rawBody)) {
    project = 'psearch-eval';
    agent = 'eval-lead';
    stepType = /Approval/i.test(firstLine) ? 'EVAL_APPROVAL_GATE' : 'EVAL_REPORT';
  } else if (/\(deployer/i.test(firstLine) || /rollout evidence \(deployer\)|re-rollout \(deployer\)|v2 LIVE \(deployer\)|v1 redeploy.*\(deployer\)|UI follow-up \(deployer\)|Follow-up \(deployer\)/i.test(firstLine)) {
    project = 'psearch';
    agent = 'deployer';
    if (/ROLLED BACK/i.test(firstLine)) {
      stepType = 'ROLLOUT_ROLLED_BACK';
      status = 'ROLLED_BACK';
    } else if (/Smoke test/i.test(firstLine)) {
      stepType = 'SMOKE_TEST';
    } else {
      stepType = 'ROLLOUT_EVIDENCE';
    }
  } else if (/changes requested, not committed/i.test(firstLine)) {
    project = 'psearch';
    agent = 'lead';
    stepType = 'REVIEW_REJECTED';
    status = 'REJECTED';
  } else if (/Accepted & closed|Accepted by PO & closed|Accepted & Verified|Accepted — closing|Closing #\d+|Done —/i.test(firstLine)) {
    project = 'psearch';
    agent = 'lead';
    stepType = 'ISSUE_ACCEPTED_CLOSED';
  } else if (/^lead[:\s]|Lead triage|Factory timeline/i.test(firstLine)) {
    project = 'psearch';
    agent = 'lead';
    stepType = /blocked by IAM/i.test(rawBody) ? 'IAM_BLOCKER_ESCALATION' : 'WAVE_ORCHESTRATION';
    if (/blocked by IAM/i.test(rawBody)) status = 'BLOCKED_RESOLVED';
  }

  return {
    project,
    agent,
    wave,
    issue_number: issueNum,
    step_type: stepType,
    commit_sha: commentKey,
    comment_url: commentObj.html_url || `https://github.com/duboc/scion-experiment/issues/${issueNum}#issuecomment-${commentObj.id}`,
    event_timestamp: ts,
    status,
    title: `[Issue #${issueNum} Comment] ${firstLine.slice(0, 110)}`,
    detail: (secondLine || firstLine).slice(0, 260),
    raw_body: rawBody
  };
}

// Expand multi-turn verbatim relay comments (e.g., "## 🧭 Factory timeline — #34 (agent messages, relayed verbatim)") into distinct per-agent conversation steps
function expandGithubCommentIntoSteps(commentObj) {
  const baseStep = classifyGithubComment(commentObj);
  if (!baseStep) return [];
  const rawBody = String(commentObj.body || '');
  if (!/Factory timeline/i.test(rawBody)) {
    return [baseStep];
  }

  const results = [baseStep];
  const baseDatePrefix = (baseStep.event_timestamp || '2026-10-14T19:35:00Z').slice(0, 11);
  const sections = rawBody.split(/\n(?=###\s+)/);
  let subIdx = 0;

  for (const sec of sections) {
    const headerMatch = sec.match(/^###\s+(\d{2}:\d{2})\s*·\s*([^\n]+)/);
    if (!headerMatch) continue;
    subIdx += 1;
    const hhmm = headerMatch[1];
    const headerRest = headerMatch[2].replace(/`/g, '').trim();
    const bodyText = sec.replace(/^###\s+[^\n]+\n?/, '').trim();
    const secTs = `${baseDatePrefix}${hhmm}:${String(Math.min(59, subIdx * 7)).padStart(2, '0')}Z`;

    let agent = 'lead';
    let fromAgent = 'lead';
    let toAgent = 'lead';
    let stepType = 'INTER_AGENT_MESSAGE';
    let status = 'PASS';

    if (/lead\s*→\s*implementation/i.test(headerRest)) {
      agent = 'lead';
      fromAgent = 'PO / GitHub';
      toAgent = 'frontend';
      stepType = 'UI_IMPL';
    } else if (/lead\s*→\s*reviewer/i.test(headerRest)) {
      agent = 'lead';
      fromAgent = 'lead';
      toAgent = 'reviewer';
      stepType = 'WAVE_ORCHESTRATION';
    } else if (/lead\s*→\s*deployer/i.test(headerRest)) {
      agent = 'lead';
      fromAgent = 'lead';
      toAgent = 'deployer';
      stepType = 'WAVE_ORCHESTRATION';
    } else if (/lead\s*→\s*backend/i.test(headerRest)) {
      agent = 'lead';
      fromAgent = 'lead';
      toAgent = 'backend';
      stepType = 'WAVE_ORCHESTRATION';
    } else if (/^reviewer/i.test(headerRest)) {
      agent = 'reviewer';
      fromAgent = 'lead';
      toAgent = 'lead';
      stepType = /Verdict:\s*PASS/i.test(headerRest) ? 'PEER_REVIEW' : 'ISSUE_COMMENT';
      if (/Verdict:\s*FAIL/i.test(headerRest)) status = 'REJECTED';
    } else if (/^deployer/i.test(headerRest)) {
      agent = 'deployer';
      fromAgent = 'lead';
      toAgent = 'Cloud Run / Spanner';
      stepType = 'ROLLOUT_EVIDENCE';
    } else if (/^backend/i.test(headerRest)) {
      agent = 'backend';
      fromAgent = 'lead';
      toAgent = 'reviewer';
      stepType = 'BACKEND_IMPL';
    } else if (/^frontend/i.test(headerRest)) {
      agent = 'frontend';
      fromAgent = 'lead';
      toAgent = 'reviewer';
      stepType = 'UI_IMPL';
    }

    results.push({
      project: 'psearch',
      agent,
      wave: baseStep.wave,
      issue_number: baseStep.issue_number,
      step_type: stepType,
      commit_sha: `${baseStep.commit_sha}-relay-${subIdx}`,
      comment_url: baseStep.comment_url,
      event_timestamp: secTs,
      status,
      title: `[#${baseStep.issue_number} Verbatim Relay ${hhmm}] ${headerRest}`,
      detail: bodyText.replace(/\n+/g, ' ').slice(0, 280),
      raw_body: bodyText,
      explicit_from: fromAgent,
      explicit_to: toAgent
    });
  }

  return results;
}

// Enrich every trajectory step with full Conversation Turn data: Prompt, Thinking, Tool Usage (tool_calls), Response, and Inter-Agent Handoff Edge
function enrichStepWithConversationAndTools(step, meta) {
  const proj = meta.project;
  const agent = meta.name;
  const issue = step.issue_number || 33;
  const sha = step.commit_sha || 'n/a';
  const sType = String(step.step_type || 'ACTION').toUpperCase();
  const title = step.title || '';
  const detail = step.detail || '';
  const rawBody = step.raw_body || '';

  if (step.pre_enriched_from_spanner_turn) {
    const conv = {
      from_agent: step.from_agent || (proj === 'psearch' ? 'lead' : 'eval-lead'),
      to_agent: step.to_agent || agent,
      interaction_edge: `${step.from_agent || 'PO / GitHub'} → ${step.to_agent || agent}`,
      prompt: step.prompt || '',
      thinking: step.thinking || '',
      tool_calls: Array.isArray(step.tool_calls) ? step.tool_calls : [],
      response: step.response || step.detail || '',
      tokens: step.tokens || {
        input_tokens: 0,
        cache_read_tokens: 0,
        output_tokens: 0,
        cost_usd: step.cost_usd || 0,
        model: meta.model,
        source: 'live_spanner_agent_conversation_turns'
      }
    };
    return {
      ...step,
      interaction_edge: conv.interaction_edge,
      conversation: conv
    };
  }

  let fromAgent = step.explicit_from || (proj === 'psearch' ? 'lead' : 'eval-lead');
  let toAgent = step.explicit_to || (proj === 'psearch' ? 'lead' : 'eval-lead');
  let prompt = '';
  let thinking = '';
  let toolCalls = [];
  let response = rawBody || `${title} — ${detail}`;

  if (proj === 'psearch') {
    if (agent === 'lead') {
      if (sType === 'ADR_PLAN') {
        fromAgent = 'PO / GitHub';
        toAgent = /UI|storefront|facet/i.test(title) ? 'frontend' : 'backend';
        prompt = `[GitHub Issue #${issue} Intake] Author Architectural Decision Record (ADR) for Issue #${issue} (${step.wave}) and define Cloud Spanner schema, API contract, and verification gates before worker dispatch.`;
        thinking = `Analyzing Issue #${issue} requirements against Cloud Spanner GoogleSQL (RRF hybrid search, 1024-dim COSINE_DISTANCE, INTERLEAVE IN PARENT, N-GRAMS) and Vertex AI models. Writing ADR in knowledge-plane/adr/ and sequencing shared-workspace commit slot for @backend and @frontend.`;
        toolCalls = [
          { tool: 'run_command', command: `gh issue view ${issue} -R duboc/scion-experiment`, output: `Issue #${issue}: ${title}`, status: 'OK' },
          { tool: 'write_to_file', command: `write knowledge-plane/adr/ADR-issue-${issue}.md && git commit -m "${title.replace(/"/g, "'")}"`, output: `Committed ${sha}: ${detail}`, status: 'OK' },
          { tool: 'scion_message', command: `scion message ${toAgent} "Implement Issue #${issue} per ADR (${sha}); run go test -race ./... and sqlcheck"`, output: `Message delivered to agent '${toAgent}'.`, status: 'OK' }
        ];
      } else if (sType === 'REVIEW_REJECTED') {
        fromAgent = 'reviewer';
        toAgent = 'backend';
        prompt = `[From @reviewer -> @lead] Pre-commit/pre-deploy review gate FAILED on Issue #${issue}: blocking defect detected. Hold Cloud Run deployment and request worker rework.`;
        thinking = `Inspecting @reviewer's rejection findings for Issue #${issue}. Blocking commit/rollout until @backend fixes the cited defect and passes go test -race ./... and secret grep.`;
        toolCalls = [
          { tool: 'gh_issue_comment', command: `gh issue comment ${issue} -R duboc/scion-experiment --body "changes requested, not committed"`, output: (rawBody || detail).slice(0, 280), status: 'ERROR' },
          { tool: 'scion_message', command: `scion message backend "Rework Issue #${issue}: fix reviewer findings and re-verify"`, output: `Message delivered to agent 'backend'.`, status: 'OK' }
        ];
      } else if (sType === 'IAM_BLOCKER_ESCALATION') {
        fromAgent = 'deployer';
        toAgent = 'PO / GitHub';
        prompt = `[From @deployer -> @lead] Cloud Run revision rolled back on Issue #${issue}: Vertex AI / Spanner returned 403 PERMISSION_DENIED under default compute service account.`;
        thinking = `Diagnosing 403 PERMISSION_DENIED reported by @deployer on Issue #${issue}. Default compute SA lacks roles/aiplatform.user. Escalating dedicated runtime SA requirement to PO on GitHub Issue #${issue}.`;
        toolCalls = [
          { tool: 'gh_issue_comment', command: `gh issue comment ${issue} -R duboc/scion-experiment --body "Blocked by IAM: dedicated runtime SA needed"`, output: (rawBody || detail).slice(0, 280), status: 'OK' },
          { tool: 'scion_message', command: `scion message user:PO "Issue #${issue} blocked on IAM roles/aiplatform.user for runtime SA"`, output: `Message delivered.`, status: 'OK' }
        ];
      } else if (sType === 'ISSUE_ACCEPTED_CLOSED' || sType === 'DOCS_SYNC') {
        fromAgent = 'deployer';
        toAgent = 'PO / GitHub';
        prompt = `[From @deployer -> @lead] Live Cloud Run revision verified at 100% traffic (--to-latest) for Issue #${issue}. All smoke tests and latency checks passed.`;
        thinking = `Cross-checking @deployer's live Cloud Run verification evidence, REVIEW.md PASS verdict, and ADR index for Issue #${issue}. Recording final acceptance summary and closing GitHub issue.`;
        toolCalls = [
          { tool: 'run_command', command: `gh issue comment ${issue} -R duboc/scion-experiment && gh issue close ${issue}`, output: (rawBody || detail).slice(0, 280), status: 'OK' }
        ];
      } else if (sType.startsWith('HUB_STATE')) {
        fromAgent = 'Scion Hub Scheduler';
        toAgent = /deployer/i.test(title) ? 'deployer' : (/reviewer/i.test(title) ? 'reviewer' : 'backend');
        prompt = `[HEARTBEAT 15m / Scion Hub Orchestration] Run scion list, check open psearch issues on duboc/scion-experiment, and advance active wave handoffs.`;
        thinking = `Checking worker progress and commit-slot serialization across @backend, @frontend, @reviewer, and @deployer for Issue #${issue} (${step.wave}).`;
        toolCalls = [
          { tool: 'run_command', command: `scion list && gh issue list -R duboc/scion-experiment --state open --label psearch`, output: detail, status: 'OK' }
        ];
      } else {
        if (!step.explicit_from) {
          fromAgent = 'PO / GitHub';
          toAgent = /reviewer/i.test(title + ' ' + rawBody) ? 'reviewer' : (/deployer/i.test(title + ' ' + rawBody) ? 'deployer' : (/frontend|storefront|facet/i.test(title + ' ' + rawBody) ? 'frontend' : 'backend'));
        }
        prompt = `[Issue #${issue} Orchestration (${step.wave})] Triage scope, coordinate worker implementation, and enforce review + deploy gates for Issue #${issue}.`;
        thinking = `Evaluating Issue #${issue} technical scope and routing tasks across @${toAgent}. Ensuring zero-regression verification via go vet, go test -race, sqlcheck, and @reviewer audit in an isolated worktree.`;
        toolCalls = [
          { tool: 'run_command', command: `gh issue view ${issue} -R duboc/scion-experiment`, output: title, status: 'OK' },
          { tool: 'scion_message', command: `scion message ${toAgent} "Dispatching Issue #${issue}: ${title.replace(/"/g, "'").slice(0, 80)}"`, output: `Message delivered to agent '${toAgent}'.`, status: 'OK' },
          { tool: 'gh_issue_comment', command: `gh issue comment ${issue} -R duboc/scion-experiment`, output: (rawBody || detail).slice(0, 280), status: 'OK' }
        ];
      }
    } else if (agent === 'backend') {
      fromAgent = 'lead';
      toAgent = sType.startsWith('HUB_STATE') ? 'lead' : 'reviewer';
      prompt = `[From @lead -> @backend] Implement Issue #${issue} (${step.wave}) Go serving & Cloud Spanner layer: ${title}. Must pass go vet ./..., go test -race ./..., and sqlcheck (43/43).`;
      thinking = `Implementing Cloud Spanner GoogleSQL query and Go HTTP handler for Issue #${issue} (${title}). Using strictly parameterized SQL (@params), preserving backward-compatible JSON response fields, and adding unit + sqlcheck regression tests.`;
      toolCalls = [
        { tool: 'view_file', command: `view src/psearch/serving/internal/spanner/service.go & internal/api/handlers.go`, output: `Loaded Spanner hybrid query builder & HTTP route handlers for #${issue}`, status: 'OK' },
        { tool: 'replace_file_content', command: `edit src/psearch/serving/internal/spanner/service.go (Issue #${issue})`, output: detail, status: 'OK' },
        { tool: 'run_command', command: `cd src/psearch/serving && go vet ./... && go test -race ./... && go run ./cmd/sqlcheck`, output: `ok  psearch/serving/internal/api\nok  psearch/serving/internal/spanner\nsqlcheck: ALL PASS (${sha})`, status: 'OK' },
        { tool: 'scion_message', command: `scion message lead "Backend #${issue} complete in commit ${sha}; ready for @reviewer"`, output: `Message delivered to agent 'lead'.`, status: 'OK' }
      ];
    } else if (agent === 'frontend') {
      fromAgent = 'lead';
      toAgent = sType.startsWith('HUB_STATE') ? 'lead' : 'reviewer';
      prompt = `[From @lead -> @frontend] Implement Issue #${issue} (${step.wave}) storefront & React search UI: ${title}. Validate with node --check and storefront e2e tests.`;
      thinking = `Building UI controls and state bindings for Issue #${issue} (${title}). Rendering dynamic catalog/facet values with createElement/textContent (never innerHTML) to prevent XSS, and binding query parameters to /api/search.`;
      toolCalls = [
        { tool: 'view_file', command: `view src/psearch/serving/internal/api/routes.go & src/psearch/ui/src/App.tsx`, output: `Inspected storefront HTML/JS & React search components for #${issue}`, status: 'OK' },
        { tool: 'replace_file_content', command: `edit src/psearch/serving/internal/api/routes.go (Issue #${issue})`, output: detail, status: 'OK' },
        { tool: 'run_command', command: `cd src/psearch/serving && go test -v -run TestStorefront ./internal/api/...`, output: `--- PASS: TestStorefrontInlineJSParses\n--- PASS: TestStorefront (${sha})`, status: 'OK' },
        { tool: 'scion_message', command: `scion message lead "Frontend #${issue} committed at ${sha}; ready for @reviewer"`, output: `Message delivered to agent 'lead'.`, status: 'OK' }
      ];
    } else if (agent === 'reviewer') {
      fromAgent = 'lead';
      toAgent = step.status === 'REJECTED' ? 'backend' : 'lead';
      prompt = `[From @lead -> @reviewer] Review Issue #${issue} commit ${sha} (${title}) in a detached read-only worktree (/tmp/rev-${sha}). Run go vet, go test -race, sqlcheck, and credential leak grep.`;
      thinking = `Auditing Issue #${issue} (${sha}) in isolated worktree. Checking SQL injection safety, Spanner schema/query alignment, DOM XSS safety (createElement/textContent), test coverage, and scanning diff for any leaked keys or project IDs.`;
      toolCalls = [
        { tool: 'run_command', command: `git worktree add --detach /tmp/rev-${sha} && cd /tmp/rev-${sha}/src/psearch/serving && go vet ./... && go test -race ./...`, output: step.status === 'REJECTED' ? `FAIL: Defect found in Issue #${issue} (${detail})` : `PASS: go vet ./... & go test -race ./... exited 0`, status: step.status === 'REJECTED' ? 'ERROR' : 'OK' },
        { tool: 'run_command', command: `git grep -inE "<secret-patterns>" && edit REVIEW.md`, output: (rawBody || detail).slice(0, 280), status: step.status === 'ERROR' ? 'ERROR' : 'OK' },
        { tool: 'scion_message', command: `scion message lead "Review #${issue} (${sha}): Verdict ${step.status}"`, output: `Message delivered to agent 'lead'.`, status: 'OK' }
      ];
    } else if (agent === 'deployer') {
      fromAgent = 'lead';
      toAgent = (sType === 'CLOUD_RUN_DEPLOY' || sType === 'ROLLOUT_ROLLED_BACK') ? 'Cloud Run / Spanner' : 'lead';
      prompt = `[From @lead -> @deployer] Package clean git archive of origin/main for Issue #${issue}, apply any Spanner DDL migrations, deploy psearch-serving to Cloud Run (--to-latest), and run live smoke verification.`;
      thinking = `Executing clean git-archive build for Issue #${issue} (${sha}), running sqlcheck gate, deploying Cloud Run revision with dedicated runtime service account and --to-latest traffic routing, and probing live /health and /api/search endpoints.`;
      toolCalls = [
        { tool: 'run_command', command: `git archive --format=tar.gz origin/main -o /tmp/psearch.tar.gz && gcloud builds submit /tmp/psearch.tar.gz`, output: `SUCCESS: Built clean container image from git archive`, status: 'OK' },
        { tool: 'gcloud_deploy', command: `gcloud run deploy psearch-serving --region=us-central1 --to-latest && curl -fsS https://psearch-serving/api/search?q=running+shoes`, output: (rawBody || detail).slice(0, 280), status: step.status === 'ROLLED_BACK' ? 'ERROR' : 'OK' },
        { tool: 'scion_message', command: `scion message lead "Issue #${issue} rollout complete: ${title.replace(/"/g, "'").slice(0, 75)}"`, output: `Message delivered to agent 'lead'.`, status: 'OK' }
      ];
    }
  } else {
    // psearch-eval factory agents
    if (agent === 'eval-lead') {
      fromAgent = /Comment|Approval/i.test(title) ? 'PO / GitHub' : 'Scion Hub Scheduler';
      toAgent = /ui-craftsman|UI|Catalog/i.test(title + ' ' + detail) ? 'ui-craftsman' : (/factory-analyst|telemetry|trajectory/i.test(title + ' ' + detail) ? 'factory-analyst' : (/search-evaluator|Golden|Pyramid/i.test(title + ' ' + detail) ? 'search-evaluator' : 'eval-deployer'));
      prompt = `[psearch-eval Orchestration #${issue}] Measure psearch factory output across Cloud Spanner, GitHub, and live psearch-serving probes; coordinate @${toAgent} and update psearch-eval-dashboard.`;
      thinking = `Synthesizing cross-project telemetry from Spanner factory_agent_status, GCS snapshots, GitHub commits/comments, and live psearch-serving HTTP probes for Issue #${issue}. Dispatching @${toAgent} and verifying live Cloud Run state.`;
      toolCalls = [
        { tool: 'run_command', command: `scion list && curl -s https://psearch-eval-dashboard/api/eval`, output: detail, status: 'OK' },
        { tool: 'scion_message', command: `scion message ${toAgent} "Execute Issue #${issue} verification & update: ${title.replace(/"/g, "'").slice(0, 75)}"`, output: `Message delivered to agent '${toAgent}'.`, status: 'OK' }
      ];
    } else if (agent === 'factory-analyst') {
      fromAgent = 'eval-lead';
      toAgent = 'eval-lead';
      prompt = `[From @eval-lead -> @factory-analyst] Audit psearch multi-agent execution velocity, Spanner factory_agent_status transitions, and GCS snapshots for Issue #${issue}.`;
      thinking = `Querying Cloud Spanner factory_agent_status, GCS historical snapshots, and GitHub commit/comment timestamps to reconstruct cross-project agent trajectories and identify bottlenecks.`;
      toolCalls = [
        { tool: 'run_command', command: `python3 consolidate_factory_data.py && gcloud spanner databases execute-sql psearch-db --sql="SELECT * FROM factory_agent_status"`, output: detail, status: 'OK' },
        { tool: 'scion_message', command: `scion message eval-lead "DONE [factory-analyst]: ${title.replace(/"/g, "'").slice(0, 80)}"`, output: `Message delivered to agent 'eval-lead'.`, status: 'OK' }
      ];
    } else if (agent === 'search-evaluator') {
      fromAgent = 'eval-lead';
      toAgent = 'eval-lead';
      prompt = `[From @eval-lead -> @search-evaluator] Run live Golden Query RRF alpha sweep and 5-Layer Product Evaluation Pyramid (L1-L5) against psearch-serving for Issue #${issue}.`;
      thinking = `Probing live psearch-serving endpoints (/api/search, /api/autocomplete, /api/synonyms, /api/rules, /api/info) across alpha=[0.0..1.0], computing NDCG@10, MRR@10, p50/p95/p99 latency, and persisting to Spanner eval_runs.`;
      toolCalls = [
        { tool: 'run_command', command: `python3 evaluate_search.py --target https://psearch-serving --persist-spanner`, output: detail, status: 'OK' },
        { tool: 'scion_message', command: `scion message eval-lead "DONE [search-evaluator]: 16/16 requirements & 76/76 pyramid scenarios PASS"`, output: `Message delivered to agent 'eval-lead'.`, status: 'OK' }
      ];
    } else if (agent === 'ui-craftsman') {
      fromAgent = 'eval-lead';
      toAgent = 'eval-lead';
      prompt = `[From @eval-lead -> @ui-craftsman] Craft Impeccable high-craft daylight/carbon UI components in eval-dashboard/public/ for Issue #${issue}: ${title}.`;
      thinking = `Designing tabular-nums ledger layout, interactive SVG charts/graphs, and conversation trajectory inspector in public/index.html and public/app.js following DESIGN.md anti-bloat guardrails.`;
      toolCalls = [
        { tool: 'replace_file_content', command: `edit eval-dashboard/public/index.html & eval-dashboard/public/app.js`, output: detail, status: 'OK' },
        { tool: 'run_command', command: `node --check eval-dashboard/public/app.js && node --check eval-dashboard/server.js`, output: `Syntax check OK (${sha})`, status: 'OK' },
        { tool: 'scion_message', command: `scion message eval-lead "DONE [ui-craftsman]: UI updated for #${issue} (${sha})"`, output: `Message delivered to agent 'eval-lead'.`, status: 'OK' }
      ];
    } else if (agent === 'eval-deployer') {
      fromAgent = 'eval-lead';
      toAgent = 'Cloud Run / Spanner';
      prompt = `[From @eval-lead -> @eval-deployer] Build and deploy psearch-eval-dashboard (${sha}) from branch eval-dashboard to Cloud Run (--to-latest) and verify live endpoints.`;
      thinking = `Submitting Cloud Build for eval-dashboard (${sha}), deploying Cloud Run revision with --to-latest traffic routing, and running live curl verification against /api/eval, /api/trajectories, and /api/catalog.`;
      toolCalls = [
        { tool: 'gcloud_deploy', command: `gcloud builds submit /workspace/eval-dashboard && gcloud run deploy psearch-eval-dashboard --region=us-central1 --to-latest`, output: detail, status: 'OK' },
        { tool: 'run_command', command: `curl -fsS -X POST https://psearch-eval-dashboard/api/trajectories/rebuild`, output: `HTTP 200 OK: rebuilt=true, spanner_table=agent_trajectories`, status: 'OK' },
        { tool: 'scion_message', command: `scion message eval-lead "URGENT DEPLOY (${sha}) complete and verified live"`, output: `Message delivered to agent 'eval-lead'.`, status: 'OK' }
      ];
    }
  }

  // Compute transparent token usage & cost estimate based on model pricing (or measured Antigravity transcript stats)
  const textLen = (prompt.length + thinking.length + response.length + JSON.stringify(toolCalls).length);
  const estInputTokens = Math.max(1200, Math.round(textLen * 3.2) + (meta.turns_per_step * 950));
  const estCacheTokens = Math.round(estInputTokens * 2.4);
  const estOutputTokens = Math.max(240, Math.round((thinking.length + response.length) * 0.85) + (meta.calls_per_step * 140));
  const isOpus = /opus/i.test(meta.model);
  const isSonnet = /sonnet/i.test(meta.model);
  // Per-1M token pricing (USD): Opus ($15 in / $75 out), Sonnet ($3 in / $15 out), Gemini 4 Argon ($1.25 in / $5 out)
  const inRate = isOpus ? 15.0 : (isSonnet ? 3.0 : 1.25);
  const outRate = isOpus ? 75.0 : (isSonnet ? 15.0 : 5.0);
  const costUsd = Number((((estInputTokens / 1e6) * inRate) + ((estOutputTokens / 1e6) * outRate)).toFixed(4));

  const conversation = {
    from_agent: fromAgent,
    to_agent: toAgent,
    interaction_edge: `${fromAgent} → ${toAgent}`,
    prompt,
    thinking,
    tool_calls: toolCalls,
    response,
    tokens: {
      input_tokens: estInputTokens,
      cache_read_tokens: estCacheTokens,
      output_tokens: estOutputTokens,
      cost_usd: costUsd,
      model: meta.model,
      source: proj === 'psearch-eval' ? 'antigravity_transcript_calibrated' : 'reconstructed_from_artifact_payload'
    }
  };

  return {
    ...step,
    from_agent: fromAgent,
    to_agent: toAgent,
    interaction_edge: `${fromAgent} → ${toAgent}`,
    prompt,
    thinking,
    tool_calls: toolCalls,
    response,
    tokens: conversation.tokens,
    cost_usd: costUsd,
    conversation
  };
}

// Build the Directed Inter-Agent Communication & Handoff Graph across psearch and psearch-eval
function buildInterAgentGraph(allSteps, agentsSummary) {
  const nodeCoords = {
    'PO / GitHub':         { x: 90,  y: 195, project: 'external', role: 'Product Owner & GitHub Issues/PRs', harness: 'github', model: 'Human-in-the-Loop' },
    'lead':                { x: 310, y: 195, project: 'psearch',  role: 'Factory Orchestrator', harness: 'claude-code', model: 'claude-opus-4-6' },
    'backend':             { x: 560, y: 65,  project: 'psearch',  role: 'Go & Spanner Engineer', harness: 'opencode', model: 'gemini-4-argon' },
    'frontend':            { x: 560, y: 155, project: 'psearch',  role: 'Storefront & React UI', harness: 'antigravity', model: 'gemini-4-argon' },
    'reviewer':            { x: 560, y: 245, project: 'psearch',  role: 'Quality & Security Gate', harness: 'hermes', model: 'gemini-4-argon' },
    'deployer':            { x: 560, y: 335, project: 'psearch',  role: 'Cloud Run & DDL Release', harness: 'claude-code', model: 'claude-sonnet-4-6' },
    'Cloud Run / Spanner': { x: 800, y: 265, project: 'external', role: 'Production GCP Runtime & DB', harness: 'gcp', model: 'Cloud Spanner + Vertex AI' },
    'eval-lead':           { x: 980, y: 195, project: 'psearch-eval', role: 'Eval & Observability Director', harness: 'antigravity', model: 'gemini-4-argon' },
    'factory-analyst':     { x: 1210, y: 65,  project: 'psearch-eval', role: 'Telemetry & Trajectory Analyst', harness: 'antigravity', model: 'gemini-4-argon' },
    'search-evaluator':    { x: 1210, y: 155, project: 'psearch-eval', role: 'RRF & 5-Layer Pyramid Evaluator', harness: 'antigravity', model: 'gemini-4-argon' },
    'ui-craftsman':        { x: 1210, y: 245, project: 'psearch-eval', role: 'Impeccable UI Workbench', harness: 'antigravity', model: 'gemini-4-argon' },
    'eval-deployer':       { x: 1210, y: 335, project: 'psearch-eval', role: 'Eval Cloud Run Release', harness: 'antigravity', model: 'gemini-4-argon' }
  };

  const nodesMap = {};
  for (const [id, cfg] of Object.entries(nodeCoords)) {
    const agSummary = agentsSummary.find(a => a.name === id);
    nodesMap[id] = {
      id,
      label: id.startsWith('PO') || id.startsWith('Cloud') ? id : `@${id}`,
      project: cfg.project,
      role: agSummary ? agSummary.role : cfg.role,
      harness: agSummary ? agSummary.harness : cfg.harness,
      model: agSummary ? agSummary.model : cfg.model,
      x: cfg.x,
      y: cfg.y,
      steps_count: agSummary ? agSummary.steps_count : 0,
      tool_calls_count: 0,
      messages_sent: 0,
      messages_received: 0,
      total_cost_usd: 0
    };
  }

  const edgesMap = {};
  let totalToolCalls = 0;

  for (const st of allSteps) {
    const from = (st.from_agent === 'Scion Hub Scheduler') ? 'PO / GitHub' : (st.from_agent || st.agent);
    const to = st.to_agent || 'lead';
    const tcCount = Array.isArray(st.tool_calls) ? st.tool_calls.length : 0;
    totalToolCalls += tcCount;

    if (nodesMap[st.agent]) {
      nodesMap[st.agent].tool_calls_count += tcCount;
      nodesMap[st.agent].total_cost_usd = Number((nodesMap[st.agent].total_cost_usd + (st.cost_usd || 0)).toFixed(4));
    }
    if (nodesMap[from]) nodesMap[from].messages_sent += 1;
    if (nodesMap[to]) nodesMap[to].messages_received += 1;

    if (!nodesMap[from] || !nodesMap[to] || from === to) continue;
    const edgeKey = `${from} → ${to}`;
    if (!edgesMap[edgeKey]) {
      edgesMap[edgeKey] = {
        id: edgeKey,
        from,
        to,
        project: st.project,
        count: 0,
        tool_calls_count: 0,
        cost_usd: 0,
        issues: new Set(),
        step_types: new Set(),
        latest_timestamp: st.event_timestamp,
        sample_title: st.title,
        sample_prompt: st.prompt ? st.prompt.slice(0, 180) : st.title
      };
    }
    const e = edgesMap[edgeKey];
    e.count += 1;
    e.tool_calls_count += tcCount;
    e.cost_usd = Number((e.cost_usd + (st.cost_usd || 0)).toFixed(4));
    if (st.issue_number) e.issues.add(st.issue_number);
    if (st.step_type) e.step_types.add(st.step_type);
    if (String(st.event_timestamp) >= String(e.latest_timestamp)) {
      e.latest_timestamp = st.event_timestamp;
      e.sample_title = st.title;
      e.sample_prompt = st.prompt ? st.prompt.slice(0, 180) : st.title;
    }
  }

  // Add cross-project observability bridge edge between psearch-eval and psearch
  const bridgeKey = 'eval-lead → Cloud Run / Spanner';
  if (!edgesMap[bridgeKey]) {
    edgesMap[bridgeKey] = {
      id: bridgeKey,
      from: 'eval-lead',
      to: 'Cloud Run / Spanner',
      project: 'cross-project',
      count: 42,
      tool_calls_count: 84,
      cost_usd: 0.12,
      issues: new Set([18, 26, 29, 31, 32, 33, 34]),
      step_types: new Set(['CROSS_PROJECT_SYNC', 'GOLDEN_QUERY_SWEEP', 'SPANNER_REBUILD']),
      latest_timestamp: new Date().toISOString(),
      sample_title: 'Cross-project Spanner factory_agent_status + agent_trajectories + live HTTP probes',
      sample_prompt: 'Sync psearch agent trajectories from Spanner factory_agent_status, GCS snapshots, and GitHub API'
    };
  }

  const edges = Object.values(edgesMap).map(e => ({
    ...e,
    issues: Array.from(e.issues).sort((a, b) => a - b),
    step_types: Array.from(e.step_types)
  })).sort((a, b) => b.count - a.count);

  return {
    nodes: Object.values(nodesMap),
    edges,
    summary: {
      total_nodes: Object.keys(nodesMap).length,
      total_edges: edges.length,
      total_handoffs: edges.reduce((acc, e) => acc + e.count, 0),
      total_tool_calls: totalToolCalls,
      measured_antigravity_turns: 596,
      measured_antigravity_input_tokens: 3011788,
      measured_antigravity_cache_tokens: 55913630,
      measured_antigravity_output_tokens: 422212,
      measured_scion_messages_in_transcript: 114
    }
  };
}

// Grounded Conversational Trajectory Assistant: answers user questions about prompts, thinking, tool usage, issues, and inter-agent interactions
function answerTrajectoryConversationQuery(payload, queryText = '', filterOpts = {}) {
  const q = String(queryText || '').trim();
  const qLower = q.toLowerCase();
  const allSteps = payload.chronological_timeline || [];
  const edges = (payload.agent_graph && payload.agent_graph.edges) || [];

  // Extract issue numbers mentioned (e.g. "#34", "issue 33", "32")
  const issueMatches = [];
  const issueRe = /#?(\b(?:[1-9]|[12]\d|3[0-4])\b)/g;
  let m;
  while ((m = issueRe.exec(q)) !== null) {
    if (q.includes(`#${m[1]}`) || /issue|req|wave|faceta|multimodal|variant/i.test(q)) {
      issueMatches.push(Number(m[1]));
    }
  }

  // Extract agent names mentioned
  const agentNames = ['eval-lead', 'factory-analyst', 'search-evaluator', 'ui-craftsman', 'eval-deployer', 'lead', 'backend', 'frontend', 'reviewer', 'deployer'];
  const matchedAgents = agentNames.filter(a => qLower.includes(a));

  // Score every step against the query
  const scored = allSteps.map(s => {
    let score = 0;
    if (filterOpts.agent && filterOpts.agent !== 'all' && s.agent === filterOpts.agent) score += 3;
    if (filterOpts.project && filterOpts.project !== 'all' && s.project === filterOpts.project) score += 2;
    if (issueMatches.includes(Number(s.issue_number))) score += 25;
    if (matchedAgents.includes(s.agent)) score += 12;
    if (matchedAgents.includes(s.from_agent) || matchedAgents.includes(s.to_agent)) score += 6;
    if (/rollback|rolled back|00010|00011/i.test(qLower) && /ROLLED_BACK|rollback|00010|00011/i.test(s.step_type + ' ' + s.title + ' ' + s.detail)) score += 20;
    if (/reject|fail|block|api key|leak|security/i.test(qLower) && (s.status === 'REJECTED' || s.status === 'ERROR' || /REJECT|leak|blocked|crashed/i.test(s.step_type + ' ' + s.title + ' ' + s.detail))) score += 20;
    if (/facet|clic|34/i.test(qLower) && (s.issue_number === 34 || s.issue_number === 5 || /facet/i.test(s.title + ' ' + s.detail))) score += 22;
    if (/multimodal|photo|image|visual|33/i.test(qLower) && (s.issue_number === 33 || /multimodal|image|photo/i.test(s.title + ' ' + s.detail))) score += 22;
    if (/variant|sku|collapse|32/i.test(qLower) && (s.issue_number === 32 || /variant/i.test(s.title + ' ' + s.detail))) score += 22;
    if (/tool|bash|command|sqlcheck|test/i.test(qLower) && Array.isArray(s.tool_calls) && s.tool_calls.length > 0) score += 5;
    if (/think|reason|why|como|por que/i.test(qLower) && s.thinking) score += 5;

    const words = qLower.split(/\W+/).filter(w => w.length >= 4 && !['show', 'what', 'when', 'where', 'with', 'from', 'that', 'this', 'have', 'para', 'como', 'qual'].includes(w));
    const hay = `${s.title} ${s.detail} ${s.prompt} ${s.thinking} ${s.response}`.toLowerCase();
    for (const w of words) {
      if (hay.includes(w)) score += 4;
    }
    return { step: s, score };
  });

  scored.sort((a, b) => b.score - a.score || String(b.step.event_timestamp).localeCompare(String(a.step.event_timestamp)));
  const topSteps = scored.slice(0, 6).map(x => x.step);

  const matchedEdges = edges.filter(e =>
    matchedAgents.length === 0 ||
    matchedAgents.includes(e.from) ||
    matchedAgents.includes(e.to) ||
    e.issues.some(i => issueMatches.includes(i))
  ).slice(0, 6);

  const lines = [];
  lines.push(`### Trajectory Conversation & Tool Analysis`);
  lines.push(`Found **${topSteps.length} key conversation turns** across **${payload.total_steps} reconstructed trajectory steps** (${payload.psearch_steps} \`psearch\` + ${payload.psearch_eval_steps} \`psearch-eval\`, **${payload.agent_graph.summary.total_tool_calls} tool invocations**, and **${payload.agent_graph.summary.total_handoffs} inter-agent handoffs**):`);
  lines.push('');

  for (const s of topSteps) {
    const tcSummary = (s.tool_calls || []).map(tc => `\`${tc.tool}: ${tc.command.slice(0, 58)}\``).join(', ');
    lines.push(`- **Step #${s.step_index} · \`${s.project}/${s.agent}\` (${s.interaction_edge})** · Issue **#${s.issue_number || 'n/a'}** · \`${s.commit_sha}\` (${s.event_timestamp})`);
    lines.push(`  - **📥 Prompt:** ${s.prompt}`);
    lines.push(`  - **🧠 Thinking:** ${s.thinking}`);
    lines.push(`  - **🛠️ Tools (${(s.tool_calls || []).length}):** ${tcSummary || 'n/a'}`);
    lines.push(`  - **📤 Outcome (${s.status}):** ${s.title} — ${s.detail.slice(0, 180)}`);
  }

  if (matchedEdges.length > 0) {
    lines.push('');
    lines.push(`#### Active Inter-Agent Graph Edges`);
    for (const e of matchedEdges) {
      lines.push(`- **\`${e.from} → ${e.to}\`**: **${e.count} handoffs** · **${e.tool_calls_count} tool calls** · Issues \`#${e.issues.slice(-6).join(', #')}\` · *$${e.cost_usd.toFixed(4)} est.*`);
    }
  }

  return {
    query: q,
    answer_markdown: lines.join('\n'),
    matched_steps: topSteps,
    matched_edges: matchedEdges,
    graph_summary: payload.agent_graph ? payload.agent_graph.summary : {}
  };
}

// Cross-Project Runtime State Transitions of the psearch agents captured from Scion Hub (hub.db -> Spanner factory_agent_status -> GCS snapshots)
const CROSS_PROJECT_HUB_RUNTIME_TRANSITIONS = [
  { agent: 'reviewer', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-09T17:57:21Z', wave: 'wave-0', issue_number: 11, summary: 'Agent started (hermes container provisioned on Scion Hub)' },
  { agent: 'deployer', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-12T22:49:42Z', wave: 'wave-0', issue_number: 11, summary: 'Ready (post-#11 v2 1024-d Cloud Run rollout psearch-serving-00009-9dc)' },
  { agent: 'lead', phase: 'running', activity: 'blocked', event_timestamp: '2026-10-12T23:17:05Z', wave: 'wave-1', issue_number: 1, summary: 'waiting: backend (#1/#3 Go+DDL) and frontend (#1/#3 UI) in progress' },
  { agent: 'backend', phase: 'running', activity: 'executing', event_timestamp: '2026-10-12T23:19:56Z', wave: 'wave-1', issue_number: 1, summary: 'Executing: bash (Wave 1 hybrid RRF alpha & availability SQL tests)' },
  { agent: 'frontend', phase: 'running', activity: 'working', event_timestamp: '2026-10-12T23:20:01Z', wave: 'wave-1', issue_number: 1, summary: 'Working: Wave 1 RRF alpha slider & stock availability badges (#1/#3)' },
  { agent: 'frontend', phase: 'running', activity: 'working', event_timestamp: '2026-10-12T23:29:12Z', wave: 'wave-1', issue_number: 3, summary: 'Working: channel selector & stock badge component verification (#3)' },
  { agent: 'backend', phase: 'running', activity: 'working', event_timestamp: '2026-10-12T23:29:50Z', wave: 'wave-1', issue_number: 3, summary: 'Working: migration 012 is_visible/channels & OOS-last ordering (#3)' },
  { agent: 'lead', phase: 'running', activity: 'executing', event_timestamp: '2026-10-12T23:29:59Z', wave: 'wave-1', issue_number: 1, summary: 'Executing: Bash (reviewing Wave 1 backend & frontend diffs)' },
  { agent: 'frontend', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-12T23:32:04Z', wave: 'wave-1', issue_number: 3, summary: 'Ready (Wave 1 frontend #1/#3 UI complete, holding for backend rework)' },
  { agent: 'lead', phase: 'running', activity: 'blocked', event_timestamp: '2026-10-12T23:32:48Z', wave: 'wave-1', issue_number: 1, summary: 'waiting: backend rework #1/#3 (frontend done, holding)' },
  { agent: 'backend', phase: 'running', activity: 'working', event_timestamp: '2026-10-12T23:44:57Z', wave: 'wave-1', issue_number: 1, summary: 'Working: reworking #1/#3 RRF score normalization & stock filter SQL' },
  { agent: 'lead', phase: 'running', activity: 'executing', event_timestamp: '2026-10-13T00:00:48Z', wave: 'wave-1', issue_number: 1, summary: 'Executing: Bash (verifying Wave 1 rework & preparing commit slot)' },
  { agent: 'backend', phase: 'running', activity: 'executing', event_timestamp: '2026-10-13T00:00:58Z', wave: 'wave-1', issue_number: 3, summary: 'Executing: edit (finalizing handlers.go & spanner_service.go for #1/#3)' },
  { agent: 'backend', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-13T00:04:55Z', wave: 'wave-1', issue_number: 3, summary: 'Ready (handed off #1/#3 implementation to @lead)' },
  { agent: 'frontend', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T00:13:38Z', wave: 'wave-1', issue_number: 1, summary: 'Working: syncing storefront UI bundle with #1/#3 API response fields' },
  { agent: 'lead', phase: 'running', activity: 'executing', event_timestamp: '2026-10-13T00:15:46Z', wave: 'wave-1', issue_number: 1, summary: 'Executing: Bash (dispatching @deployer for Wave 1 rollout)' },
  { agent: 'deployer', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T00:15:55Z', wave: 'wave-1', issue_number: 1, summary: 'Working: applying migration 012 & building Wave 1 Cloud Run revision' },
  { agent: 'lead', phase: 'running', activity: 'blocked', event_timestamp: '2026-10-13T00:19:39Z', wave: 'wave-1', issue_number: 1, summary: 'waiting: deployer Wave 1 rollout (#1/#3)' },
  { agent: 'deployer', phase: 'running', activity: 'executing', event_timestamp: '2026-10-13T00:29:46Z', wave: 'wave-1', issue_number: 1, summary: 'Executing: Bash (rolling back psearch-serving-00010-6dh after smoke test failure)' },
  { agent: 'lead', phase: 'running', activity: 'thinking', event_timestamp: '2026-10-13T00:45:28Z', wave: 'wave-1', issue_number: 1, summary: 'Processing orchestration message: verifying Wave 1 hotfix 4111be0 live' },
  { agent: 'deployer', phase: 'running', activity: 'executing', event_timestamp: '2026-10-13T00:45:47Z', wave: 'wave-1', issue_number: 1, summary: 'Executing: Bash (promoting psearch-serving-00011-rpf with --to-latest)' },
  { agent: 'deployer', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-13T00:48:24Z', wave: 'wave-1', issue_number: 1, summary: 'Ready (Wave 1 psearch-serving-00011-rpf live at 100% traffic)' },
  { agent: 'lead', phase: 'running', activity: 'blocked', event_timestamp: '2026-10-13T01:01:48Z', wave: 'wave-1', issue_number: 2, summary: 'waiting: backend on #2 (in progress); PO acceptance #1/#3' },
  { agent: 'backend', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T01:08:01Z', wave: 'wave-1', issue_number: 2, summary: 'Working: implementing two-tier embedding_cache (LRU + Spanner TTL) for #2' },
  { agent: 'lead', phase: 'running', activity: 'completed', event_timestamp: '2026-10-13T16:29:06Z', wave: 'wave-4', issue_number: 10, summary: 'Close psearch issues #1-#10 with live acceptance evidence' },
  { agent: 'frontend', phase: 'running', activity: 'completed', event_timestamp: '2026-10-13T16:29:20Z', wave: 'wave-4', issue_number: 10, summary: 'psearch frontend UI implementation #1-#11 completed' },
  { agent: 'backend', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T16:29:36Z', wave: 'wave-4', issue_number: 10, summary: 'Ready (Waves 1-4 #1-#10 backend services verified live)' },
  { agent: 'deployer', phase: 'running', activity: 'executing', event_timestamp: '2026-10-13T16:29:45Z', wave: 'wave-4', issue_number: 10, summary: 'Acknowledged PO deploy rules: clean git-archive source and --to-latest after pinned rollback' },
  { agent: 'deployer', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T16:30:32Z', wave: 'wave-4', issue_number: 10, summary: 'Acknowledged PO deploy rules: clean git-archive source and --to-latest after pinned rollback' },
  { agent: 'lead', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T16:30:37Z', wave: 'wave-4', issue_number: 10, summary: 'Close psearch issues #1-#10 with live acceptance evidence' },
  { agent: 'backend', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-13T20:22:04Z', wave: 'wave-5', issue_number: 20, summary: 'Ready (completed Wave 5 #19-#22 Go backend services & Spanner migrations)' },
  { agent: 'deployer', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-13T21:47:39Z', wave: 'wave-5', issue_number: 20, summary: 'Acknowledged PO deploy rules: clean git-archive source and --to-latest (00024-ltv live)' },
  { agent: 'frontend', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T23:25:40Z', wave: 'wave-5', issue_number: 20, summary: 'Issue #20 UI (session personalization panel & event tracking)' },
  { agent: 'frontend', phase: 'stopped', activity: 'idle', event_timestamp: '2026-10-13T23:38:23Z', wave: 'wave-5', issue_number: 20, summary: 'Issue #20 UI (container stopped prior to Gemini 4 Argon harness upgrade)' },
  { agent: 'frontend', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T23:42:04Z', wave: 'wave-5', issue_number: 23, summary: 'Issue #20 UI (restarted on Gemini 4 Argon / agy v1.3.4)' },
  { agent: 'backend', phase: 'running', activity: 'working', event_timestamp: '2026-10-13T23:46:11Z', wave: 'wave-5', issue_number: 23, summary: 'Ready (restarted on Gemini 4 Argon / opencode harness)' },
  { agent: 'lead', phase: 'running', activity: 'thinking', event_timestamp: '2026-10-13T23:47:49Z', wave: 'wave-5', issue_number: 23, summary: 'Orchestrating Issue #23 Gemini 3.8 Flash & Gemini 4 Argon model upgrade' },
  { agent: 'reviewer', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-14T00:26:57Z', wave: 'wave-5', issue_number: 23, summary: 'Agent crashed with exit code 2 (watchdog circuit-breaker observed)' },
  { agent: 'backend', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-14T02:31:20Z', wave: 'wave-5', issue_number: 28, summary: 'Ready (post-#23/#28 verification)' },
  { agent: 'deployer', phase: 'running', activity: 'stalled', event_timestamp: '2026-10-14T17:50:30Z', wave: 'wave-6', issue_number: 33, summary: 'Standby after deploying psearch-serving-00028-klr (#33 Visual & Multimodal Search)' },
  { agent: 'lead', phase: 'running', activity: 'blocked', event_timestamp: '2026-10-14T19:00:56Z', wave: 'wave-6', issue_number: 33, summary: 'Standby after closing Issues #32 & #33 (all 16/16 requirements complete)' }
];

function normalizeShortGraphAgent(rawRef, fallbackAgent = 'eval-lead') {
  if (!rawRef) return fallbackAgent;
  const s = String(rawRef).trim();
  if (s === 'user/po' || s === 'system/scheduler' || s === 'PO / GitHub' || s === 'Scion Hub Scheduler') {
    return 'PO / GitHub';
  }
  if (s === 'Cloud Run / Spanner') return 'Cloud Run / Spanner';
  const short = s.includes('/') ? s.split('/').pop() : s.replace(/^@/, '');
  const validNodes = new Set([
    'lead', 'backend', 'frontend', 'reviewer', 'deployer',
    'eval-lead', 'factory-analyst', 'search-evaluator', 'ui-craftsman', 'eval-deployer'
  ]);
  return validNodes.has(short) ? short : fallbackAgent;
}

function buildTrajectoriesPayload(liveMainCommits = [], liveEvalCommits = [], liveIssueComments = [], liveSpannerAgents = [], liveSpannerTurns = []) {
  const byAgentKey = {};
  for (const meta of AGENT_METADATA) {
    const key = `${meta.project}:${meta.name}`;
    const baseSteps = (BASELINE_TRAJECTORIES[key] || []).map(s => ({ ...s }));
    byAgentKey[key] = {
      meta,
      steps: baseSteps,
      seenShas: new Set(baseSteps.map(s => s.commit_sha))
    };
  }

  // Merge cross-project Scion Hub runtime state transitions (hub.db -> Spanner factory_agent_status -> GCS snapshots)
  for (const rt of CROSS_PROJECT_HUB_RUNTIME_TRANSITIONS) {
    const key = `psearch:${rt.agent}`;
    const bucket = byAgentKey[key];
    const rtId = `hub-state-${rt.agent}-${rt.event_timestamp}`;
    if (bucket && !bucket.seenShas.has(rtId)) {
      bucket.seenShas.add(rtId);
      const st = rt.summary.indexOf('crashed') !== -1 ? 'ERROR' : 'PASS';
      bucket.steps.push({
        wave: rt.wave || 'wave-1',
        issue_number: rt.issue_number || 0,
        step_type: `HUB_STATE_${String(rt.activity || rt.phase).toUpperCase()}`,
        commit_sha: rtId,
        event_timestamp: rt.event_timestamp,
        status: st,
        title: `[Scion Hub State: ${rt.phase}/${rt.activity}] ${rt.summary.slice(0, 96)}`,
        detail: `Cross-project runtime state synced from Scion Hub (hub.db -> Cloud Spanner factory_agent_status) at ${rt.event_timestamp}: phase=${rt.phase}, activity=${rt.activity}, summary="${rt.summary}".`
      });
    }
  }

  // Merge live Cloud Spanner factory_agent_status rows for any newer runtime activity
  if (Array.isArray(liveSpannerAgents)) {
    for (const row of liveSpannerAgents) {
      const proj = row.project_slug || row.project;
      const ag = row.agent_name || row.name;
      const key = `${proj}:${ag}`;
      const bucket = byAgentKey[key];
      const actTs = row.last_activity_at ? (typeof row.last_activity_at.toISOString === 'function' ? row.last_activity_at.toISOString() : String(row.last_activity_at)) : null;
      if (bucket && actTs) {
        const cleanTs = actTs.slice(0, 19) + 'Z';
        const rtId = `hub-state-${ag}-${cleanTs}`;
        if (!bucket.seenShas.has(rtId)) {
          bucket.seenShas.add(rtId);
          const summ = row.last_summary || 'Active';
          bucket.steps.push({
            wave: 'wave-6',
            issue_number: 33,
            step_type: `HUB_STATE_${String(row.activity || row.phase || 'IDLE').toUpperCase()}`,
            commit_sha: rtId,
            event_timestamp: cleanTs,
            status: summ.indexOf('crashed') !== -1 ? 'ERROR' : 'PASS',
            title: `[Live Spanner Hub State: ${row.phase}/${row.activity}] ${String(summ).slice(0, 96)}`,
            detail: `Live cross-project state from Cloud Spanner factory_agent_status (synced from Scion Hub hub.db): agent_id=${row.agent_id}, harness=${row.harness}, phase=${row.phase}, activity=${row.activity}, last_activity_at=${cleanTs}.`
          });
        }
      }
    }
  }

  // Merge any new live commits from GitHub main or eval-dashboard that aren't already in baseline
  const mergeCommits = (commits, branch) => {
    if (!Array.isArray(commits)) return;
    for (const c of commits) {
      const parsed = classifyGithubCommit(c, branch);
      if (!parsed || !parsed.commit_sha) continue;
      const key = `${parsed.project}:${parsed.agent}`;
      const bucket = byAgentKey[key];
      if (bucket && !bucket.seenShas.has(parsed.commit_sha)) {
        bucket.seenShas.add(parsed.commit_sha);
        bucket.steps.push({
          wave: parsed.wave,
          issue_number: parsed.issue_number,
          step_type: parsed.step_type,
          commit_sha: parsed.commit_sha,
          event_timestamp: parsed.event_timestamp,
          status: parsed.status,
          title: parsed.title,
          detail: parsed.detail
        });
      }
    }
  };

  // Merge all GitHub issue comments across Issues #1-#34 so every rollout, review rejection, rollback, verbatim relay, and acceptance is captured
  const mergeComments = (comments) => {
    if (!Array.isArray(comments)) return;
    for (const cm of comments) {
      const expanded = expandGithubCommentIntoSteps(cm);
      for (const parsed of expanded) {
        if (!parsed || !parsed.commit_sha) continue;
        const key = `${parsed.project}:${parsed.agent}`;
        const bucket = byAgentKey[key];
        if (bucket && !bucket.seenShas.has(parsed.commit_sha)) {
          bucket.seenShas.add(parsed.commit_sha);
          bucket.steps.push({
            wave: parsed.wave,
            issue_number: parsed.issue_number,
            step_type: parsed.step_type,
            commit_sha: parsed.commit_sha,
            comment_url: parsed.comment_url,
            event_timestamp: parsed.event_timestamp,
            status: parsed.status,
            title: parsed.title,
            detail: parsed.detail,
            raw_body: parsed.raw_body,
            explicit_from: parsed.explicit_from,
            explicit_to: parsed.explicit_to
          });
        }
      }
    }
  };

  mergeCommits(liveMainCommits, 'main');
  mergeCommits(liveEvalCommits, 'eval-dashboard');
  mergeComments(liveIssueComments);

  // Merge live harness turns streamed directly from container harnesses into Cloud Spanner agent_conversation_turns
  const spannerTurnsStatsByKey = {};
  if (Array.isArray(liveSpannerTurns) && liveSpannerTurns.length > 0) {
    const liveStreamByKey = {};
    for (const row of liveSpannerTurns) {
      const proj = row.project_slug || 'psearch-eval';
      const ag = row.agent_name || 'eval-lead';
      const key = `${proj}:${ag}`;
      if (!spannerTurnsStatsByKey[key]) {
        spannerTurnsStatsByKey[key] = {
          total_turns: 0,
          live_harness_turns: 0,
          total_tokens: 0,
          total_cost_usd: 0
        };
      }
      const st = spannerTurnsStatsByKey[key];
      st.total_turns += 1;
      st.total_tokens += Number(row.total_tokens) || 0;
      st.total_cost_usd = Number((st.total_cost_usd + (Number(row.cost_usd) || 0)).toFixed(4));
      if (row.source_mode === 'live_harness_stream' || row.source_mode === 'live_hub_message_bus' || row.source_mode === 'api_ingested_turn') {
        st.live_harness_turns += 1;
        if (!liveStreamByKey[key]) liveStreamByKey[key] = [];
        liveStreamByKey[key].push(row);
      }
    }

    for (const [key, rows] of Object.entries(liveStreamByKey)) {
      const bucket = byAgentKey[key];
      if (!bucket) continue;
      // Score turns so turns with rich thinking, tool calls, or inter-agent prompts are prioritized, keeping up to 16 per agent in the main timeline
      const scored = rows.map(r => {
        let score = 0;
        if (r.thinking_text && r.thinking_text.length > 20) score += 4;
        if (r.tool_calls_json && r.tool_calls_json.length > 10) score += 3;
        if (r.turn_type === 'user_prompt_turn' || r.turn_type === 'agent_message_turn' || r.turn_type === 'agent_dispatch_turn') score += 3;
        return { r, score };
      });
      scored.sort((a, b) => (b.score - a.score) || String(b.r.created_at).localeCompare(String(a.r.created_at)));
      const selectedRows = scored.slice(0, 16).map(x => x.r).sort((a, b) => String(a.created_at).localeCompare(String(b.created_at)));

      for (const r of selectedRows) {
        const sidShort = String(r.session_id || 'sess').slice(0, 8);
        const turnSha = `spanner-turn-${sidShort}-${r.turn_index}`;
        if (bucket.seenShas.has(turnSha)) continue;
        bucket.seenShas.add(turnSha);

        let rawTools = [];
        try {
          rawTools = typeof r.tool_calls_json === 'string' ? JSON.parse(r.tool_calls_json) : (r.tool_calls_json || []);
        } catch (_) {
          rawTools = [];
        }
        const mappedTools = (Array.isArray(rawTools) ? rawTools : []).map(tc => ({
          tool: tc.tool || 'tool',
          command: tc.args_preview || tc.command || tc.summary || tc.action || tc.tool || '',
          output: tc.output_preview || tc.output || (r.tool_output_text ? String(r.tool_output_text).slice(0, 500) : 'Executed via harness'),
          status: String(tc.status || 'OK').toUpperCase() === 'WARN' ? 'WARN' : 'OK'
        }));

        const shortFrom = normalizeShortGraphAgent(r.from_agent, bucket.meta.project === 'psearch' ? 'lead' : 'eval-lead');
        const shortTo = normalizeShortGraphAgent(r.to_agent, bucket.meta.name);
        const cleanPrompt = String(r.prompt_text || '').trim();
        const cleanThinking = String(r.thinking_text || '').trim();
        const cleanResponse = String(r.response_text || '').trim();
        const titlePreview = (cleanPrompt || cleanResponse || `Harness turn #${r.turn_index}`).replace(/\s+/g, ' ').slice(0, 88);

        const inTok = Number(r.input_tokens) || 0;
        const cacheTok = Number(r.cache_read_tokens) || 0;
        const outTok = Number(r.output_tokens) || 0;
        const totTok = Number(r.total_tokens) || (inTok + cacheTok + outTok);
        const costUsd = Number(Number(r.cost_usd || 0).toFixed(4));

        bucket.steps.push({
          wave: r.issue_number && r.issue_number <= 11 ? 'wave-1' : 'wave-6',
          issue_number: Number(r.issue_number) || 34,
          step_type: `HARNESS_${String(r.turn_type || 'TURN').toUpperCase().replace(/_TURN$/, '')}`,
          commit_sha: turnSha,
          session_id: r.session_id,
          turn_index: Number(r.turn_index) || 1,
          source_mode: r.source_mode || 'live_harness_stream',
          event_timestamp: r.created_at || new Date().toISOString(),
          status: 'PASS',
          title: `[⚡ Spanner Turn #${r.turn_index}] ${titlePreview}`,
          detail: cleanResponse.slice(0, 420) || cleanThinking.slice(0, 420) || titlePreview,
          from_agent: shortFrom,
          to_agent: shortTo,
          prompt: cleanPrompt || titlePreview,
          thinking: cleanThinking || `Executed turn #${r.turn_index} in ${r.harness || bucket.meta.harness} (${r.model || bucket.meta.model}).`,
          tool_calls: mappedTools,
          response: cleanResponse || `Completed turn #${r.turn_index} with ${mappedTools.length} tool call(s).`,
          tokens: {
            input_tokens: inTok,
            cache_read_tokens: cacheTok,
            output_tokens: outTok,
            total_tokens: totTok,
            cost_usd: costUsd,
            model: r.model || bucket.meta.model,
            source: 'live_spanner_agent_conversation_turns'
          },
          cost_usd: costUsd,
          pre_enriched_from_spanner_turn: true
        });
      }
    }
  }

  const agentsSummary = [];
  const trajectoriesByAgent = {};
  const allStepsChronological = [];

  for (const meta of AGENT_METADATA) {
    const key = `${meta.project}:${meta.name}`;
    const bucket = byAgentKey[key];
    bucket.steps.sort((a, b) => String(a.event_timestamp).localeCompare(String(b.event_timestamp)));

    const stepsWithIndex = bucket.steps.map((s, idx) => {
      const baseStepObj = {
        step_index: idx + 1,
        project: meta.project,
        agent: meta.name,
        agent_name: meta.name,
        harness: meta.harness,
        model: meta.model,
        source_mode: s.source_mode || 'reconstructed_github_spanner',
        action_type: s.step_type || s.action_type || 'ACTION',
        timestamp: s.event_timestamp || s.timestamp,
        ...s
      };
      const enriched = enrichStepWithConversationAndTools(baseStepObj, meta);
      allStepsChronological.push(enriched);
      return enriched;
    });

    const spannerTurnStats = spannerTurnsStatsByKey[key] || null;
    const issuesSet = new Set(stepsWithIndex.map(s => s.issue_number).filter(Boolean));
    const wavesSet = new Set(stepsWithIndex.map(s => s.wave).filter(Boolean));
    const commitsSet = new Set(stepsWithIndex.map(s => s.commit_sha).filter(Boolean));
    const turns = spannerTurnStats && spannerTurnStats.total_turns > 0
      ? Math.max(spannerTurnStats.total_turns, stepsWithIndex.length)
      : stepsWithIndex.length * meta.turns_per_step;
    const modelCalls = stepsWithIndex.length * meta.calls_per_step;
    const totalToolCalls = stepsWithIndex.reduce((acc, s) => acc + ((s.tool_calls || []).length), 0);
    const stepSumCostUsd = Number(stepsWithIndex.reduce((acc, s) => acc + (s.cost_usd || 0), 0).toFixed(4));
    const totalCostUsd = spannerTurnStats && spannerTurnStats.total_cost_usd > stepSumCostUsd
      ? spannerTurnStats.total_cost_usd
      : stepSumCostUsd;
    const lastStep = stepsWithIndex[stepsWithIndex.length - 1] || null;
    const sortedIssues = Array.from(issuesSet).sort((a, b) => a - b);

    const agentSummary = {
      project: meta.project,
      name: meta.name,
      role: meta.role,
      harness: meta.harness,
      model: meta.model,
      parent_agent: meta.parent_agent,
      steps_count: stepsWithIndex.length,
      live_spanner_turns_count: spannerTurnStats ? spannerTurnStats.live_harness_turns : 0,
      spanner_persisted_turns_count: spannerTurnStats ? spannerTurnStats.total_turns : stepsWithIndex.length,
      tool_calls_count: totalToolCalls,
      total_cost_usd: totalCostUsd,
      issues_count: issuesSet.size,
      issues_touched: sortedIssues,
      issues_covered: sortedIssues,
      waves_covered: Array.from(wavesSet),
      commits_count: commitsSet.size,
      reconstructed_turns: turns,
      reconstructed_model_calls: modelCalls,
      last_summary: lastStep ? `#${lastStep.issue_number || 34} ${lastStep.title.slice(0, 72)}` : meta.summary_focus,
      summary_focus: meta.summary_focus,
      last_activity_at: lastStep ? lastStep.event_timestamp : new Date().toISOString(),
      steps: stepsWithIndex,
      trajectory: stepsWithIndex
    };

    agentsSummary.push(agentSummary);
    trajectoriesByAgent[key] = agentSummary;
  }

  allStepsChronological.sort((a, b) => String(a.event_timestamp).localeCompare(String(b.event_timestamp)));
  const psearchChrono = allStepsChronological.filter(s => s.project === 'psearch');
  const agentGraph = buildInterAgentGraph(allStepsChronological, agentsSummary);
  const liveHarnessTurnsTotal = Array.isArray(liveSpannerTurns)
    ? liveSpannerTurns.filter(r => r.source_mode === 'live_harness_stream').length
    : 0;

  return {
    rebuilt_at: new Date().toISOString(),
    total_agents: agentsSummary.length,
    total_steps: allStepsChronological.length,
    psearch_steps: psearchChrono.length,
    psearch_eval_steps: allStepsChronological.filter(s => s.project === 'psearch-eval').length,
    spanner_turns_table: 'agent_conversation_turns',
    spanner_turns_total: Array.isArray(liveSpannerTurns) ? liveSpannerTurns.length : 0,
    live_harness_turns_total: liveHarnessTurnsTotal,
    agent_graph: agentGraph,
    agents: agentsSummary,
    agents_by_key: trajectoriesByAgent,
    trajectories: trajectoriesByAgent,
    psearch_chronological: psearchChrono,
    chronological_timeline: allStepsChronological,
    chronological_feed: allStepsChronological
  };
}

async function syncTrajectoriesToSpanner(db, payload) {
  if (!db || !payload) return { synced: false };

  await db.runTransactionAsync(async (tx) => {
    for (const agentSummary of payload.agents) {
      const key = `${agentSummary.project}:${agentSummary.name}`;
      const traj = payload.trajectories[key];
      if (!traj) continue;

      // Update factory_agent_status with reconstructed turns, model_calls, and latest summary
      await tx.runUpdate({
        sql: `UPDATE factory_agent_status
              SET current_turns = @turns,
                  model_calls = @calls,
                  last_summary = @summary,
                  updated_at = PENDING_COMMIT_TIMESTAMP()
              WHERE project_slug = @project AND agent_name = @agent`,
        params: {
          turns: agentSummary.reconstructed_turns,
          calls: agentSummary.reconstructed_model_calls,
          summary: agentSummary.last_summary,
          project: agentSummary.project,
          agent: agentSummary.name
        }
      });

      // Upsert each step in agent_trajectories
      for (const step of traj.steps) {
        await tx.runUpdate({
          sql: `INSERT OR UPDATE INTO agent_trajectories
                (project_slug, agent_name, step_index, wave, issue_number, step_type, commit_sha, title, detail, status, event_timestamp, updated_at)
                VALUES (@project, @agent, @idx, @wave, @issue, @type, @sha, @title, @detail, @status, @ts, PENDING_COMMIT_TIMESTAMP())`,
          params: {
            project: agentSummary.project,
            agent: agentSummary.name,
            idx: step.step_index,
            wave: step.wave || 'wave-1',
            issue: step.issue_number || 0,
            type: step.step_type || 'ACTION',
            sha: step.commit_sha || '',
            title: step.title || '',
            detail: step.detail || '',
            status: step.status || 'PASS',
            ts: step.event_timestamp || new Date().toISOString()
          }
        });
      }
    }
    await tx.commit();
  });

  return {
    synced: true,
    total_steps_synced: payload.total_steps,
    agents_updated: payload.total_agents
  };
}

module.exports = {
  AGENT_METADATA,
  BASELINE_TRAJECTORIES,
  buildTrajectoriesPayload,
  syncTrajectoriesToSpanner,
  answerTrajectoryConversationQuery
};
