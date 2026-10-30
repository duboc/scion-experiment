package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"psearch/serving-go/internal/config"
	"psearch/serving-go/internal/models"
	"psearch/serving-go/internal/services"
)

type Controller struct {
	config       *config.Config
	spannerSvc   *services.SpannerService
	embeddingSvc *services.EmbeddingService
	cache        *services.EmbeddingCache // nil when the embedding service is unavailable
	synonyms     *services.SynonymService
	rules        *services.RuleService
	ads          *services.AdService
	autocomplete *services.AutocompleteService
	ltrModel     *services.LTRModel
	intent       *services.QueryUnderstandingService
	spell        *services.SpellCheckService
	session      *services.SessionService
	analytics    *services.AnalyticsService // ADR-015 (#43): closed-loop search telemetry & attribution
	images       imageEmbedder              // ADR-010 (#33); nil when the embedding service is unavailable
}

// imageEmbedder is satisfied by services.ImageEmbeddingCache (fakeable in tests).
type imageEmbedder interface {
	EmbedImageWithMeta(ctx context.Context, data []byte, mimeType string) ([]float32, bool, error)
}

// maxSearchBodyBytes bounds POST /search bodies: a 4 MB image is ~5.4 MB in base64.
const maxSearchBodyBytes = 8 << 20

func NewController(cfg *config.Config) (*Controller, error) {
	ctx := context.Background()
	var embedder services.QueryEmbedder
	embeddingSvc, err := services.NewEmbeddingService(ctx, cfg)
	if err != nil {
		log.Printf("WARN: embedding service unavailable (%v); vector retrieval disabled, lexical search only when ACTIVE_EMBEDDING_VERSION=v2", err)
		embeddingSvc = nil
	} else {
		embedder = embeddingSvc
		log.Printf("embedding: model=%s location=%s active_version=%s", embeddingSvc.Model(), cfg.GenAILocation, cfg.ActiveVersion())
	}
	spannerSvc, err := services.NewSpannerService(ctx, cfg, embedder)
	if err != nil {
		return nil, err
	}
	var cache *services.EmbeddingCache
	if embeddingSvc != nil {
		var store services.CacheStore
		if client := spannerSvc.Client(); client != nil {
			store = services.SpannerCacheStore{Client: client}
		}
		cache = services.NewEmbeddingCache(embeddingSvc, store, embeddingSvc.Model(), cfg.GeminiEmbeddingDimension, 1000)
		spannerSvc.SetQueryEmbedder(cache)
	}
	var synStore services.SynonymStore = services.NewMemorySynonymStore()
	if client := spannerSvc.Client(); client != nil {
		synStore = services.SpannerSynonymStore{Client: client}
	}
	synonyms := services.NewSynonymService(synStore, time.Minute)
	spannerSvc.SetSynonyms(synonyms)
	var ruleStore services.RuleStore = services.NewMemoryRuleStore()
	if client := spannerSvc.Client(); client != nil {
		ruleStore = services.SpannerRuleStore{Client: client}
	}
	rules := services.NewRuleService(ruleStore, 30*time.Second)
	spannerSvc.SetRules(rules)
	var adStore services.AdStore = services.NewMemoryAdStore()
	if client := spannerSvc.Client(); client != nil {
		adStore = services.SpannerAdStore{Client: client}
	}
	ads := services.NewAdService(adStore, services.DefaultSeedProducts(), spannerSvc.Client())
	spannerSvc.SetAds(ads)
	var intentLLM services.IntentLLM
	var intentStore services.IntentStore
	if cfg.IntentLLMFallback {
		if llm, err := services.NewGeminiIntentLLM(ctx, cfg, cfg.IntentLLMModel); err != nil {
			log.Printf("WARN: intent LLM fallback disabled: %v", err)
		} else {
			intentLLM = llm
		}
		if client := spannerSvc.Client(); client != nil {
			intentStore = services.SpannerIntentStore{Client: client}
		}
	}
	vocabCtx, vocabCancel := context.WithTimeout(ctx, 5*time.Second)
	vocab := spannerSvc.CatalogVocabulary(vocabCtx)
	vocabCancel()
	intent := services.NewQueryUnderstandingService(vocab, intentLLM, intentStore)
	spell := services.NewSpellCheckService(spannerSvc.Client())
	spell.AddVocabulary(vocab, services.DefaultSuggestions(services.DefaultSeedProducts()))
	for _, syn := range services.DefaultSynonyms() {
		spell.AddVocabulary([]models.SearchResult{{Title: syn.Term + " " + strings.Join(syn.Synonyms, " ")}}, nil)
	}
	var sessionStore services.SessionStore = services.NewMemorySessionStore()
	if client := spannerSvc.Client(); client != nil {
		sessionStore = services.SpannerSessionStore{Client: client}
	}
	session := services.NewSessionService(sessionStore, spannerSvc, cfg.SessionBoostWeight)
	var analyticsStore services.AnalyticsStore = services.NewMemoryAnalyticsStore()
	if client := spannerSvc.Client(); client != nil {
		analyticsStore = services.SpannerAnalyticsStore{Client: client}
	}
	analytics := services.NewAnalyticsService(analyticsStore, spannerSvc, session)
	ltrModel := services.DefaultLTRModel()
	if cfg.LTRModelJSON != "" {
		if m, err := services.ParseLTRModel(cfg.LTRModelJSON); err != nil {
			log.Printf("WARN: invalid LTR_MODEL_JSON (%v); using baseline model", err)
		} else {
			ltrModel = m
		}
	}
	autocomplete := services.NewAutocompleteService(spannerSvc.Client(), services.DefaultSeedProducts(), 30*time.Second)
	var images imageEmbedder
	if embeddingSvc != nil {
		images = services.NewImageEmbeddingCache(embeddingSvc, 256)
	}
	return &Controller{
		images:       images,
		config:       cfg,
		spannerSvc:   spannerSvc,
		embeddingSvc: embeddingSvc,
		cache:        cache,
		synonyms:     synonyms,
		rules:        rules,
		ads:          ads,
		autocomplete: autocomplete,
		ltrModel:     ltrModel,
		intent:       intent,
		spell:        spell,
		session:      session,
		analytics:    analytics,
	}, nil
}

func (c *Controller) HealthCheck(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"status":       "healthy",
		"service":      "psearch-serving",
		"backend_mode": c.spannerSvc.BackendMode(),
	})
}

func (c *Controller) Info(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"service":                  "psearch-serving",
		"repo":                     "duboc/scion-experiment",
		"sandbox":                  c.config.ProjectID,
		"spanner_instance":         c.config.SpannerInstanceID,
		"spanner_database":         c.config.SpannerDatabaseID,
		"backend_mode":             c.spannerSvc.BackendMode(),
		"product_count":            c.spannerSvc.ProductCount(),
		"active_embedding_version": c.config.ActiveVersion(),
		"embedding_dimension":      c.config.ActiveDimension(),
		"vector_dimensions":        c.config.ActiveDimension(),
		"embedding_model":          c.config.EmbeddingModelName,
		"text_model":               c.config.IntentLLMModel,
		"pro_model":                c.config.ProLLMModel,
		"genai_location":           c.config.GenAILocation,
		"embedding_cache":          c.cacheStats(),
		"vector_index_used":        c.spannerSvc.VectorIndexUsed(),
		"use_vector_index":         c.config.EffectiveUseVectorIndex(),
		"vector_index":             "products_embedding_v2_idx",
		"ann_num_leaves_to_search": c.config.EffectiveANNNumLeaves(),
		"default_alpha":            c.config.DefaultAlpha,
		"orchestrator":             "lead",
	})
}

func (c *Controller) Search(ctx *gin.Context) {
	start := time.Now()
	var req models.SearchRequest

	if ctx.Request.Method == http.MethodGet {
		req.Query = ctx.Query("q")
		if req.Query == "" {
			req.Query = ctx.Query("query")
		}
		if v := ctx.Query("alpha"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				req.Alpha = &f
			}
		}
		if v := ctx.Query("min_score"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				req.MinScore = &f
			}
		}
		if v := ctx.Query("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
				return
			}
			req.Limit = &n
		}
		req.Category = ctx.Query("category")
		req.Categories = append(append([]string{}, ctx.QueryArray("category")...), ctx.QueryArray("categories")...)
		req.Brand = ctx.Query("brand")
		req.Brands = append(append([]string{}, ctx.QueryArray("brand")...), ctx.QueryArray("brands")...)
		req.Locale = ctx.Query("locale")
		req.TenantID = ctx.Query("tenant_id")
		req.LTR = ctx.Query("ltr") == "true" || ctx.Query("ltr") == "1"
		if v := ctx.Query("parse_intent"); v != "" {
			b := v == "true" || v == "1"
			req.ParseIntent = &b
		}
		req.PriceTable = ctx.Query("price_table_id")
		req.SortBy = ctx.Query("sort_by")
		req.Color = ctx.Query("color")
		req.Colors = append(append([]string{}, ctx.QueryArray("color")...), ctx.QueryArray("colors")...)
		req.Size = ctx.Query("size")
		req.Sizes = append(append([]string{}, ctx.QueryArray("size")...), ctx.QueryArray("sizes")...)
		req.PriceRanges = append(append([]string{}, ctx.QueryArray("price_range")...), ctx.QueryArray("price_ranges")...)
		req.SessionID = ctx.Query("session_id")
		req.ExperimentID = ctx.Query("experiment_id")
		if v := ctx.Query("session_product_ids"); v != "" {
			req.SessionProductIDs = strings.Split(v, ",")
		}
		if v, err := strconv.ParseFloat(ctx.Query("min_price"), 64); err == nil {
			req.MinPrice = v
		}
		if v, err := strconv.ParseFloat(ctx.Query("max_price"), 64); err == nil {
			req.MaxPrice = v
		}
		if v := ctx.Query("max_sla_hours"); v != "" {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				req.MaxSLAHours = n
			}
		}
		req.Channel = ctx.Query("channel")
		req.CEP = ctx.Query("cep")
		if v := ctx.Query("only_in_stock"); v == "true" || v == "1" {
			req.OnlyInStock = true
		}
		if v := strings.ToLower(strings.TrimSpace(ctx.Query("ads"))); v != "" {
			b := v != "false" && v != "0" && v != "no" && v != "off"
			req.Ads = &b
		}
		if rawPage := strings.TrimSpace(ctx.Query("page")); rawPage != "" {
			v, err := strconv.Atoi(rawPage)
			if err != nil || v < 1 {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "page must be >= 1"})
				return
			}
			req.Page = v
		}
		if rawOffset := strings.TrimSpace(ctx.Query("offset")); rawOffset != "" {
			v, err := strconv.Atoi(rawOffset)
			if err != nil || v < 0 || v > 1000 {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "offset must be between 0 and 1000"})
				return
			}
			req.Offset = v
		}
	} else {
		ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxSearchBodyBytes)
		if err := ctx.ShouldBindJSON(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large (max 8 MB; resize the image)"})
				return
			}
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	limit := c.config.DefaultLimit
	if req.Limit != nil {
		if *req.Limit < 1 || *req.Limit > 100 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
			return
		}
		limit = *req.Limit
	}
	if req.Offset < 0 || req.Offset > 1000 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "offset must be between 0 and 1000"})
		return
	}
	effectiveOffset := 0
	if req.Offset > 0 {
		effectiveOffset = req.Offset
	} else if req.Page > 1 {
		effectiveOffset = (req.Page - 1) * limit
	}
	if effectiveOffset < 0 || effectiveOffset > 1000 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "offset must be between 0 and 1000"})
		return
	}
	effectivePage := req.Page
	if req.Offset > 0 {
		effectivePage = (effectiveOffset / limit) + 1
	} else if effectivePage < 1 {
		effectivePage = 1
	}
	minScore := c.config.MinScoreValue
	if req.MinScore != nil {
		minScore = *req.MinScore
	}
	alpha := c.config.DefaultAlpha
	if req.Alpha != nil {
		alpha = *req.Alpha
	}
	if alpha < 0 {
		alpha = 0
	} else if alpha > 1 {
		alpha = 1
	}

	zip5, cepErr := services.NormalizeCEP(req.CEP)
	if cepErr != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": cepErr.Error()})
		return
	}
	if req.MaxSLAHours < 0 || req.MaxSLAHours > 24*60 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "max_sla_hours must be between 0 and 1440"})
		return
	}
	if req.MinPrice < 0 || req.MaxPrice < 0 || (req.MaxPrice > 0 && req.MinPrice > req.MaxPrice) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "min_price/max_price must be >= 0 and min_price <= max_price"})
		return
	}
	req.PriceTable, req.SortBy = strings.TrimSpace(req.PriceTable), strings.TrimSpace(req.SortBy)
	if err := services.ValidatePricing(req.PriceTable, req.SortBy); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.ExperimentID = strings.TrimSpace(req.ExperimentID)
	if err := services.ValidateExperimentID(req.ExperimentID); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Color) > 64 || len(req.Size) > 32 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "color must be <= 64 chars and size <= 32 chars"})
		return
	}
	categories, err := services.ParseAndValidateFacetSlice("categories", services.MaxFacetValueLength, []string{req.Category}, req.Categories)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	brands, err := services.ParseAndValidateFacetSlice("brands", services.MaxFacetValueLength, []string{req.Brand}, req.Brands)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	colors, err := services.ParseAndValidateFacetSlice("colors", 64, []string{req.Color}, req.Colors)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sizes, err := services.ParseAndValidateFacetSlice("sizes", 32, []string{req.Size}, req.Sizes)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	priceRanges, err := services.ValidatePriceRanges(req.PriceRanges)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	dynAttrs, err := services.ParseAndValidateDynamicAttrs(ctx.Request.URL.Query(), req.Attrs)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filters := models.SearchFilters{
		Color:       req.Color,
		Colors:      colors,
		Size:        req.Size,
		Sizes:       sizes,
		PriceTable:  req.PriceTable,
		PriceRanges: priceRanges,
		Attrs:       dynAttrs,
		SortBy:      req.SortBy,
		MinPrice:    req.MinPrice,
		MaxPrice:    req.MaxPrice,
		Zip5:        zip5,
		MaxSLAHours: req.MaxSLAHours,
		OnlyInStock: req.OnlyInStock,
		Channel:     strings.ToLower(strings.TrimSpace(req.Channel)),
		Category:    req.Category,
		Categories:  categories,
		Brand:       req.Brand,
		Brands:      brands,
		TenantID:    req.TenantID,
		Locale:      req.Locale,
	}
	// ADR-010 (Issue #33): visual search. The photo is embedded with
	// gemini-embedding-2 into the embedding_v2 space; the optional text refines it.
	var imageQuery, imageHit bool
	var imageWeight float64
	if strings.TrimSpace(req.ImageBase64) != "" {
		data, mime, ierr := services.DecodeQueryImage(req.ImageBase64, req.ImageMimeType)
		if ierr != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": ierr.Error()})
			return
		}
		imageWeight = services.DefaultImageWeight
		if req.ImageWeight != nil {
			if *req.ImageWeight <= 0 || *req.ImageWeight > 1 {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "image_weight must be in (0, 1]"})
				return
			}
			imageWeight = *req.ImageWeight
		}
		if c.images == nil || !c.config.UseGeminiEmbeddings() {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "image search requires gemini-embedding-2 (ACTIVE_EMBEDDING_VERSION=v2)"})
			return
		}
		vec, hit, eerr := c.images.EmbedImageWithMeta(ctx.Request.Context(), data, mime)
		switch {
		case eerr != nil && strings.TrimSpace(req.Query) == "":
			log.Printf("WARN: image embedding failed: %v", eerr)
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "image search temporarily unavailable"})
			return
		case eerr != nil:
			// Degrade to the text refinement alone rather than failing the request.
			log.Printf("WARN: image embedding failed (%v); text-only search", eerr)
		default:
			imageQuery, imageHit = true, hit
			filters.ImageVector, filters.ImageWeight = vec, imageWeight
			if req.Alpha == nil {
				alpha = services.DefaultMultimodalAlpha
			}
		}
	}
	// ADR-007 (Issue #19): decompose natural-language queries into a clean query
	// plus structured filters; explicit request filters win.
	var parsed *models.ParsedQuery
	searchQuery := req.Query
	// Enabled by default; parse_intent=false opts out. When nothing is extracted the
	// query is left untouched and parsed_query is omitted (unless explicitly requested).
	explicitIntent := req.ParseIntent != nil && *req.ParseIntent
	if (req.ParseIntent == nil || *req.ParseIntent) && strings.TrimSpace(req.Query) != "" {
		in, source := c.intent.Understand(ctx.Request.Context(), filters.TenantID, req.Query)
		if !in.HasFilters() && !explicitIntent {
			goto intentDone
		}
		filters = services.ApplyIntent(filters, in)
		searchQuery = in.CleanQuery
		extracted := map[string]interface{}{}
		if in.MinPrice > 0 {
			extracted["min_price"] = in.MinPrice
		}
		if in.MaxPrice > 0 {
			extracted["max_price"] = in.MaxPrice
		}
		if in.Brand != "" {
			extracted["brand"] = in.Brand
		}
		if in.Category != "" {
			extracted["category"] = in.Category
		}
		if in.Color != "" {
			extracted["color"] = in.Color
		}
		parsed = &models.ParsedQuery{OriginalQuery: req.Query, CleanQuery: in.CleanQuery, ExtractedFilters: extracted, IntentSource: source}
	}
intentDone:
	// Facets are aggregated over top facetCandidates (100), and Stage-1 retrieves
	// the page-independent fixed window W = min(max(lexical_count, 100), maxSearchWindow=500)
	// on every page so RRF scores and candidate ordering never shift across pages (ADR-012, #40 fix-3).
	const facetCandidates = 100
	const maxSearchWindow = 500
	candidateLimit := facetCandidates
	outcome, err := c.spannerSvc.HybridSearchDetailed(ctx.Request.Context(), searchQuery, candidateLimit, minScore, alpha, filters)
	// ADR-008 (Issue #21): zero-results recovery when the lexical leg found nothing.
	var didYouMean string
	var autoCorrected, relaxed bool
	if err == nil && !services.IsBrowseQuery(searchQuery) && services.LexicalHits(outcome.Results) == 0 {
		if corrected, changed := c.spell.Correct(ctx.Request.Context(), filters.TenantID, searchQuery); changed {
			didYouMean = corrected
			// The corrected token may be an entity that is only indexed as a structured
			// column (e.g. brand "samsung"), not in title/description tokens. Re-parse
			// the corrected query so such entities become filters (ADR-007/008).
			altQuery, altFilters := corrected, filters
			entity := false
			if in := c.intent.Parse(corrected); in.Brand != "" || in.Category != "" {
				altQuery, altFilters, entity = in.CleanQuery, services.ApplyIntent(filters, in), true
			}
			if alt, aerr := c.spannerSvc.HybridSearchDetailed(ctx.Request.Context(), altQuery, candidateLimit, minScore, alpha, altFilters); aerr == nil &&
				(services.LexicalHits(alt.Results) > 0 || (entity && len(alt.Results) > 0)) {
				outcome, searchQuery, filters, autoCorrected = alt, altQuery, altFilters, true
			}
		}
		if !autoCorrected && len(strings.Fields(searchQuery)) > 1 {
			rf := filters
			rf.RelaxText = true
			if alt, aerr := c.spannerSvc.HybridSearchDetailed(ctx.Request.Context(), searchQuery, candidateLimit, minScore, alpha, rf); aerr == nil && services.LexicalHits(alt.Results) > 0 {
				outcome, relaxed = alt, true
			}
		}
	}
	results := outcome.Results
	if results == nil {
		results = []models.SearchResult{} // always serialize as [] (never null)
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if outcome.RetrievalMode == services.RetrievalLexicalOnly {
		alpha = 0 // vector leg was not executed
	}
	if outcome.RetrievalMode != services.RetrievalMultimodal {
		imageQuery, imageHit, imageWeight = false, false, 0
	} else if services.IsBrowseQuery(searchQuery) {
		alpha, imageWeight = 1, 1 // image-only: vector leg alone
	}
	organicTotal := outcome.TotalFound
	if organicTotal < len(results) {
		organicTotal = len(results)
	}
	ids := make([]string, 0, len(results))
	for _, r := range results {
		ids = append(ids, r.ID)
	}
	facetIDs := ids
	if len(facetIDs) > facetCandidates {
		facetIDs = facetIDs[:facetCandidates]
	}
	facetStart := time.Now()
	facets, ferr := c.spannerSvc.DisjunctiveFacets(ctx.Request.Context(), searchQuery, minScore, alpha, filters, facetIDs)
	facetsMs := float64(time.Since(facetStart).Microseconds()) / 1000.0
	if ferr != nil {
		log.Printf("WARN: facets unavailable: %v", ferr)
		facets, facetsMs = nil, 0
	}
	candidates := len(facetIDs)

	attachPromises := func(target []models.SearchResult, includeSponsored bool) {
		if zip5 == "" || (len(target) == 0 && (!includeSponsored || len(outcome.Sponsored) == 0)) {
			return
		}
		pageIDs := make([]string, 0, len(target)+len(outcome.Sponsored))
		for _, r := range target {
			pageIDs = append(pageIDs, r.ID)
		}
		if includeSponsored {
			for _, r := range outcome.Sponsored {
				pageIDs = append(pageIDs, r.ID)
			}
		}
		if promises, err := c.spannerSvc.DeliveryPromises(ctx.Request.Context(), pageIDs, zip5, req.MaxSLAHours); err != nil {
			log.Printf("WARN: delivery promises unavailable: %v", err)
		} else {
			for i := range target {
				if p, ok := promises[target[i].ID]; ok {
					cp := *p
					cp.CEP = req.CEP
					target[i].DeliveryPromise = &cp
				}
			}
			if includeSponsored {
				for i := range outcome.Sponsored {
					if p, ok := promises[outcome.Sponsored[i].ID]; ok {
						cp := *p
						cp.CEP = req.CEP
						outcome.Sponsored[i].DeliveryPromise = &cp
					}
				}
			}
		}
	}
	// Delivery promises are attached to the whole candidate set when LTR needs the
	// sla_hours feature; otherwise only to the returned page after slicing.
	if req.LTR {
		attachPromises(results, true)
	}
	var ltrRes services.LTRResult
	explicitNonRelSort := req.SortBy == "price_asc" || req.SortBy == "price_desc" || req.SortBy == "popularity" || req.SortBy == "rating_desc"
	if req.LTR && len(results) > 0 && !explicitNonRelSort { // an explicit sort wins over LTR
		// ADR-006 stage 2: in-memory re-ranking of the stage-1 candidate window.
		signals, err := c.spannerSvc.CandidateSignals(ctx.Request.Context(), ids)
		if err != nil {
			log.Printf("WARN: ltr signals unavailable (re-ranking on relevance features only): %v", err)
		}
		ltrRes = services.Rerank(c.ltrModel, results, signals, outcome.Pins, outcome.Buries)
	}
	// ADR-007 (Issue #20): session-aware re-ranking by the session centroid.
	personalized := false
	if req.SessionID != "" || len(req.SessionProductIDs) > 0 {
		var events []services.SessionEvent
		if req.SessionID != "" && services.ValidSessionID(req.SessionID) {
			if ev, err := c.session.Recent(ctx.Request.Context(), filters.TenantID, req.SessionID); err != nil {
				log.Printf("WARN: session events unavailable: %v", err)
			} else {
				events = ev
			}
		}
		for i, id := range req.SessionProductIDs {
			if i >= 20 {
				break
			}
			if id = strings.TrimSpace(id); id != "" {
				events = append(events, services.SessionEvent{ProductID: id, EventType: "click"})
			}
		}
		personalized = c.session.Personalize(ctx.Request.Context(), filters.TenantID, events, results, outcome.Pins, outcome.Buries, req.SortBy, ltrRes.ModelVersion != "")
	}
	// ADR-012 (Issue #40 fix-2): results is already truncated to the fixed per-query
	// window W = min(max(lexical_count, 100), 500). Reachable window is min(total_found, 500),
	// and has_next = effectiveOffset + limit < min(reachable, len(truncated)).
	// ADR-012 (Issue #40 fix-3): results is the page-independent fixed window
	// W = min(max(lexical_count, 100), 500). Reachable window is min(total_found, 500),
	// and has_next = effectiveOffset + limit < min(reachable, len(results)).
	reachableTotal := organicTotal
	if reachableTotal > maxSearchWindow {
		reachableTotal = maxSearchWindow
	}
	reachableBound := reachableTotal
	if len(results) < reachableBound {
		reachableBound = len(results)
	}
	if effectiveOffset >= reachableBound {
		results = []models.SearchResult{}
	} else {
		end := effectiveOffset + limit
		if end > reachableBound {
			end = reachableBound
		}
		results = results[effectiveOffset:end]
	}
	if !req.LTR {
		attachPromises(results, true)
	}
	totalPages := 0
	if reachableTotal > 0 && limit > 0 {
		totalPages = (reachableTotal + limit - 1) / limit
	}
	hasNext := effectiveOffset+limit < reachableBound
	// ADR-011 (Issue #35): Sponsored product injection as the very last step of
	// Controller.Search, after LTR, session re-rank, and pagination.
	// Ads are kept out of facets, total_found, LTR features, and the session centroid.
	// Zero organic results => zero ads; page 1 (effectiveOffset == 0) only; non-browse queries only; ads=false disables.
	adsEnabled := req.Ads == nil || *req.Ads
	isFirstPage := effectiveOffset == 0
	hasQuery := !services.IsBrowseQuery(searchQuery) || imageQuery
	sponsoredCount := 0
	if adsEnabled && isFirstPage && hasQuery && len(results) > 0 && len(outcome.Sponsored) > 0 {
		results = services.InjectSponsored(results, outcome.Sponsored, c.config.EffectiveAdSlots())
		for _, r := range results {
			if r.Sponsored {
				sponsoredCount++
			}
		}
	}
	// Enrich the final returned page (including any injected sponsored items) with DynamicAttributes (#46 fix).
	c.spannerSvc.EnrichResultsWithAttributes(ctx.Request.Context(), results)
	tenant := strings.TrimSpace(req.TenantID)
	if tenant == "" {
		tenant = "default"
	}
	latencyMs := float64(time.Since(start).Microseconds()) / 1000.0
	queryID := services.NewQueryID()
	if c.analytics != nil {
		organicIDs := make([]string, 0, len(results))
		for _, r := range results {
			if !r.Sponsored {
				organicIDs = append(organicIDs, r.ID)
			}
		}
		impTotal := int64(organicTotal)
		if !services.IsBrowseQuery(searchQuery) && !imageQuery && !autoCorrected && !relaxed &&
			filters.Category == "" && len(filters.Categories) == 0 && filters.Brand == "" && len(filters.Brands) == 0 &&
			services.LexicalHits(outcome.Results) == 0 {
			impTotal = 0
		}
		c.analytics.EnqueueImpression(services.SearchImpression{
			ImpressionID:  queryID,
			TenantID:      tenant,
			QueryRaw:      req.Query,
			QueryNorm:     services.NormalizeQueryForAnalytics(req.Query),
			ExperimentID:  req.ExperimentID,
			RetrievalMode: outcome.RetrievalMode,
			TotalFound:    impTotal,
			LatencyMs:     latencyMs,
			ResultIDs:     organicIDs,
		})
	}

	ctx.JSON(http.StatusOK, models.SearchResponse{
		Query:            req.Query,
		QueryID:          queryID,
		ExperimentID:     req.ExperimentID,
		EffectiveAlpha:   alpha,
		Results:          results,
		TotalFound:       organicTotal,
		Page:             effectivePage,
		PageSize:         limit,
		Offset:           effectiveOffset,
		TotalPages:       totalPages,
		HasNext:          hasNext,
		SponsoredCount:   sponsoredCount,
		VectorIndexUsed:  outcome.VectorIndexUsed,
		Candidates:       candidates,
		Facets:           facets,
		FacetsLatencyMs:  facetsMs,
		TenantID:         tenant,
		Locale:           strings.TrimSpace(req.Locale),
		LatencyMs:        latencyMs,
		BackendMode:      c.spannerSvc.BackendMode(),
		EmbeddingVersion: c.config.ActiveVersion(),
		CacheHit:         outcome.CacheHit,
		ExpandedTerms:    outcome.ExpandedTerms,
		AppliedRules:     outcome.AppliedRules,
		LTR:              req.LTR && ltrRes.ModelVersion != "",
		LTRLatencyUS:     ltrRes.LatencyUS,
		LTRModel:         ltrRes.ModelVersion,
		ParsedQuery:      parsed,
		DidYouMean:       didYouMean,
		AutoCorrected:    autoCorrected,
		QueryRelaxed:     relaxed,
		PriceTableID:     req.PriceTable,
		SortBy:           req.SortBy,
		SessionID:        req.SessionID,
		Personalized:     personalized,
		RetrievalMode:    outcome.RetrievalMode,
		ImageQuery:       imageQuery,
		ImageWeight:      imageWeight,
		ImageCacheHit:    imageHit,
	})
}

func (c *Controller) cacheStats() interface{} {
	if c.cache == nil {
		return gin.H{"enabled": false}
	}
	st := c.cache.Stats()
	return gin.H{"enabled": true, "memory_hits": st.MemoryHits, "spanner_hits": st.SpannerHits,
		"misses": st.Misses, "entries": st.Entries, "max_entries": st.MaxEntries}
}

// GetEmbeddingVersion: GET /api/config/embedding-version (ADR-001 Blue/Green).
func (c *Controller) GetEmbeddingVersion(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{
		"active_embedding_version": c.config.ActiveVersion(),
		"vector_dimensions":        c.config.ActiveDimension(),
		"available":                []string{"v1", "v2"},
		"scope":                    "instance",
	})
}

// SetEmbeddingVersion: POST /api/config/embedding-version {"version":"v1"|"v2"}.
// Requires X-Admin-Token == ADMIN_TOKEN; disabled (403) when ADMIN_TOKEN is unset.
// The switch is per Cloud Run instance; set ACTIVE_EMBEDDING_VERSION for a fleet-wide change.
func (c *Controller) SetEmbeddingVersion(ctx *gin.Context) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := c.config.SetActiveVersion(body.Version); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Printf("config: active embedding version switched to %s (this instance)", body.Version)
	c.GetEmbeddingVersion(ctx)
}

// authorizeAdmin enforces X-Admin-Token == ADMIN_TOKEN (constant-time); writes 403 otherwise.
// Admin writes are disabled entirely when ADMIN_TOKEN is unset.
func (c *Controller) authorizeAdmin(ctx *gin.Context) bool {
	token := ctx.GetHeader("X-Admin-Token")
	if c.config.AdminToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(c.config.AdminToken)) != 1 {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return false
	}
	return true
}

// ListSynonyms: GET /api/synonyms?tenant_id=default&locale=pt-BR (Issue #7).
func (c *Controller) ListSynonyms(ctx *gin.Context) {
	tenant := strings.TrimSpace(ctx.Query("tenant_id"))
	if tenant == "" {
		tenant = "default"
	}
	rows, err := c.synonyms.List(ctx.Request.Context(), tenant, strings.TrimSpace(ctx.Query("locale")))
	if err != nil {
		log.Printf("WARN: list synonyms: %v", err)
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "synonyms unavailable"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tenant_id": tenant, "synonyms": rows, "count": len(rows)})
}

// UpsertSynonym: POST /api/synonyms {tenant_id, locale, term, synonyms[], weight} (admin only).
func (c *Controller) UpsertSynonym(ctx *gin.Context) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	var body services.Synonym
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	saved, err := c.synonyms.Upsert(ctx.Request.Context(), body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, saved)
}

func tenantParam(ctx *gin.Context) string {
	if t := strings.TrimSpace(ctx.Query("tenant_id")); t != "" {
		return t
	}
	return "default"
}

// ListRules: GET /api/rules?tenant_id=default (Issue #6). Public read.
func (c *Controller) ListRules(ctx *gin.Context) {
	tenant := tenantParam(ctx)
	rules, err := c.rules.List(ctx.Request.Context(), tenant)
	if err != nil {
		log.Printf("WARN: list rules: %v", err)
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "rules unavailable"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tenant_id": tenant, "rules": rules, "count": len(rules)})
}

// CreateRule: POST /api/rules (admin). PUT /api/rules/:id (admin) replaces a rule.
func (c *Controller) CreateRule(ctx *gin.Context) { c.saveRule(ctx, "") }

func (c *Controller) UpdateRule(ctx *gin.Context) { c.saveRule(ctx, ctx.Param("id")) }

func (c *Controller) saveRule(ctx *gin.Context, id string) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	var r services.Rule
	if err := ctx.ShouldBindJSON(&r); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id != "" {
		r.RuleID = id
	} else {
		r.RuleID = "" // server-assigned on create
	}
	if r.TenantID == "" {
		r.TenantID = tenantParam(ctx)
	}
	saved, err := c.rules.Save(ctx.Request.Context(), r)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	ctx.JSON(status, saved)
}

// DeleteRule: DELETE /api/rules/:id?tenant_id=default (admin).
func (c *Controller) DeleteRule(ctx *gin.Context) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	if err := c.rules.Delete(ctx.Request.Context(), tenantParam(ctx), ctx.Param("id")); err != nil {
		log.Printf("WARN: delete rule: %v", err)
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "delete failed"})
		return
	}
	ctx.Status(http.StatusNoContent)
}

// ListAdCampaigns: GET /api/ads/campaigns?tenant_id=default (Issue #35, ADR-011). Public read.
func (c *Controller) ListAdCampaigns(ctx *gin.Context) {
	tenant := tenantParam(ctx)
	campaigns, err := c.ads.List(ctx.Request.Context(), tenant)
	if err != nil {
		log.Printf("WARN: list ad campaigns: %v", err)
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "ad campaigns unavailable"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tenant_id": tenant, "campaigns": campaigns, "count": len(campaigns)})
}

// CreateAdCampaign: POST /api/ads/campaigns (admin).
func (c *Controller) CreateAdCampaign(ctx *gin.Context) { c.saveAdCampaign(ctx, "") }

// UpdateAdCampaign: PUT /api/ads/campaigns/:id (admin).
func (c *Controller) UpdateAdCampaign(ctx *gin.Context) { c.saveAdCampaign(ctx, ctx.Param("id")) }

func (c *Controller) saveAdCampaign(ctx *gin.Context, id string) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	var camp models.AdCampaign
	if err := ctx.ShouldBindJSON(&camp); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if id != "" {
		camp.CampaignID = id
	}
	if camp.TenantID == "" {
		camp.TenantID = tenantParam(ctx)
	}
	saved, err := c.ads.Save(ctx.Request.Context(), camp)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	ctx.JSON(status, saved)
}

// DeleteAdCampaign: DELETE /api/ads/campaigns/:id?tenant_id=default (admin).
func (c *Controller) DeleteAdCampaign(ctx *gin.Context) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	if err := c.ads.Delete(ctx.Request.Context(), tenantParam(ctx), ctx.Param("id")); err != nil {
		log.Printf("WARN: delete ad campaign: %v", err)
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "delete failed"})
		return
	}
	ctx.Status(http.StatusNoContent)
}

// RecordAdClick: POST /api/ads/click {campaign_id, click_id} (Issue #35, ADR-011).
func (c *Controller) RecordAdClick(ctx *gin.Context) {
	var body models.AdClickRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	res, err := c.ads.RecordClick(ctx.Request.Context(), body.CampaignID, body.ClickID)
	if err != nil {
		if errors.Is(err, services.ErrCampaignNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Printf("WARN: record ad click: %v", err)
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "click not recorded"})
		return
	}
	ctx.JSON(http.StatusOK, res)
}

// Autocomplete: GET /api/autocomplete?q=<prefix>&locale=&tenant_id=&limit= (Issue #9, ADR-006).
func (c *Controller) Autocomplete(ctx *gin.Context) {
	start := time.Now()
	q := ctx.Query("q")
	if len(q) > 100 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "q too long (max 100)"})
		return
	}
	limit := 8
	if v, err := strconv.Atoi(ctx.Query("limit")); err == nil && v > 0 {
		limit = v
	}
	if limit > 20 {
		limit = 20
	}
	sugg, cached, err := c.autocomplete.Suggest(ctx.Request.Context(), tenantParam(ctx), strings.TrimSpace(ctx.Query("locale")), q, limit)
	if err != nil {
		log.Printf("WARN: autocomplete: %v", err)
		sugg = []services.Suggestion{} // degrade to no suggestions, never 5xx on typeahead
	}
	ctx.JSON(http.StatusOK, gin.H{
		"query":       q,
		"suggestions": sugg,
		"cached":      cached,
		"latency_ms":  float64(time.Since(start).Microseconds()) / 1000.0,
	})
}

// RecordSessionEvent: POST /api/session/events {session_id, product_id, event_type, tenant_id?} (Issue #20).
func (c *Controller) RecordSessionEvent(ctx *gin.Context) {
	var e services.SessionEvent
	if err := ctx.ShouldBindJSON(&e); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if e.TenantID == "" {
		e.TenantID = tenantParam(ctx)
	}
	saved, err := c.session.Record(ctx.Request.Context(), e)
	if err != nil {
		if strings.Contains(err.Error(), "must") || strings.Contains(err.Error(), "required") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			log.Printf("WARN: record session event: %v", err)
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "event not recorded"})
		}
		return
	}
	ctx.JSON(http.StatusCreated, saved)
}

// ListSessionEvents: GET /api/session/events?session_id=... (newest first, last 20).
func (c *Controller) ListSessionEvents(ctx *gin.Context) {
	sid := ctx.Query("session_id")
	if !services.ValidSessionID(sid) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}
	events, err := c.session.Recent(ctx.Request.Context(), tenantParam(ctx), sid)
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "session unavailable"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"session_id": sid, "events": events, "count": len(events)})
}

// UpsertProductAttributes: PUT /api/products/:id/attributes (Issue #42, ADR-014; admin only).
func (c *Controller) UpsertProductAttributes(ctx *gin.Context) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	productID := strings.TrimSpace(ctx.Param("id"))
	if productID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "product id is required"})
		return
	}
	var raw map[string]interface{}
	if err := ctx.ShouldBindJSON(&raw); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tenant := tenantParam(ctx)
	if t, ok := raw["tenant_id"].(string); ok && strings.TrimSpace(t) != "" {
		tenant = strings.TrimSpace(t)
		delete(raw, "tenant_id")
	}
	saved, err := c.spannerSvc.UpsertProductAttributes(ctx.Request.Context(), tenant, productID, raw)
	if err != nil {
		if errors.Is(err, services.ErrProductNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"product_id": productID,
		"tenant_id":  tenant,
		"attributes": saved,
		"count":      len(saved),
	})
}

// SimilarProducts: GET /api/products/:id/similar?mode=similar|substitutes|co_viewed&limit=6&... (Issue #44, ADR-016).
func (c *Controller) SimilarProducts(ctx *gin.Context) {
	seedID := strings.TrimSpace(ctx.Param("id"))
	if seedID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "product id is required"})
		return
	}
	mode, err := services.ValidateSimilarMode(ctx.Query("mode"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	limit := services.DefaultSimilarLimit
	if rawLimit := strings.TrimSpace(ctx.Query("limit")); rawLimit != "" {
		v, err := strconv.Atoi(rawLimit)
		if err != nil || v < 1 || v > services.MaxSimilarLimit {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 24"})
			return
		}
		limit = v
	}
	onlyInStock := mode == services.SimilarModeSubstitutes
	if v := strings.ToLower(strings.TrimSpace(ctx.Query("only_in_stock"))); v != "" {
		onlyInStock = v == "true" || v == "1"
	}
	if mode == services.SimilarModeSubstitutes {
		onlyInStock = true
	}

	resp, err := c.spannerSvc.SimilarProducts(ctx.Request.Context(), c.session, services.SimilarOptions{
		SeedID:      seedID,
		Mode:        mode,
		Limit:       limit,
		TenantID:    tenantParam(ctx),
		Locale:      strings.TrimSpace(ctx.Query("locale")),
		Channel:     strings.TrimSpace(ctx.Query("channel")),
		PriceTable:  strings.TrimSpace(ctx.Query("price_table_id")),
		CEP:         strings.TrimSpace(ctx.Query("cep")),
		OnlyInStock: onlyInStock,
	})
	if err != nil {
		if errors.Is(err, services.ErrProductNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

// RecordAnalyticsEvent: POST /api/analytics/events (Issue #43, ADR-015).
func (c *Controller) RecordAnalyticsEvent(ctx *gin.Context) {
	var body models.AnalyticsEventRequest
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.TenantID == "" {
		body.TenantID = tenantParam(ctx)
	}
	res, err := c.analytics.RecordEvent(ctx.Request.Context(), body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, res)
}

// AnalyticsSummary: GET /api/analytics/summary?tenant_id=default&window_hours=168 (Issue #43, ADR-015).
func (c *Controller) AnalyticsSummary(ctx *gin.Context) {
	windowHours := 168
	if raw := strings.TrimSpace(ctx.Query("window_hours")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 2160 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "window_hours must be between 1 and 2160"})
			return
		}
		windowHours = v
	}
	sum, err := c.analytics.Summary(ctx.Request.Context(), tenantParam(ctx), windowHours)
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "analytics summary unavailable"})
		return
	}
	ctx.JSON(http.StatusOK, sum)
}

// RollupAnalytics: POST /api/analytics/rollup (Issue #43/#47, ADR-015; admin only).
func (c *Controller) RollupAnalytics(ctx *gin.Context) {
	if !c.authorizeAdmin(ctx) {
		return
	}
	opts := services.RollupOptions{
		TenantID:    tenantParam(ctx),
		WindowDays:  30,
		PriorWeight: services.DefaultRollupPriorK,
		DryRun:      ctx.Query("dry_run") == "true" || ctx.Query("dry_run") == "1",
	}
	if raw := strings.TrimSpace(ctx.Query("window_days")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > 90 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "window_days must be an integer between 1 and 90"})
			return
		}
		opts.WindowDays = v
	}
	for _, qp := range []string{"prior_weight", "k"} {
		if raw := strings.TrimSpace(ctx.Query(qp)); raw != "" {
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil || services.ValidateRollupParams(opts.WindowDays, v) != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "prior_weight (k) must be a finite number in (0, 10000]"})
				return
			}
			opts.PriorWeight = v
		}
	}
	var body struct {
		TenantID    string   `json:"tenant_id"`
		WindowDays  *int     `json:"window_days"`
		PriorWeight *float64 `json:"prior_weight"`
		K           *float64 `json:"k"`
		DryRun      *bool    `json:"dry_run"`
	}
	if ctx.Request.Body != nil && ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&body); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if body.WindowDays != nil {
		if *body.WindowDays < 1 || *body.WindowDays > 90 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "window_days must be between 1 and 90"})
			return
		}
		opts.WindowDays = *body.WindowDays
	}
	if body.PriorWeight != nil {
		if services.ValidateRollupParams(opts.WindowDays, *body.PriorWeight) != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "prior_weight (k) must be a finite number in (0, 10000]"})
			return
		}
		opts.PriorWeight = *body.PriorWeight
	} else if body.K != nil {
		if services.ValidateRollupParams(opts.WindowDays, *body.K) != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "k must be a finite number in (0, 10000]"})
			return
		}
		opts.PriorWeight = *body.K
	}
	if body.DryRun != nil {
		opts.DryRun = *body.DryRun
	}
	if t := strings.TrimSpace(body.TenantID); t != "" {
		opts.TenantID = t
	}
	res, err := c.analytics.RollupProductSignalsWithOptions(ctx.Request.Context(), opts)
	if err != nil {
		if errors.Is(err, services.ErrInvalidRollupParam) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, res)
}
