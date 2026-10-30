package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
	"psearch/serving-go/internal/config"
	"psearch/serving-go/internal/models"
)

type SpannerService struct {
	client       *spanner.Client
	config       *config.Config
	embeddings   QueryEmbedder
	seedProducts []models.SearchResult
	backendMode  string
	synonyms     *SynonymService // optional query expansion (Issue #7)
	rules        *RuleService    // optional merchandising rules (Issue #6)
	ads          *AdService      // optional sponsored campaigns (Issue #35, ADR-011)
	inventory    []InventoryRow  // memory-mode regional inventory (Issue #8), lazily built
	invOnce      sync.Once
	pricing      *memoryPricing              // memory-mode contract prices (Issue #22)
	variants           map[string][]models.Variant          // memory-mode SKU variants (Issue #32)
	varOnce            sync.Once
	attrs              map[string]map[string][]string // memory-mode dynamic product attributes (Issue #42)
	attrOnce           sync.Once
	attrMu             sync.RWMutex
	nullVectorIDs      map[string]bool // memory-mode NULL vector override for ADR-016 fallback tests
	priceOnce          sync.Once
	annCooldownUntil   atomic.Int64 // ADR-012 (#39): UnixNano timestamp until which ScaNN ANN is cooled down after index error
	lastANNSucceeded   atomic.Bool  // ADR-012 (#39): true when the most recent ScaNN ANN query succeeded
	hybridQueryRunner  func(ctx context.Context, sql string, params map[string]interface{}, minScore, alpha float64, plan vectorPlan, f models.SearchFilters) ([]models.SearchResult, []models.SearchResult, error)
	lexicalCountRunner func(ctx context.Context, sql string, params map[string]interface{}) (int, error)
}

const annCooldownDuration = 60 * time.Second

// IsANNCooledDown reports whether ScaNN ANN is temporarily disabled after an index/emulator error.
func (s *SpannerService) IsANNCooledDown() bool {
	return time.Now().UnixNano() < s.annCooldownUntil.Load()
}

// VectorIndexActive reports whether ScaNN ANN is enabled and not in a cooldown window (ADR-012, #39).
func (s *SpannerService) VectorIndexActive() bool {
	return s != nil && s.config != nil && s.config.EffectiveUseVectorIndex() && !s.IsANNCooledDown()
}

// VectorIndexUsed reports whether ScaNN ANN is active AND has succeeded on Spanner (ADR-012, #39).
func (s *SpannerService) VectorIndexUsed() bool {
	return s.VectorIndexActive() && s.lastANNSucceeded.Load()
}

func (s *SpannerService) shouldUseANN(plan vectorPlan) bool {
	return !plan.lexicalOnly && plan.column == vectorColumnV2 && s.VectorIndexActive()
}

func isANNIndexError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"products_embedding_v2_idx",
		"approx_cosine_distance",
		"vector index",
		"index not found",
		"backfill",
		"not ready",
		"unsupported",
		"unimplemented",
		"invalidargument",
		"failedprecondition",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func (s *SpannerService) memPricing() *memoryPricing {
	s.priceOnce.Do(func() { s.pricing = newMemoryPricing(DefaultPriceRows(s.seedProducts)) })
	return s.pricing
}

// SetRules installs the merchandising rule service (Issue #6, ADR-004).
func (s *SpannerService) SetRules(r *RuleService) { s.rules = r }

func nonNilBoosts(b []ruleBoost) []ruleBoost {
	if b == nil {
		return []ruleBoost{}
	}
	return b
}

func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// SetSynonyms installs the synonym expansion service (Issue #7, ADR-003).
func (s *SpannerService) SetSynonyms(syn *SynonymService) { s.synonyms = syn }

func NewSpannerService(ctx context.Context, cfg *config.Config, embeddings QueryEmbedder) (*SpannerService, error) {
	svc := &SpannerService{
		config:       cfg,
		embeddings:   embeddings,
		seedProducts: DefaultSeedProducts(),
		backendMode:  "memory",
	}

	if cfg.UseMemoryStore || cfg.ProjectID == "" || cfg.SpannerInstanceID == "" || cfg.SpannerDatabaseID == "" {
		return svc, nil
	}

	databaseName := fmt.Sprintf("projects/%s/instances/%s/databases/%s",
		cfg.ProjectID, cfg.SpannerInstanceID, cfg.SpannerDatabaseID)
	client, err := spanner.NewClient(ctx, databaseName)
	if err != nil {
		log.Printf("WARN: Spanner client creation failed (%v), using memory fallback", err)
		return svc, nil
	}
	svc.client = client
	svc.backendMode = "spanner"

	go func() {
		seedCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		// Retry once: on a cold, CPU-throttled instance the first session can take
		// longer than the budget. The check only seeds an EMPTY products table, so a
		// failure here never affects serving.
		err := svc.SeedIfEmpty(seedCtx)
		if err != nil && seedCtx.Err() != nil {
			retryCtx, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel2()
			err = svc.SeedIfEmpty(retryCtx)
		}
		if err != nil {
			log.Printf("INFO: Spanner seed check skipped (non-fatal): %v", err)
		}
	}()

	return svc, nil
}

func (s *SpannerService) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

// SetQueryEmbedder replaces the query embedder (used to install the embedding cache,
// which needs the Spanner client created by NewSpannerService).
func (s *SpannerService) SetQueryEmbedder(e QueryEmbedder) { s.embeddings = e }

// Client exposes the Spanner client (nil in memory mode).
func (s *SpannerService) Client() *spanner.Client { return s.client }

// NewMemorySpannerServiceForTest constructs an in-memory SpannerService with a custom
// seed catalog for unit testing large-catalog pagination and count semantics (ADR-012, #40).
func NewMemorySpannerServiceForTest(cfg *config.Config, products []models.SearchResult) *SpannerService {
	return &SpannerService{
		config:       cfg,
		seedProducts: products,
		backendMode:  "memory",
	}
}

// NewSpannerRunnerServiceForTest constructs a SpannerService with lexicalCountRunner and
// hybridQueryRunner hooks that simulate RRF rank shifting whenever params["limit"] != wantWindow,
// verifying that Stage-1 retrieval always uses the page-independent fixed window W (#40 fix-3).
func NewSpannerRunnerServiceForTest(cfg *config.Config, products []models.SearchResult, lexicalCount, wantWindow int, errLog interface{ Fatalf(string, ...interface{}) }) *SpannerService {
	return &SpannerService{
		config:       cfg,
		seedProducts: products,
		backendMode:  "spanner",
		lexicalCountRunner: func(_ context.Context, _ string, _ map[string]interface{}) (int, error) {
			return lexicalCount, nil
		},
		hybridQueryRunner: func(_ context.Context, _ string, params map[string]interface{}, _, _ float64, _ vectorPlan, _ models.SearchFilters) ([]models.SearchResult, []models.SearchResult, error) {
			gotLimit, _ := params["limit"].(int64)
			if int(gotLimit) != wantWindow && errLog != nil {
				errLog.Fatalf("hybridQueryRunner called with @limit=%d, want fixed window W=%d", gotLimit, wantWindow)
			}
			lim := int(gotLimit)
			if lim > len(products) {
				lim = len(products)
			}
			out := make([]models.SearchResult, lim)
			// If gotLimit ever varied across pages, shifting indices by gotLimit%17 would cause duplicate/missing IDs.
			shift := lim % 17
			for i := 0; i < lim; i++ {
				srcIdx := (i + shift) % lim
				cp := products[srcIdx]
				txtScore := 0.0
				if srcIdx < lexicalCount {
					txtScore = 0.015
				}
				cp.Score = map[string]float64{"text": txtScore, "vector": 0.01, "final_score": txtScore + 0.01}
				out[i] = cp
			}
			return out, nil, nil
		},
	}
}

func (s *SpannerService) BackendMode() string {
	return s.backendMode
}

func (s *SpannerService) ProductCount() int {
	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		stmt := spanner.Statement{SQL: `SELECT COUNT(*) FROM products`}
		iter := s.client.Single().Query(ctx, stmt)
		defer iter.Stop()
		if row, err := iter.Next(); err == nil {
			var cnt int64
			if err := row.Columns(&cnt); err == nil && cnt > 0 {
				return int(cnt)
			}
		}
	}
	return len(s.seedProducts)
}

func (s *SpannerService) SeedIfEmpty(ctx context.Context) error {
	if s.client == nil {
		return nil
	}
	stmt := spanner.Statement{SQL: `SELECT COUNT(*) FROM products`}
	iter := s.client.Single().Query(ctx, stmt)
	row, err := iter.Next()
	iter.Stop()
	if err != nil {
		return err
	}
	var count int64
	if err := row.Columns(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	log.Printf("Seeding %d baseline products into Cloud Spanner (%s)...", len(s.seedProducts), s.config.SpannerDatabaseID)
	var muts []*spanner.Mutation
	for _, p := range s.seedProducts {
		rawJSON, _ := json.Marshal(p)
		emb := DeterministicEmbedding(p.Title+" "+p.Description+" "+strings.Join(p.Categories, " ")+" "+strings.Join(p.Brands, " "), 768)
		cat := ""
		if len(p.Categories) > 0 {
			cat = p.Categories[0]
		}
		brand := ""
		if len(p.Brands) > 0 {
			brand = p.Brands[0]
		}
		sig := models.BoostSignals{}
		if p.Signals != nil {
			sig = *p.Signals
		}
		stock := int64(0)
		if p.AvailableQuantity != nil {
			stock = int64(*p.AvailableQuantity)
		}
		muts = append(muts, spanner.InsertOrUpdate("products",
			[]string{"product_id", "product_data", "title", "description", "category", "brand", "price", "is_available", "stock", "is_visible", "channels", "tenant_id", "locale", "clicks", "orders", "discount_pct", "popularity_score", "margin_pct", "ctr", "conversion_rate", "recency_score", "embedding", "timestamp"},
			[]interface{}{
				p.ID,
				spanner.NullJSON{Value: json.RawMessage(rawJSON), Valid: true},
				p.Title,
				p.Description,
				cat,
				brand,
				p.PriceInfo.NumericPrice,
				p.Availability == "IN_STOCK",
				stock,
				true,
				p.Channels,
				p.TenantID,
				p.Locale,
				sig.Clicks, sig.Orders, sig.DiscountPct, sig.PopularityScore, sig.MarginPct, sig.CTR, sig.ConversionRate, sig.RecencyScore,
				emb,
				spanner.CommitTimestamp,
			},
		))
	}
	_, err = s.client.Apply(ctx, muts)
	return err
}

// Retrieval modes reported per search response so operators can verify which
// path actually served a query (Issue #11 rollout finding).
const (
	RetrievalHybridV1    = "hybrid_v1"             // Spanner RRF, legacy 768-d deterministic vectors
	RetrievalHybridV2    = "hybrid_v2"             // Spanner RRF, gemini-embedding-2 1024-d vectors
	RetrievalLexicalOnly = "lexical_only_degraded" // v2 active but query embedding unavailable
	RetrievalMemory      = "memory_fallback"
	RetrievalBrowse      = "catalog_browse" // empty / "*" query: full catalog listing (#30)       // in-process demo engine (no Spanner / Spanner error)
)

// SearchOutcome carries results plus the retrieval path that produced them.
type SearchOutcome struct {
	Results         []models.SearchResult
	TotalFound      int                   // ADR-012 (#40): pre-slice organic match count
	Sponsored       []models.SearchResult // ADR-011 (#35): candidate sponsored matches before slot placement
	RetrievalMode   string
	VectorIndexUsed bool     // ADR-012 (#39): true when ScaNN vector index served the vector leg
	CacheHit        bool     // query embedding served from the embedding cache (v2 only)
	ExpandedTerms   []string // synonym phrases added to the text leg (Issue #7)
	AppliedRules    []string // merchandising rule_ids applied (Issue #6)
	Pins, Buries    []string // merchandising pins/buries (kept by the LTR re-ranker, Issue #10)
}

// fixedSearchWindow computes the offset-independent candidate window W = min(max(lexicalCount, 100), 500)
// so the ANN vector leg never inflates total_found as pagination offset grows (ADR-012, #40 fix-2).
func fixedSearchWindow(lexicalCount int) int {
	w := lexicalCount
	if w < 100 {
		w = 100
	}
	if w > 500 {
		w = 500
	}
	return w
}

// HybridSearch executes hybrid vector + full-text RRF search against Cloud Spanner (with deterministic fallback).
func (s *SpannerService) HybridSearch(ctx context.Context, query string, limit int, minScore float64, alpha float64, filters models.SearchFilters) ([]models.SearchResult, error) {
	out, err := s.HybridSearchDetailed(ctx, query, limit, minScore, alpha, filters)
	return out.Results, err
}

// HybridSearchDetailed is HybridSearch plus the retrieval mode that served the query.
func (s *SpannerService) HybridSearchDetailed(ctx context.Context, query string, limit int, minScore float64, alpha float64, filters models.SearchFilters) (SearchOutcome, error) {
	if limit <= 0 {
		limit = s.config.DefaultLimit
	}
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}

	// Catalog browse (#30): an empty or "*" query lists the catalog (filters still
	// apply) instead of running hybrid retrieval.
	if IsBrowseQuery(query) && len(filters.ImageVector) == 0 {
		if s.client != nil || s.lexicalCountRunner != nil {
			var totalCnt int
			if cnt, cerr := s.spannerBrowseCount(ctx, filters); cerr == nil {
				totalCnt = cnt
			}
			w := fixedSearchWindow(totalCnt)
			queryLimit := limit
			if limit >= 100 {
				queryLimit = w
			}
			if s.client != nil {
				res, browseErr := s.spannerBrowse(ctx, queryLimit, filters)
				if browseErr == nil {
					if len(res) > w {
						res = res[:w]
					}
					if totalCnt < len(res) {
						totalCnt = len(res)
					}
					return SearchOutcome{Results: res, TotalFound: totalCnt, RetrievalMode: RetrievalBrowse}, nil
				}
				log.Printf("WARN: Spanner catalog browse fallback triggered: %v", browseErr)
			}
		}
		res, total := s.memoryHybridSearchExpandedWithTotal("", limit, minScore, alpha, filters, Expansion{}, RuleSet{})
		return SearchOutcome{Results: res, TotalFound: total, RetrievalMode: RetrievalMemory}, nil
	}
	nf := normalizeFilters(filters)
	expansion := s.synonyms.Expand(ctx, nf.TenantID, nf.Locale, query)
	ruleSet := s.rules.Match(ctx, nf.TenantID, query)
	if s.client != nil && len(filters.ImageVector) > 0 && s.config.UseGeminiEmbeddings() {
		var plan vectorPlan
		var lexCnt int
		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			plan = s.planImageQuery(gctx, query, filters)
			return nil
		})
		if !IsBrowseQuery(query) {
			g.Go(func() error {
				if cnt, cerr := s.spannerLexicalCount(gctx, query, expansion, filters); cerr == nil {
					lexCnt = cnt
				}
				return nil
			})
		}
		_ = g.Wait()
		plan.expansion = expansion
		plan.rules = ruleSet
		w := fixedSearchWindow(lexCnt)
		queryLimit := limit
		if limit >= 100 {
			queryLimit = w
		}
		res, sponsored, vecIdxUsed, searchErr := s.spannerHybridSearchWithAds(ctx, query, queryLimit, minScore, alpha, plan, filters)
		if searchErr == nil {
			if len(res) > w {
				res = res[:w]
			}
			totalFound := len(res)
			if lexCnt > totalFound {
				totalFound = lexCnt
			}
			return SearchOutcome{Results: res, TotalFound: totalFound, Sponsored: sponsored, RetrievalMode: RetrievalMultimodal, VectorIndexUsed: vecIdxUsed, CacheHit: plan.cacheHit, ExpandedTerms: expansion.Terms, AppliedRules: ruleSet.Applied, Pins: ruleSet.Pins, Buries: ruleSet.Buries}, nil
		}
		log.Printf("WARN: Spanner multimodal search fallback triggered: %v", searchErr)
	}
	if len(filters.ImageVector) > 0 && IsBrowseQuery(query) {
		// The in-memory engine has no product vectors in the gemini-embedding-2
		// space, so an image-only query cannot be served from it. Return an empty,
		// honest result rather than an unrelated catalog listing.
		return SearchOutcome{Results: []models.SearchResult{}, RetrievalMode: RetrievalMemory}, nil
	}
	if (s.client != nil || s.hybridQueryRunner != nil) && strings.TrimSpace(query) != "" {
		var plan vectorPlan
		var lexCnt int
		g, gctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			plan = s.planVectorQuery(gctx, query)
			return nil
		})
		if s.client != nil || s.lexicalCountRunner != nil {
			g.Go(func() error {
				if cnt, cerr := s.spannerLexicalCount(gctx, query, expansion, filters); cerr == nil {
					lexCnt = cnt
				}
				return nil
			})
		}
		_ = g.Wait()
		plan.expansion = expansion
		plan.rules = ruleSet
		w := fixedSearchWindow(lexCnt)
		queryLimit := limit
		if limit >= 100 {
			queryLimit = w
		}
		res, sponsored, vecIdxUsed, err := s.spannerHybridSearchWithAds(ctx, query, queryLimit, minScore, alpha, plan, filters)
		if len(res) > w {
			res = res[:w]
		}
		totalFound := len(res)
		if lexCnt > totalFound {
			totalFound = lexCnt
		}
		switch {
		case err != nil:
			log.Printf("WARN: Spanner HybridSearch fallback triggered: %v", err)
		case plan.lexicalOnly:
			// Degraded v2: return Spanner's lexical results as-is (even if empty).
			// Falling back to the in-memory engine here would mask the outage with
			// deterministic 768-d vector scores.
			return SearchOutcome{Results: res, TotalFound: totalFound, Sponsored: sponsored, RetrievalMode: RetrievalLexicalOnly, ExpandedTerms: expansion.Terms, AppliedRules: ruleSet.Applied, Pins: ruleSet.Pins, Buries: ruleSet.Buries}, nil
		default:
			// A successful Spanner query is authoritative even when it returns no rows
			// (e.g. tenant/category filters exclude everything). Falling back to the
			// in-memory seed engine here could leak products Spanner filtered out (#4).
			mode := RetrievalHybridV1
			if plan.column == vectorColumnV2 {
				mode = RetrievalHybridV2
			}
			return SearchOutcome{Results: res, TotalFound: totalFound, Sponsored: sponsored, RetrievalMode: mode, VectorIndexUsed: vecIdxUsed, CacheHit: plan.cacheHit, ExpandedTerms: expansion.Terms, AppliedRules: ruleSet.Applied, Pins: ruleSet.Pins, Buries: ruleSet.Buries}, nil
		}
	}

	org, total := s.memoryHybridSearchExpandedWithTotal(query, limit, minScore, alpha, filters, expansion, ruleSet)
	sponsored := s.memorySponsoredMatches(ctx, query, alpha, filters)
	return SearchOutcome{Results: org, TotalFound: total, Sponsored: sponsored, RetrievalMode: RetrievalMemory, ExpandedTerms: expansion.Terms, AppliedRules: ruleSet.Applied, Pins: ruleSet.Pins, Buries: ruleSet.Buries}, nil
}

// vectorPlan describes how the vector leg of the hybrid query is executed.
type vectorPlan struct {
	column      string      // vector column to search ("embedding" for v1, "embedding_v2" for v2)
	embedding   interface{} // []float64 (v1 deterministic) or []float32 (v2 Gemini)
	lexicalOnly bool        // true when no usable query embedding exists (degraded mode)
	cacheHit    bool        // query embedding came from the embedding cache
	vectorOnly  bool        // image-only query: no text leg (ADR-010, #33)
	multimodal  bool        // a query image contributed to the vector (ADR-010, #33)
	expansion   Expansion   // synonym expansion for the text leg
	rules       RuleSet     // merchandising rules matching the query
}

// metaEmbedder is implemented by EmbeddingCache.
type metaEmbedder interface {
	EmbedQueryWithMeta(ctx context.Context, text string) ([]float32, bool, error)
}

const (
	vectorColumnV1 = "embedding"
	vectorColumnV2 = "embedding_v2"
)

// planVectorQuery selects the vector column / query embedding for the active
// embedding version (ADR-001 Blue/Green). v1 keeps the legacy 768-d deterministic
// embedding; v2 uses gemini-embedding-2 via the Gen AI SDK. Any embedding failure
// degrades to lexical-only retrieval instead of failing the request (Issue #11).
func (s *SpannerService) planVectorQuery(ctx context.Context, query string) vectorPlan {
	if !s.config.UseGeminiEmbeddings() {
		return vectorPlan{column: vectorColumnV1, embedding: DeterministicEmbedding(query, config.LegacyEmbeddingDimension)}
	}
	if s.embeddings == nil {
		log.Printf("WARN: ACTIVE_EMBEDDING_VERSION=v2 but embedding service is unavailable; lexical-only search")
		return vectorPlan{column: vectorColumnV2, lexicalOnly: true}
	}
	var emb []float32
	var hit bool
	var err error
	if me, ok := s.embeddings.(metaEmbedder); ok {
		emb, hit, err = me.EmbedQueryWithMeta(ctx, query)
	} else {
		emb, err = s.embeddings.EmbedQuery(ctx, query)
	}
	if err != nil {
		log.Printf("WARN: query embedding failed (%v); lexical-only search", err)
		return vectorPlan{column: vectorColumnV2, lexicalOnly: true}
	}
	return vectorPlan{column: vectorColumnV2, embedding: emb, cacheHit: hit}
}

// planImageQuery builds the vector leg for a query image (ADR-010, #33). With
// text, the image and text vectors are fused and the text also drives the fts
// leg; if the text embedding fails the image vector is used alone (the fts leg
// still runs). Without text only the vector leg runs.
func (s *SpannerService) planImageQuery(ctx context.Context, query string, filters models.SearchFilters) vectorPlan {
	w := filters.ImageWeight
	if w <= 0 || w > 1 {
		w = DefaultImageWeight
	}
	if IsBrowseQuery(query) {
		return vectorPlan{column: vectorColumnV2, embedding: FuseVectors(filters.ImageVector, nil, 1), vectorOnly: true, multimodal: true}
	}
	txt := s.planVectorQuery(ctx, query)
	var tv []float32
	if !txt.lexicalOnly {
		tv, _ = txt.embedding.([]float32)
	}
	return vectorPlan{column: vectorColumnV2, embedding: FuseVectors(filters.ImageVector, tv, w), cacheHit: txt.cacheHit, multimodal: true}
}

// catalogFilterSQL is applied inside BOTH retrieval legs (ADR-002 / Issue #3;
// disjunctive multi-select OR within dimension, AND across dimensions per ADR-013 #41):
// hidden products are never returned; channel and only-available filters are
// optional (empty channel / false flag disables them). NULL columns default to
// visible, all channels, available.
var catalogFilterSQL = `COALESCE(is_visible, TRUE)
					AND (@channel = '' OR @channel IN UNNEST(IFNULL(channels, ['web', 'mobile', 'store'])))
					AND (NOT @only_available OR (IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0))
					AND IFNULL(products.tenant_id, 'default') = @tenant_id
					AND (@locale = '' OR IFNULL(locale, '') = @locale)
					AND ((@category = '' AND ARRAY_LENGTH(@categories) = 0) OR LOWER(IFNULL(category, '')) = @category OR LOWER(IFNULL(category, '')) IN UNNEST(@categories))
					AND ((@brand = '' AND ARRAY_LENGTH(@brands) = 0) OR LOWER(IFNULL(brand, '')) = @brand OR LOWER(IFNULL(brand, '')) IN UNNEST(@brands))
					AND (@min_price <= 0 OR IFNULL(` + effectivePriceSQL("products") + `, 0) >= @min_price)
					AND (@max_price <= 0 OR IFNULL(` + effectivePriceSQL("products") + `, 0) <= @max_price)
					AND (ARRAY_LENGTH(@price_ranges) = 0 OR (` + facetPriceSQL + `) IN UNNEST(@price_ranges))
					AND ` + assortmentSQL + `
					AND ` + variantFilterSQL + `
					AND ` + deliveryFilterSQL

// annStoringFilterSQL contains ONLY scalar predicates on columns stored in
// products_embedding_v2_idx (STORING: is_visible, is_available, stock, tenant_id,
// locale, category, brand, price). Cloud Spanner rejects APPROX_COSINE_DISTANCE
// with @{FORCE_INDEX=products_embedding_v2_idx} if non-STORING columns or complex
// subqueries (channels UNNEST, product_prices, skus, regional_inventory) appear
// before ORDER BY distance ASC (Issue #39 prod rollout finding).
const annStoringFilterSQL = `COALESCE(is_visible, TRUE)
					AND (NOT @only_available OR (IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0))
					AND IFNULL(products.tenant_id, 'default') = @tenant_id
					AND (@locale = '' OR IFNULL(locale, '') = @locale)
					AND (@category = '' OR LOWER(IFNULL(category, '')) = @category)
					AND (@brand = '' OR LOWER(IFNULL(brand, '')) = @brand)
					AND (@min_price <= 0 OR IFNULL(price, 0) >= @min_price)`

func catalogFilterForAlias(alias string) string {
	priceBucketAlias := strings.ReplaceAll(facetPriceTmpl, "EFF", effectivePriceSQL(alias))
	return `COALESCE(` + alias + `.is_visible, TRUE)
					AND (@channel = '' OR @channel IN UNNEST(IFNULL(` + alias + `.channels, ['web', 'mobile', 'store'])))
					AND (NOT @only_available OR (IFNULL(` + alias + `.is_available, TRUE) AND IFNULL(` + alias + `.stock, 1) > 0))
					AND IFNULL(` + alias + `.tenant_id, 'default') = @tenant_id
					AND (@locale = '' OR IFNULL(` + alias + `.locale, '') = @locale)
					AND ((@category = '' AND ARRAY_LENGTH(@categories) = 0) OR LOWER(IFNULL(` + alias + `.category, '')) = @category OR LOWER(IFNULL(` + alias + `.category, '')) IN UNNEST(@categories))
					AND ((@brand = '' AND ARRAY_LENGTH(@brands) = 0) OR LOWER(IFNULL(` + alias + `.brand, '')) = @brand OR LOWER(IFNULL(` + alias + `.brand, '')) IN UNNEST(@brands))
					AND (@min_price <= 0 OR IFNULL(` + effectivePriceSQL(alias) + `, 0) >= @min_price)
					AND (@max_price <= 0 OR IFNULL(` + effectivePriceSQL(alias) + `, 0) <= @max_price)
					AND (ARRAY_LENGTH(@price_ranges) = 0 OR (` + priceBucketAlias + `) IN UNNEST(@price_ranges))
					AND ` + strings.ReplaceAll(assortmentSQL, "products.", alias+".") + `
					AND ` + strings.ReplaceAll(variantFilterSQL, "products.", alias+".") + `
					AND ` + strings.ReplaceAll(deliveryFilterSQL, "products.", alias+".")
}

// optimizerHint pins the hybrid query to the latest Spanner query optimizer.
const optimizerHint = "@{OPTIMIZER_VERSION=latest_version} "

// buildHybridSQL renders the RRF hybrid query. column is always one of the
// package constants (never user input). Unavailable products (is_available =
// FALSE or stock <= 0) are always ordered after available ones.
func buildHybridSQL(column string, lexicalOnly bool) string {
	return buildHybridSQLWith(column, lexicalOnly, false)
}

// buildHybridSQLWith optionally searches the accent-folded tokenlists
// (title_tokens_folded / description_tokens_folded, migration 014b).
func buildHybridSQLWith(column string, lexicalOnly, folded bool) string {
	return buildHybridSQLMode(column, lexicalOnly, false, folded)
}

// buildHybridSQLMode adds vectorOnly (ADR-010, #33): an image-only query has no
// text, so only the ann leg runs (weight 1.0) and the fts leg is omitted.
func buildHybridSQLMode(column string, lexicalOnly, vectorOnly, folded bool) string {
	return buildHybridSQLANN(column, lexicalOnly, vectorOnly, folded, false, 20)
}

// buildHybridSQLANN renders the hybrid RRF SQL using either the Cloud Spanner ScaNN
// VECTOR INDEX (`products_embedding_v2_idx` + `APPROX_COSINE_DISTANCE`, ADR-012 #39)
// when useANN is true on embedding_v2, or exact `COSINE_DISTANCE` otherwise.
func buildHybridSQLANN(column string, lexicalOnly, vectorOnly, folded, useANN bool, numLeaves int) string {
	if numLeaves <= 0 {
		numLeaves = 20
	}
	titleTok, descTok := "title_tokens", "description_tokens"
	if folded {
		titleTok, descTok = "title_tokens_folded", "description_tokens_folded"
	}
	fts := `
		fts AS (
			SELECT offset + 1 AS rank, product_id, title, product_data, is_available, stock
			FROM UNNEST(ARRAY(
				SELECT AS STRUCT product_id, title, product_data, is_available, stock
				FROM products
				WHERE (SEARCH(` + titleTok + `, @query_text) OR SEARCH(` + descTok + `, @query_text)
					OR SEARCH(` + titleTok + `, @syn_text) OR SEARCH(` + descTok + `, @syn_text))
					AND ` + catalogFilterSQL + `
				ORDER BY
					/* exact_first: exact matches always outrank synonym-only matches (ADR-003) */
					IF(SCORE(` + titleTok + `, @query_text) + SCORE(` + descTok + `, @query_text) > 0, 0, 1),
					(SCORE(` + titleTok + `, @query_text) + SCORE(` + descTok + `, @query_text))
						+ @syn_weight * (SCORE(` + titleTok + `, @syn_text) + SCORE(` + descTok + `, @syn_text)) DESC
				LIMIT @limit
			)) WITH OFFSET AS offset
		)`
	ann := `
		ann AS (
			SELECT offset + 1 AS rank, product_id, title, product_data, is_available, stock
			FROM UNNEST(ARRAY(
				SELECT AS STRUCT product_id, title, product_data, is_available, stock
				FROM products
				WHERE ` + column + ` IS NOT NULL
					AND ` + catalogFilterSQL + `
				ORDER BY COSINE_DISTANCE(` + column + `, @query_embedding)
				LIMIT @limit
			)) WITH OFFSET AS offset
		),`
	if useANN && column == vectorColumnV2 && !lexicalOnly {
		ann = `
		ann AS (
			SELECT offset + 1 AS rank, product_id, '' AS title, CAST(NULL AS JSON) AS product_data, is_available, stock
			FROM UNNEST(ARRAY(
				SELECT AS STRUCT product_id, is_available, stock,
					APPROX_COSINE_DISTANCE(` + column + `, @query_embedding, options => JSON '{"num_leaves_to_search": ` + strconv.Itoa(numLeaves) + `}') AS distance
				FROM products@{FORCE_INDEX=products_embedding_v2_idx}
				WHERE ` + column + ` IS NOT NULL
					AND ` + annStoringFilterSQL + `
				ORDER BY distance ASC
				LIMIT @ann_limit
			)) WITH OFFSET AS offset
		),`
	}
	union := `
			SELECT 'vector' AS source, @alpha AS weight, rank, product_id, title, product_data, is_available, stock FROM ann
			UNION ALL
			SELECT 'text' AS source, (1.0 - @alpha) AS weight, rank, product_id, title, product_data, is_available, stock FROM fts`
	adTextBranch := `
						SELECT product_id, 1.0 AS relevance
						FROM products
						WHERE SEARCH(` + titleTok + `, @query_text) OR SEARCH(` + descTok + `, @query_text)`
	adVecBranch := `
						SELECT product_id, (1.0 - COSINE_DISTANCE(` + column + `, @query_embedding)) AS relevance
						FROM products
						WHERE ` + column + ` IS NOT NULL
							AND (1.0 - COSINE_DISTANCE(` + column + `, @query_embedding)) >= @ad_min_similarity`
	adUnion := adVecBranch + `
						UNION ALL` + adTextBranch
	if vectorOnly && !lexicalOnly {
		fts = ""
		ann = strings.TrimSuffix(ann, ",")
		union = `
			SELECT 'vector' AS source, 1.0 AS weight, rank, product_id, title, product_data, is_available, stock FROM ann`
		adUnion = adVecBranch
	}
	if lexicalOnly {
		ann = ""
		union = `
			SELECT 'text' AS source, 1.0 AS weight, rank, product_id, title, product_data, is_available, stock FROM fts`
		adUnion = adTextBranch
	}
	sponsoredCTE := `,
		sponsored_matches AS (
			SELECT sm.ad_rank, sm.product_id, sm.title, sm.product_data, sm.is_available, sm.stock,
				` + effectivePriceSQL("p") + ` AS effective_price,
				` + variantsArraySQL("sm") + ` AS variants,
				sm.campaign_id, sm.cpc_micros, sm.relevance
			FROM (
				SELECT offset + 1 AS ad_rank, product_id, title, product_data, is_available, stock, campaign_id, cpc_micros, relevance
				FROM UNNEST(ARRAY(
					SELECT AS STRUCT
						products.product_id AS product_id,
						products.title AS title,
						products.product_data AS product_data,
						products.is_available AS is_available,
						products.stock AS stock,
						ac.campaign_id AS campaign_id,
						ac.cpc_micros AS cpc_micros,
						rel.relevance AS relevance
					FROM ad_campaigns AS ac
					JOIN products ON products.product_id = ac.product_id
					JOIN (
						SELECT product_id, MAX(relevance) AS relevance
						FROM (` + adUnion + `
						)
						GROUP BY product_id
					) AS rel ON rel.product_id = ac.product_id
					WHERE ac.tenant_id = @tenant_id
						AND ac.status = 'active'
						AND ac.spent_micros + ac.cpc_micros <= ac.budget_micros
						AND (ac.start_at IS NULL OR ac.start_at <= CURRENT_TIMESTAMP())
						AND (ac.end_at IS NULL OR ac.end_at >= CURRENT_TIMESTAMP())
						AND IFNULL(products.is_available, TRUE) AND IFNULL(products.stock, 1) > 0
						AND ` + catalogFilterSQL + `
					ORDER BY CAST(ac.cpc_micros AS FLOAT64) * rel.relevance DESC, ac.campaign_id
					LIMIT @ad_k
				)) WITH OFFSET AS offset
			) AS sm
			JOIN products AS p ON p.product_id = sm.product_id
		)`
	outerSelectCols := `a.*`
	outerWhere := ""
	if useANN && column == vectorColumnV2 && !lexicalOnly {
		// Because the ANN CTE only projects STORING columns (product_id, is_available, stock)
		// and over-fetches (@ann_limit), the outer join on products AS p supplies p.title
		// and p.product_data and enforces the full catalogFilterSQL.
		outerSelectCols = `a.rrf_score, a.vector_score, a.text_score, a.product_id,
						p.title AS title, p.product_data AS product_data, p.is_available AS is_available, p.stock AS stock`
		outerWhere = "\n\t\t\t\t\tWHERE " + catalogFilterForAlias("p")
	}
	// The aggregate query is wrapped so the unavailable-last ORDER BY works on
	// plain columns (ordering by ANY_VALUE(...) directly nests aggregates, which
	// Spanner rejects: "Aggregations of aggregations are not allowed").
	return `
		WITH` + ann + fts + sponsoredCTE + `
		SELECT rrf_score, vector_score, text_score, product_id, title, product_data, is_available, stock, boost_score, rule_mult, effective_price, variants, is_sponsored, campaign_id, cpc_micros
		FROM (
			SELECT * FROM (
				SELECT rrf_score, vector_score, text_score, product_id, title, product_data, is_available, stock, boost_score, rule_mult, effective_price, variants,
					FALSE AS is_sponsored, '' AS campaign_id, 0 AS cpc_micros,
					pin_rank, buried, 0 AS ad_rank, popularity_sort, rating_sort
				FROM (
					/* ADR-004: final = rrf_score + boost_signal_score + merchandising_rule_delta,
					   rule_delta = (rule_mult - 1) * rrf_score; pins first, buries last (within availability) */
					SELECT ` + outerSelectCols + `,
						@boost_weight * ` + boostSignalSQL + ` AS boost_score,
						IFNULL((SELECT EXP(SUM(LN(r.multiplier))) FROM UNNEST(@rule_boosts) AS r
							WHERE (r.brand = '' OR LOWER(IFNULL(p.brand, '')) = r.brand)
								AND (r.category = '' OR LOWER(IFNULL(p.category, '')) = r.category)), 1.0) AS rule_mult,
						IFNULL((SELECT MIN(o) FROM UNNEST(@pin_ids) AS pid WITH OFFSET AS o WHERE pid = a.product_id), 1000) AS pin_rank,
						IF(a.product_id IN UNNEST(@bury_ids), 1, 0) AS buried,
						` + effectivePriceSQL("p") + ` AS effective_price,
						` + popularitySortSQL("p") + ` AS popularity_sort,
						` + ratingSortSQL("p") + ` AS rating_sort,
						` + variantsArraySQL("a") + ` AS variants
					FROM (
					SELECT
						SUM(weight / (60.0 + rank)) AS rrf_score,
						MAX(IF(source = 'vector', 1.0 / (60.0 + rank), 0.0)) AS vector_score,
						MAX(IF(source = 'text', 1.0 / (60.0 + rank), 0.0)) AS text_score,
						product_id,
						ANY_VALUE(title) AS title,
						ANY_VALUE(product_data) AS product_data,
						ANY_VALUE(is_available) AS is_available,
						ANY_VALUE(stock) AS stock
					FROM (` + union + `
					)
					GROUP BY product_id
					) AS a
					JOIN products AS p ON p.product_id = a.product_id` + outerWhere + `
				)
				/* Issue #45: select Stage-1 @limit candidates by relevance/sort regardless of availability
				   so ANN fill cannot push strong lexical OOS matches out of the window; ADR-002 OOS-last
				   ordering is applied in the outer SELECT over the selected candidate window. */
				ORDER BY
					pin_rank, buried,
					IF(@sort_by = 'price_asc', effective_price, NULL) ASC, IF(@sort_by = 'price_desc', effective_price, NULL) DESC,
					IF(@sort_by = 'popularity', popularity_sort, NULL) DESC, IF(@sort_by = 'rating_desc', rating_sort, NULL) DESC,
					rrf_score * rule_mult + boost_score DESC /* final_score */,
					vector_score DESC, text_score DESC, product_id
				LIMIT @limit
			)
			UNION ALL
			SELECT relevance AS rrf_score, relevance AS vector_score, relevance AS text_score, product_id, title, product_data, is_available, stock,
				0.0 AS boost_score, 1.0 AS rule_mult, effective_price, variants,
				TRUE AS is_sponsored, campaign_id, cpc_micros,
				1000 AS pin_rank, 0 AS buried, ad_rank, 0.0 AS popularity_sort, 0.0 AS rating_sort
			FROM sponsored_matches
		)
		ORDER BY is_sponsored, ad_rank,
			IF(IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0, 0, 1) /* unavailable_rank */,
			pin_rank, buried,
			IF(@sort_by = 'price_asc', effective_price, NULL) ASC, IF(@sort_by = 'price_desc', effective_price, NULL) DESC,
			IF(@sort_by = 'popularity', popularity_sort, NULL) DESC, IF(@sort_by = 'rating_desc', rating_sort, NULL) DESC,
			rrf_score * rule_mult + boost_score DESC /* final_score */,
			vector_score DESC, text_score DESC, product_id
	`
}

// CheckSpannerQuery runs the real Spanner hybrid query (no memory fallback) and
// returns its error, so operators can validate SQL against psearch-db before a
// rollout (cmd/sqlcheck). Read-only.
func (s *SpannerService) CheckSpannerQuery(ctx context.Context, query string, alpha float64, filters models.SearchFilters) (int, string, error) {
	if s.client == nil {
		return 0, "", fmt.Errorf("no Spanner client (memory mode)")
	}
	plan := s.planVectorQuery(ctx, query)
	res, err := s.spannerHybridSearch(ctx, query, s.config.DefaultLimit, 0, alpha, plan, filters)
	mode := RetrievalHybridV1
	if plan.lexicalOnly {
		mode = RetrievalLexicalOnly
	} else if plan.column == vectorColumnV2 {
		mode = RetrievalHybridV2
	}
	return len(res), mode, err
}

// CheckSpannerQueryANN runs the Spanner hybrid query with USE_VECTOR_INDEX explicitly
// toggled to useANN (ADR-012, #39) and reports whether the ScaNN vector index was used.
func (s *SpannerService) CheckSpannerQueryANN(ctx context.Context, query string, alpha float64, filters models.SearchFilters, useANN bool) (int, bool, string, error) {
	if s.client == nil && s.hybridQueryRunner == nil {
		return 0, false, "", fmt.Errorf("no Spanner client (memory mode)")
	}
	prev := s.config.UseVectorIndex
	prevCooldown := s.annCooldownUntil.Swap(0)
	s.config.UseVectorIndex = useANN
	defer func() {
		s.config.UseVectorIndex = prev
		s.annCooldownUntil.Store(prevCooldown)
	}()
	plan := s.planVectorQuery(ctx, query)
	res, _, vecUsed, err := s.spannerHybridSearchWithAds(ctx, query, s.config.DefaultLimit, 0, alpha, plan, filters)
	mode := RetrievalHybridV1
	if plan.lexicalOnly {
		mode = RetrievalLexicalOnly
	} else if plan.column == vectorColumnV2 {
		mode = RetrievalHybridV2
	}
	return len(res), vecUsed, mode, err
}

// isUnavailable implements the ADR-002 availability rule shared by Spanner and memory modes.
func isUnavailable(availability string, quantity *int) bool {
	return availability == "OUT_OF_STOCK" || (quantity != nil && *quantity <= 0)
}

// matchesChannel reports whether a product sold on channels matches the requested channel
// (empty request or empty product channel list = all channels).
func matchesChannel(requested string, channels []string) bool {
	if requested == "" || len(channels) == 0 {
		return true
	}
	for _, c := range channels {
		if c == requested {
			return true
		}
	}
	return false
}

func injectDynamicAttrClauses(sql string, attrs map[string][]string, params map[string]interface{}) string {
	if len(attrs) == 0 {
		return sql
	}
	prodClause, attrParams := dynamicAttrFilterSQL("products", attrs)
	pClause, _ := dynamicAttrFilterSQL("p", attrs)
	for k, v := range attrParams {
		params[k] = v
	}
	sql = strings.ReplaceAll(sql, catalogFilterSQL, catalogFilterSQL+prodClause)
	pBase := catalogFilterForAlias("p")
	sql = strings.ReplaceAll(sql, pBase, pBase+pClause)
	return sql
}

func (s *SpannerService) enrichResultsWithAttributes(ctx context.Context, results []models.SearchResult) {
	if len(results) == 0 {
		return
	}
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	attrMap := s.ProductAttributesByIDs(ctx, ids)
	for i := range results {
		if m, ok := attrMap[results[i].ID]; ok {
			attachDynamicAttributes(&results[i], m)
		}
	}
}

func (s *SpannerService) spannerHybridSearch(ctx context.Context, query string, limit int, minScore float64, alpha float64, plan vectorPlan, filters models.SearchFilters) ([]models.SearchResult, error) {
	organic, _, _, err := s.spannerHybridSearchWithAds(ctx, query, limit, minScore, alpha, plan, filters)
	return organic, err
}

func (s *SpannerService) spannerHybridSearchWithAds(ctx context.Context, query string, limit int, minScore float64, alpha float64, plan vectorPlan, filters models.SearchFilters) ([]models.SearchResult, []models.SearchResult, bool, error) {
	queryText := query
	if s.config.TextFolding {
		queryText = FoldAccents(query)
	}
	if filters.RelaxText {
		queryText = RelaxedQuery(queryText) // ADR-008: implicit AND -> OR
	}
	f := normalizeFilters(filters)
	// Synonym leg: when there is no expansion, reuse the exact query with weight 0
	// (avoids an empty SEARCH query and leaves scores unchanged).
	synText, synWeight := queryText, 0.0
	if len(plan.expansion.Terms) > 0 {
		synText, synWeight = plan.expansion.SpannerQuery(), plan.expansion.Weight
	}
	annLimit := limit * 3
	if annLimit < 60 {
		annLimit = 60
	}
	if annLimit > 500 {
		annLimit = 500
	}
	params := map[string]interface{}{
		"query_text":     queryText,
		"syn_text":       synText,
		"syn_weight":     synWeight,
		"boost_weight":   s.config.BoostWeight,
		"rule_boosts":    nonNilBoosts(plan.rules.Boosts),
		"pin_ids":        nonNilStrings(plan.rules.Pins),
		"bury_ids":       nonNilStrings(plan.rules.Buries),
		"tenant_id":      f.TenantID,
		"locale":         f.Locale,
		"category":       f.Category,
		"categories":     nonNilStrings(f.Categories),
		"brand":          f.Brand,
		"brands":         nonNilStrings(f.Brands),
		"zip5":           f.Zip5,
		"min_price":      f.MinPrice,
		"price_table":    f.PriceTable,
		"price_ranges":   nonNilStrings(f.PriceRanges),
		"color":          f.Color,
		"colors":         nonNilStrings(f.Colors),
		"size":           f.Size,
		"sizes":          nonNilStrings(f.Sizes),
		"sort_by":        priceSortKey(f.SortBy),
		"max_price":      f.MaxPrice,
		"max_sla":        f.MaxSLAHours,
		"alpha":          alpha,
		"limit":          int64(limit),
		"ann_limit":      int64(annLimit),
		"channel":        f.Channel,
		"only_available": f.OnlyInStock,
		"ad_k":           int64(s.config.EffectiveAdMax()),
	}
	if !plan.lexicalOnly {
		params["query_embedding"] = plan.embedding
		params["ad_min_similarity"] = s.config.EffectiveAdMinSimilarity()
	}

	useANN := s.shouldUseANN(plan)
	numLeaves := s.config.EffectiveANNNumLeaves()
	// The correlated product_prices lookups (#22) change the plan shape; SCORE() in
	// the fts leg then requires query optimizer version >= 6, which is newer than the
	// database default. A statement hint is the least invasive fix (verified by
	// deployer on psearch-db: sqlcheck 27/27).
	sql := optimizerHint + buildHybridSQLANN(plan.column, plan.lexicalOnly, plan.vectorOnly, s.config.TextFolding, useANN, numLeaves)
	sql = injectDynamicAttrClauses(sql, f.Attrs, params)
	results, sponsored, err := s.executeHybridStatement(ctx, sql, params, minScore, alpha, plan, f)
	if err != nil && useANN && isANNIndexError(err) {
		log.Printf("WARN: ScaNN vector index unavailable (%v); cooling down ANN for %s and falling back to exact COSINE_DISTANCE", err, annCooldownDuration)
		s.lastANNSucceeded.Store(false)
		s.annCooldownUntil.Store(time.Now().Add(annCooldownDuration).UnixNano())
		exactSQL := optimizerHint + buildHybridSQLANN(plan.column, plan.lexicalOnly, plan.vectorOnly, s.config.TextFolding, false, numLeaves)
		exactSQL = injectDynamicAttrClauses(exactSQL, f.Attrs, params)
		results, sponsored, err = s.executeHybridStatement(ctx, exactSQL, params, minScore, alpha, plan, f)
		if err == nil && s.client != nil {
			s.enrichResultsWithAttributes(ctx, results)
		}
		return results, sponsored, false, err
	}
	if useANN && err == nil {
		s.lastANNSucceeded.Store(true)
	}
	if err == nil && s.client != nil {
		s.enrichResultsWithAttributes(ctx, results)
	}
	return results, sponsored, useANN && err == nil, err
}

func (s *SpannerService) executeHybridStatement(ctx context.Context, sql string, params map[string]interface{}, minScore, alpha float64, plan vectorPlan, f models.SearchFilters) ([]models.SearchResult, []models.SearchResult, error) {
	if s.hybridQueryRunner != nil {
		return s.hybridQueryRunner(ctx, sql, params, minScore, alpha, plan, f)
	}
	stmt := spanner.Statement{SQL: sql, Params: params}

	iter := s.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var results, sponsored []models.SearchResult
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		var rrfScore, vecScore, txtScore float64
		var productID, title string
		var productDataJSON spanner.NullJSON
		var isAvailable spanner.NullBool
		var stock spanner.NullInt64
		var boostScore, ruleMult float64
		var effPrice spanner.NullFloat64
		var variants []*variantRow
		var isSponsored bool
		var campaignID string
		var cpcMicros int64
		if err := row.Columns(&rrfScore, &vecScore, &txtScore, &productID, &title, &productDataJSON, &isAvailable, &stock, &boostScore, &ruleMult, &effPrice, &variants, &isSponsored, &campaignID, &cpcMicros); err != nil {
			return nil, nil, err
		}
		if !isSponsored && rrfScore < minScore {
			continue
		}
		var item models.SearchResult
		if productDataJSON.Valid {
			raw, _ := json.Marshal(productDataJSON.Value)
			_ = json.Unmarshal(raw, &item)
		}
		if item.ID == "" {
			item.ID = productID
		}
		if item.Title == "" {
			item.Title = title
		}
		if stock.Valid {
			quantity := int(stock.Int64)
			item.AvailableQuantity = &quantity
		}
		if isAvailable.Valid || stock.Valid {
			if (isAvailable.Valid && !isAvailable.Bool) || (stock.Valid && stock.Int64 <= 0) {
				item.Availability = "OUT_OF_STOCK"
			} else {
				item.Availability = "IN_STOCK"
			}
		}
		if effPrice.Valid {
			applyEffectivePrice(&item, effPrice.Float64, f.PriceTable)
		}
		attachVariants(&item, toVariants(variants))
		effAlpha := alpha
		if plan.lexicalOnly {
			effAlpha = 0
		} else if plan.vectorOnly {
			effAlpha = 1
		}
		item.Score = map[string]float64{
			"hybrid":          rrfScore,
			"vector":          vecScore,
			"text":            txtScore,
			"alpha":           effAlpha,
			"boost":           boostScore,
			"rule_multiplier": ruleMult,
			"final_score":     rrfScore*ruleMult + boostScore,
		}
		if isSponsored {
			item.Sponsored = true
			item.AdCampaignID = campaignID
			item.Score["cpc_micros"] = float64(cpcMicros)
			item.Score["ad_relevance"] = rrfScore
			sponsored = append(sponsored, item)
		} else {
			results = append(results, item)
		}
	}
	return results, sponsored, nil
}

func (s *SpannerService) memoryHybridSearch(query string, limit int, minScore float64, alpha float64, filters models.SearchFilters) []models.SearchResult {
	nf := normalizeFilters(filters)
	return s.memoryHybridSearchExpanded(query, limit, minScore, alpha, filters,
		s.synonyms.Expand(context.Background(), nf.TenantID, nf.Locale, query), s.rules.Match(context.Background(), nf.TenantID, query))
}

func (s *SpannerService) memoryHybridSearchExpanded(query string, limit int, minScore float64, alpha float64, filters models.SearchFilters, expansion Expansion, rules RuleSet) []models.SearchResult {
	out, _ := s.memoryHybridSearchExpandedWithTotal(query, limit, minScore, alpha, filters, expansion, rules)
	return out
}

func (s *SpannerService) memoryHybridSearchExpandedWithTotal(query string, limit int, minScore float64, alpha float64, filters models.SearchFilters, expansion Expansion, rules RuleSet) ([]models.SearchResult, int) {
	filters = normalizeFilters(filters)
	qLower := FoldAccents(strings.ToLower(strings.TrimSpace(query)))
	qTokens := strings.Fields(qLower)
	qVec := DeterministicEmbedding(query, 768)

	type scored struct {
		prod     models.SearchResult
		vecSim   float64
		txtRaw   float64
		exact    bool
		vecRank  int
		txtRank  int
		rrfScore float64
		vecRRF   float64
		txtRRF   float64
	}

	items := make([]scored, len(s.seedProducts))
	for i, p := range s.seedProducts {
		docText := FoldAccents(strings.ToLower(p.Title + " " + p.Description + " " + strings.Join(p.Brands, " ") + " " + strings.Join(p.Categories, " ")))
		pVec := DeterministicEmbedding(docText, 768)
		vecSim := CosineSimilarity(qVec, pVec)

		var txtRaw float64
		if qLower == "" {
			txtRaw = 1.0
		} else {
			for _, tok := range qTokens {
				if strings.Contains(FoldAccents(strings.ToLower(p.Title)), tok) {
					txtRaw += 2.0
				}
				if strings.Contains(docText, tok) {
					txtRaw += 1.0
				}
			}
		}
		exact := txtRaw > 0
		titleFolded := FoldAccents(strings.ToLower(p.Title))
		for _, term := range expansion.Terms {
			if strings.Contains(titleFolded, term) {
				txtRaw += 2.0 * expansion.Weight
			}
			if strings.Contains(docText, term) {
				txtRaw += 1.0 * expansion.Weight
			}
		}
		items[i] = scored{prod: p, vecSim: vecSim, txtRaw: txtRaw, exact: exact}
	}

	// Assign vector ranks
	vecOrder := make([]int, len(items))
	for i := range vecOrder {
		vecOrder[i] = i
	}
	sort.Slice(vecOrder, func(i, j int) bool {
		return items[vecOrder[i]].vecSim > items[vecOrder[j]].vecSim
	})
	for rankIdx, itemIdx := range vecOrder {
		items[itemIdx].vecRank = rankIdx + 1
	}

	// Assign text ranks
	txtOrder := make([]int, len(items))
	for i := range txtOrder {
		txtOrder[i] = i
	}
	sort.Slice(txtOrder, func(i, j int) bool {
		if items[txtOrder[i]].exact != items[txtOrder[j]].exact {
			return items[txtOrder[i]].exact // exact matches outrank synonym-only (ADR-003)
		}
		if items[txtOrder[i]].txtRaw == items[txtOrder[j]].txtRaw {
			return items[txtOrder[i]].prod.ID < items[txtOrder[j]].prod.ID
		}
		return items[txtOrder[i]].txtRaw > items[txtOrder[j]].txtRaw
	})
	for rankIdx, itemIdx := range txtOrder {
		items[itemIdx].txtRank = rankIdx + 1
	}

	var out []models.SearchResult
	lexicalCount := 0
	for i := range items {
		if filters.OnlyInStock && isUnavailable(items[i].prod.Availability, items[i].prod.AvailableQuantity) {
			continue
		}
		eff := s.memPricing().effective(filters.PriceTable, items[i].prod.ID, items[i].prod.PriceInfo.NumericPrice)
		matchedVariants, variantOK := s.memoryVariantMatch(items[i].prod.ID, filters)
		if !variantOK {
			continue
		}
		matchedAttrs, attrOK := s.memoryAttrMatch(items[i].prod.ID, filters.Attrs)
		if !attrOK {
			continue
		}
		if !s.memPricing().visible(filters.PriceTable, items[i].prod.ID) ||
			(filters.MinPrice > 0 && eff < filters.MinPrice) || (filters.MaxPrice > 0 && eff > filters.MaxPrice) ||
			(len(filters.PriceRanges) > 0 && !containsFold(filters.PriceRanges, PriceBucket(eff))) {
			continue
		}
		if !matchesChannel(filters.Channel, items[i].prod.Channels) || !matchesCatalog(filters, items[i].prod) ||
			!s.deliverableInMemory(items[i].prod.ID, filters.Zip5, filters.MaxSLAHours) {
			continue
		}
		if items[i].txtRaw > 0 || qLower == "" {
			lexicalCount++
			items[i].txtRRF = 1.0 / float64(60+items[i].txtRank)
		}
		items[i].vecRRF = 1.0 / float64(60+items[i].vecRank)
		items[i].rrfScore = alpha*items[i].vecRRF + (1.0-alpha)*items[i].txtRRF
		if items[i].rrfScore < minScore {
			continue
		}
		res := items[i].prod
		boost := 0.0
		if res.Signals != nil {
			boost = s.config.BoostWeight * BoostSignalScore(*res.Signals)
		}
		brand, category := "", ""
		if len(res.Brands) > 0 {
			brand = res.Brands[0]
		}
		if len(res.Categories) > 0 {
			category = res.Categories[0]
		}
		applyEffectivePrice(&res, s.memPricing().effective(filters.PriceTable, res.ID, res.PriceInfo.NumericPrice), filters.PriceTable)
		attachVariants(&res, matchedVariants)
		attachDynamicAttributes(&res, matchedAttrs)
		mult := rules.Multiplier(brand, category)
		res.Score = map[string]float64{
			"hybrid":          items[i].rrfScore,
			"vector":          items[i].vecRRF,
			"text":            items[i].txtRRF,
			"cosine_sim":      items[i].vecSim,
			"alpha":           alpha,
			"boost":           boost,
			"rule_multiplier": mult,
			"final_score":     items[i].rrfScore*mult + boost,
		}
		out = append(out, res)
	}

	// Issue #45: Stage-1 candidate selection orders by relevance/sort regardless of
	// availability so OOS lexical matches are never pushed out of the candidate window
	// by in-stock vector neighbors; ADR-002 OOS-last ordering is applied after slicing
	// the candidate window.
	sort.SliceStable(out, func(i, j int) bool {
		if pi, pj := pinRank(rules.Pins, out[i].ID), pinRank(rules.Pins, out[j].ID); pi != pj {
			return pi < pj // pinned products first, in rule order (ADR-004)
		}
		if bi, bj := containsID(rules.Buries, out[i].ID), containsID(rules.Buries, out[j].ID); bi != bj {
			return !bi // buried products last
		}
		if decided, less := compareExplicitSort(out[i], out[j], filters.SortBy); decided {
			return less
		}
		if out[i].Score["final_score"] != out[j].Score["final_score"] {
			return out[i].Score["final_score"] > out[j].Score["final_score"]
		}
		if out[i].Score["vector"] != out[j].Score["vector"] {
			return out[i].Score["vector"] > out[j].Score["vector"]
		}
		if out[i].Score["text"] != out[j].Score["text"] {
			return out[i].Score["text"] > out[j].Score["text"]
		}
		return out[i].ID < out[j].ID // deterministic tie-break
	})
	w := fixedSearchWindow(lexicalCount)
	effectiveLimit := limit
	if limit >= 100 {
		effectiveLimit = w
	}
	if effectiveLimit > 0 && len(out) > effectiveLimit {
		out = out[:effectiveLimit]
	}
	if len(out) > w {
		out = out[:w]
	}
	// Apply ADR-002 unavailable-last ordering over the selected Stage-1 candidate window.
	sort.SliceStable(out, func(i, j int) bool {
		ui := isUnavailable(out[i].Availability, out[i].AvailableQuantity)
		uj := isUnavailable(out[j].Availability, out[j].AvailableQuantity)
		if ui != uj {
			return !ui
		}
		return false
	})
	// ADR-012 (#40 fix-3): total_found = max(lexical_count, len(truncated)).
	totalFound := len(out)
	if lexicalCount > totalFound {
		totalFound = lexicalCount
	}
	return out, totalFound
}

func normalizeLowerSlice(scalar string, slice []string) (string, []string) {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		for _, part := range strings.Split(s, ",") {
			v := strings.ToLower(strings.TrimSpace(part))
			if v != "" && !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
		}
	}
	add(scalar)
	for _, s := range slice {
		add(s)
	}
	if len(out) == 1 {
		return out[0], out
	}
	return "", out
}

// normalizeFilters lowercases/trims filter values and applies ADR-002 / ADR-013 defaults.
func normalizeFilters(f models.SearchFilters) models.SearchFilters {
	f.Channel = strings.ToLower(strings.TrimSpace(f.Channel))
	f.Category, f.Categories = normalizeLowerSlice(f.Category, f.Categories)
	f.Brand, f.Brands = normalizeLowerSlice(f.Brand, f.Brands)
	f.Color, f.Colors = normalizeLowerSlice(f.Color, f.Colors)
	f.Size, f.Sizes = normalizeLowerSlice(f.Size, f.Sizes)
	if len(f.PriceRanges) > 0 {
		var pr []string
		seen := map[string]bool{}
		for _, raw := range f.PriceRanges {
			for _, part := range strings.Split(raw, ",") {
				v := strings.TrimSpace(part)
				if v != "" && !seen[v] {
					seen[v] = true
					pr = append(pr, v)
				}
			}
		}
		f.PriceRanges = pr
	}
	f.TenantID = strings.TrimSpace(f.TenantID)
	if f.TenantID == "" {
		f.TenantID = "default"
	}
	f.Locale = strings.TrimSpace(f.Locale)
	return f
}

// matchesCatalog applies tenant / locale / category / brand filters in memory mode
// (disjunctive OR within dimension, AND across dimensions per ADR-013 #41).
func matchesCatalog(f models.SearchFilters, p models.SearchResult) bool {
	tenant := p.TenantID
	if tenant == "" {
		tenant = "default"
	}
	if tenant != f.TenantID {
		return false
	}
	if f.Locale != "" && p.Locale != f.Locale {
		return false
	}
	if len(f.Categories) > 0 {
		matched := false
		for _, want := range f.Categories {
			if containsFold(p.Categories, want) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	} else if f.Category != "" && !containsFold(p.Categories, f.Category) {
		return false
	}
	if len(f.Brands) > 0 {
		matched := false
		for _, want := range f.Brands {
			if containsFold(p.Brands, want) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	} else if f.Brand != "" && !containsFold(p.Brands, f.Brand) {
		return false
	}
	return true // price filters use the effective (contract) price, applied by the caller
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func pinRank(pins []string, id string) int {
	for i, p := range pins {
		if p == id {
			return i
		}
	}
	return 1000
}

func containsID(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// IsBrowseQuery reports whether a query asks for the whole catalog (#30).
func IsBrowseQuery(q string) bool {
	q = strings.TrimSpace(q)
	return q == "" || q == "*"
}

// browseSQL lists catalog products with all catalog filters applied, selecting the
// Stage-1 @limit window by sort/boost score regardless of availability (#45) and
// ordering available products first over the selected window (ADR-002).
func browseSQL() string {
	boost := regexp.MustCompile(`\bp\.`).ReplaceAllString(boostSignalSQL, "products.")
	return optimizerHint + `SELECT product_id, title, product_data, is_available, stock, effective_price, boost_score, variants
		FROM (
			SELECT product_id, title, product_data, is_available, stock,
				` + effectivePriceSQL("products") + ` AS effective_price,
				@boost_weight * ` + boost + ` AS boost_score,
				` + variantsArraySQL("products") + ` AS variants,
				` + popularitySortSQL("products") + ` AS popularity_sort,
				` + ratingSortSQL("products") + ` AS rating_sort
			FROM products
			WHERE ` + catalogFilterSQL + `
			ORDER BY
				IF(@sort_by = 'price_asc', effective_price, NULL) ASC, IF(@sort_by = 'price_desc', effective_price, NULL) DESC,
				IF(@sort_by = 'popularity', popularity_sort, NULL) DESC,
				IF(@sort_by = 'rating_desc', rating_sort, NULL) DESC,
				boost_score DESC, product_id
			LIMIT @limit
		)
		ORDER BY IF(IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0, 0, 1),
			IF(@sort_by = 'price_asc', effective_price, NULL) ASC, IF(@sort_by = 'price_desc', effective_price, NULL) DESC,
			IF(@sort_by = 'popularity', popularity_sort, NULL) DESC,
			IF(@sort_by = 'rating_desc', rating_sort, NULL) DESC,
			boost_score DESC, product_id`
}

func (s *SpannerService) spannerBrowse(ctx context.Context, limit int, filters models.SearchFilters) ([]models.SearchResult, error) {
	f := normalizeFilters(filters)
	params := map[string]interface{}{
		"limit": int64(limit), "channel": f.Channel, "only_available": f.OnlyInStock, "tenant_id": f.TenantID,
		"locale": f.Locale, "category": f.Category, "categories": nonNilStrings(f.Categories),
		"brand": f.Brand, "brands": nonNilStrings(f.Brands), "zip5": f.Zip5, "max_sla": f.MaxSLAHours,
		"min_price": f.MinPrice, "max_price": f.MaxPrice, "price_table": f.PriceTable, "price_ranges": nonNilStrings(f.PriceRanges),
		"sort_by": priceSortKey(f.SortBy),
		"color": f.Color, "colors": nonNilStrings(f.Colors), "size": f.Size, "sizes": nonNilStrings(f.Sizes),
		"boost_weight": s.config.BoostWeight,
	}
	sql := injectDynamicAttrClauses(browseSQL(), f.Attrs, params)
	iter := s.client.Single().Query(ctx, spanner.Statement{SQL: sql, Params: params})
	defer iter.Stop()
	results := []models.SearchResult{}
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			s.enrichResultsWithAttributes(ctx, results)
			return results, nil
		}
		if err != nil {
			return nil, err
		}
		var productID, title string
		var productDataJSON spanner.NullJSON
		var isAvailable spanner.NullBool
		var stock spanner.NullInt64
		var effPrice spanner.NullFloat64
		var boost float64
		var variants []*variantRow
		if err := row.Columns(&productID, &title, &productDataJSON, &isAvailable, &stock, &effPrice, &boost, &variants); err != nil {
			return nil, err
		}
		var item models.SearchResult
		if productDataJSON.Valid {
			raw, _ := json.Marshal(productDataJSON.Value)
			_ = json.Unmarshal(raw, &item)
		}
		if item.ID == "" {
			item.ID = productID
		}
		if item.Title == "" {
			item.Title = title
		}
		if stock.Valid {
			q := int(stock.Int64)
			item.AvailableQuantity = &q
		}
		if isAvailable.Valid || stock.Valid {
			if (isAvailable.Valid && !isAvailable.Bool) || (stock.Valid && stock.Int64 <= 0) {
				item.Availability = "OUT_OF_STOCK"
			} else {
				item.Availability = "IN_STOCK"
			}
		}
		if effPrice.Valid {
			applyEffectivePrice(&item, effPrice.Float64, f.PriceTable)
		}
		attachVariants(&item, toVariants(variants))
		item.Score = map[string]float64{"hybrid": 0, "vector": 0, "text": 0, "boost": boost, "rule_multiplier": 1, "final_score": boost}
		results = append(results, item)
	}
}

// CheckSpannerBrowse runs the catalog-browse SQL (cmd/sqlcheck).
func (s *SpannerService) CheckSpannerBrowse(ctx context.Context, filters models.SearchFilters) (int, error) {
	if s.client == nil {
		return 0, fmt.Errorf("no Spanner client (memory mode)")
	}
	res, err := s.spannerBrowse(ctx, 100, filters)
	return len(res), err
}

// buildLexicalCountSQL renders the fast COUNT(*) query over the catalog-filtered
// lexical match set (same SEARCH + synonym predicates + catalogFilterSQL as fts; ADR-012 #40).
func buildLexicalCountSQL(folded bool) string {
	titleTok, descTok := "title_tokens", "description_tokens"
	if folded {
		titleTok, descTok = "title_tokens_folded", "description_tokens_folded"
	}
	return optimizerHint + `SELECT COUNT(*)
		FROM products
		WHERE (SEARCH(` + titleTok + `, @query_text) OR SEARCH(` + descTok + `, @query_text)
			OR SEARCH(` + titleTok + `, @syn_text) OR SEARCH(` + descTok + `, @syn_text))
			AND ` + catalogFilterSQL
}

// buildBrowseCountSQL renders the fast COUNT(*) query for catalog browse (ADR-012 #40).
func buildBrowseCountSQL() string {
	return optimizerHint + `SELECT COUNT(*)
		FROM products
		WHERE ` + catalogFilterSQL
}

func (s *SpannerService) filterCountParams(f models.SearchFilters) map[string]interface{} {
	f = normalizeFilters(f)
	return map[string]interface{}{
		"channel":        f.Channel,
		"only_available": f.OnlyInStock,
		"tenant_id":      f.TenantID,
		"locale":         f.Locale,
		"category":       f.Category,
		"categories":     nonNilStrings(f.Categories),
		"brand":          f.Brand,
		"brands":         nonNilStrings(f.Brands),
		"zip5":           f.Zip5,
		"max_sla":        f.MaxSLAHours,
		"min_price":      f.MinPrice,
		"max_price":      f.MaxPrice,
		"price_table":    f.PriceTable,
		"price_ranges":   nonNilStrings(f.PriceRanges),
		"color":          f.Color,
		"colors":         nonNilStrings(f.Colors),
		"size":           f.Size,
		"sizes":          nonNilStrings(f.Sizes),
	}
}

func (s *SpannerService) spannerLexicalCount(ctx context.Context, query string, expansion Expansion, filters models.SearchFilters) (int, error) {
	queryText := query
	if s.config.TextFolding {
		queryText = FoldAccents(query)
	}
	if filters.RelaxText {
		queryText = RelaxedQuery(queryText)
	}
	synText := queryText
	if len(expansion.Terms) > 0 {
		synText = expansion.SpannerQuery()
	}
	params := s.filterCountParams(filters)
	params["query_text"] = queryText
	params["syn_text"] = synText
	sql := injectDynamicAttrClauses(buildLexicalCountSQL(s.config.TextFolding), filters.Attrs, params)
	if s.lexicalCountRunner != nil {
		return s.lexicalCountRunner(ctx, sql, params)
	}
	if s.client == nil {
		return 0, fmt.Errorf("no Spanner client")
	}
	iter := s.client.Single().Query(ctx, spanner.Statement{SQL: sql, Params: params})
	defer iter.Stop()
	row, err := iter.Next()
	if err != nil {
		return 0, err
	}
	var cnt int64
	if err := row.Columns(&cnt); err != nil {
		return 0, err
	}
	return int(cnt), nil
}

func (s *SpannerService) spannerBrowseCount(ctx context.Context, filters models.SearchFilters) (int, error) {
	params := s.filterCountParams(filters)
	sql := injectDynamicAttrClauses(buildBrowseCountSQL(), filters.Attrs, params)
	if s.lexicalCountRunner != nil {
		return s.lexicalCountRunner(ctx, sql, params)
	}
	if s.client == nil {
		return 0, fmt.Errorf("no Spanner client")
	}
	iter := s.client.Single().Query(ctx, spanner.Statement{SQL: sql, Params: params})
	defer iter.Stop()
	row, err := iter.Next()
	if err != nil {
		return 0, err
	}
	var cnt int64
	if err := row.Columns(&cnt); err != nil {
		return 0, err
	}
	return int(cnt), nil
}

// CheckSpannerCount validates the lexical and browse COUNT(*) SQL against Spanner (cmd/sqlcheck, ADR-012 #40).
func (s *SpannerService) CheckSpannerCount(ctx context.Context, query string, filters models.SearchFilters) (int, error) {
	if IsBrowseQuery(query) {
		return s.spannerBrowseCount(ctx, filters)
	}
	nf := normalizeFilters(filters)
	exp := s.synonyms.Expand(ctx, nf.TenantID, nf.Locale, query)
	return s.spannerLexicalCount(ctx, query, exp, filters)
}
