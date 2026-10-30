package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
	"psearch/serving-go/internal/config"
	"psearch/serving-go/internal/models"
)

// Item-to-item similarity, in-stock substitutes & session co-visitation (ADR-016, Issue #44).

const (
	SimilarModeSimilar     = "similar"
	SimilarModeSubstitutes = "substitutes"
	SimilarModeCoViewed    = "co_viewed"
	DefaultSimilarLimit    = 6
	MaxSimilarLimit        = 24
)

// SimilarOptions configures GET /api/products/:id/similar (ADR-016, Issue #44).
type SimilarOptions struct {
	SeedID      string
	Mode        string
	Limit       int
	TenantID    string
	Locale      string
	Channel     string
	PriceTable  string
	CEP         string
	OnlyInStock bool
}

// ValidateSimilarMode validates and normalizes the recommendation mode (`similar` | `substitutes` | `co_viewed`).
func ValidateSimilarMode(mode string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(mode))
	switch m {
	case "", SimilarModeSimilar:
		return SimilarModeSimilar, nil
	case SimilarModeSubstitutes, SimilarModeCoViewed:
		return m, nil
	default:
		return "", fmt.Errorf("invalid mode %q (want similar | substitutes | co_viewed)", mode)
	}
}

type seedProductInfo struct {
	ProductID string
	Title     string
	Category  string
	Brand     string
	Price     float64
	VecV1     []float64
	VecV2     []float32
}

// buildSimilarVectorSQL renders the item-to-item kNN query against Cloud Spanner.
// When useANN is true on embedding_v2, the inner forced-index scan uses ONLY STORING
// predicates (annStoringFilterSQL) and over-fetches (@ann_limit), while the outer join
// on products AS p applies the full catalogFilterForAlias("p") (ADR-012 / ADR-016).
func buildSimilarVectorSQL(column string, useANN bool, numLeaves int) string {
	if numLeaves <= 0 {
		numLeaves = 20
	}
	if useANN && column == vectorColumnV2 {
		return optimizerHint + `
			SELECT v.distance, p.product_id, p.title, p.product_data, p.is_available, p.stock,
				` + effectivePriceSQL("p") + ` AS effective_price,
				` + variantsArraySQL("p") + ` AS variants
			FROM (
				SELECT product_id,
					APPROX_COSINE_DISTANCE(` + column + `, @seed_embedding, options => JSON '{"num_leaves_to_search": ` + strconv.Itoa(numLeaves) + `}') AS distance
				FROM products@{FORCE_INDEX=products_embedding_v2_idx}
				WHERE ` + column + ` IS NOT NULL
					AND product_id != @seed_id
					AND ` + annStoringFilterSQL + `
				ORDER BY distance ASC
				LIMIT @ann_limit
			) AS v
			JOIN products AS p ON p.product_id = v.product_id
			WHERE p.product_id != @seed_id
				AND ` + catalogFilterForAlias("p") + `
			ORDER BY v.distance ASC, p.product_id
			LIMIT @limit`
	}
	return optimizerHint + `
		SELECT COSINE_DISTANCE(products.` + column + `, @seed_embedding) AS distance,
			products.product_id, products.title, products.product_data, products.is_available, products.stock,
			` + effectivePriceSQL("products") + ` AS effective_price,
			` + variantsArraySQL("products") + ` AS variants
		FROM products
		WHERE products.` + column + ` IS NOT NULL
			AND products.product_id != @seed_id
			AND ` + catalogFilterSQL + `
		ORDER BY distance ASC, products.product_id
		LIMIT @limit`
}

func (s *SpannerService) lookupSeedProduct(ctx context.Context, seedID, tenantID, priceTable string) (seedProductInfo, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	if s.client == nil {
		for _, p := range s.seedProducts {
			pTenant := p.TenantID
			if pTenant == "" {
				pTenant = "default"
			}
			if p.ID == seedID && pTenant == tenantID {
				if !s.memPricing().visible(priceTable, p.ID) {
					return seedProductInfo{}, ErrProductNotFound
				}
				eff := s.memPricing().effective(priceTable, p.ID, p.PriceInfo.NumericPrice)
				cat := firstOr(p.Categories, "")
				brand := firstOr(p.Brands, "")
				var v1 []float64
				s.attrMu.RLock()
				hasNullOverride := s.nullVectorIDs != nil && s.nullVectorIDs[p.ID]
				s.attrMu.RUnlock()
				if !hasNullOverride {
					docText := FoldAccents(strings.ToLower(p.Title + " " + p.Description + " " + strings.Join(p.Brands, " ") + " " + strings.Join(p.Categories, " ")))
					v1 = DeterministicEmbedding(docText, config.LegacyEmbeddingDimension)
				}
				return seedProductInfo{
					ProductID: p.ID,
					Title:     p.Title,
					Category:  cat,
					Brand:     brand,
					Price:     eff,
					VecV1:     v1,
				}, nil
			}
		}
		return seedProductInfo{}, ErrProductNotFound
	}

	stmt := spanner.Statement{
		SQL: `SELECT product_id, IFNULL(title, ''), IFNULL(category, ''), IFNULL(brand, ''),
				IFNULL(` + effectivePriceSQL("products") + `, IFNULL(price, 0.0)) AS eff_price,
				embedding, embedding_v2
			FROM products
			WHERE product_id = @seed_id
				AND COALESCE(is_visible, TRUE)
				AND IFNULL(products.tenant_id, 'default') = @tenant_id
				AND ` + assortmentSQL,
		Params: map[string]interface{}{
			"seed_id":     seedID,
			"tenant_id":   tenantID,
			"price_table": priceTable,
		},
	}
	iter := s.client.Single().Query(ctx, stmt)
	defer iter.Stop()
	row, err := iter.Next()
	if err == iterator.Done {
		return seedProductInfo{}, ErrProductNotFound
	}
	if err != nil {
		return seedProductInfo{}, err
	}
	var info seedProductInfo
	if err := row.Columns(&info.ProductID, &info.Title, &info.Category, &info.Brand, &info.Price, &info.VecV1, &info.VecV2); err != nil {
		return seedProductInfo{}, err
	}
	return info, nil
}

// SetNullEmbeddingForTest marks a product ID as having a NULL vector in memory mode
// to test the ADR-016 lexical token fallback.
func (s *SpannerService) SetNullEmbeddingForTest(productID string, isNull bool) {
	s.attrMu.Lock()
	defer s.attrMu.Unlock()
	if s.nullVectorIDs == nil {
		s.nullVectorIDs = map[string]bool{}
	}
	if isNull {
		s.nullVectorIDs[productID] = true
	} else {
		delete(s.nullVectorIDs, productID)
	}
}

func (s *SpannerService) querySimilarVectorSpanner(ctx context.Context, seed seedProductInfo, filters models.SearchFilters, limit int) ([]models.SearchResult, map[string]float64, bool, error) {
	col := vectorColumnV1
	var seedEmb interface{} = seed.VecV1
	if s.config.UseGeminiEmbeddings() {
		col = vectorColumnV2
		seedEmb = seed.VecV2
	}
	// Lexical fallback when seed embedding is NULL (e.g. newly ingested SKU before backfill; ADR-016).
	if (col == vectorColumnV2 && len(seed.VecV2) == 0) || (col == vectorColumnV1 && len(seed.VecV1) == 0) {
		lexFilters := filters
		lexFilters.RelaxText = true
		lexQuery := strings.TrimSpace(seed.Title + " " + seed.Category)
		out, err := s.HybridSearchDetailed(ctx, lexQuery, limit+5, 0, 0, lexFilters)
		if err != nil {
			return nil, nil, false, err
		}
		var res []models.SearchResult
		sims := map[string]float64{}
		for idx, item := range out.Results {
			if item.ID == seed.ProductID {
				continue
			}
			sims[item.ID] = math.Max(0.1, 1.0-float64(idx)*0.06)
			res = append(res, item)
		}
		return res, sims, false, nil
	}

	f := normalizeFilters(filters)
	annLimit := (limit + 5) * 3
	if annLimit < 60 {
		annLimit = 60
	}
	if annLimit > 500 {
		annLimit = 500
	}
	params := map[string]interface{}{
		"seed_id":        seed.ProductID,
		"seed_embedding": seedEmb,
		"tenant_id":      f.TenantID,
		"locale":         f.Locale,
		"category":       f.Category,
		"categories":     nonNilStrings(f.Categories),
		"brand":          f.Brand,
		"brands":         nonNilStrings(f.Brands),
		"zip5":           f.Zip5,
		"min_price":      f.MinPrice,
		"max_price":      f.MaxPrice,
		"price_table":    f.PriceTable,
		"price_ranges":   nonNilStrings(f.PriceRanges),
		"color":          f.Color,
		"colors":         nonNilStrings(f.Colors),
		"size":           f.Size,
		"sizes":          nonNilStrings(f.Sizes),
		"max_sla":        f.MaxSLAHours,
		"channel":        f.Channel,
		"only_available": f.OnlyInStock,
		"limit":          int64(limit + 8),
		"ann_limit":      int64(annLimit),
	}

	runQuery := func(useANN bool) ([]models.SearchResult, map[string]float64, error) {
		sql := specializeSQLForFilters(buildSimilarVectorSQL(col, useANN, s.config.EffectiveANNNumLeaves()), f)
		iter := s.client.Single().Query(ctx, spanner.Statement{SQL: sql, Params: params})
		defer iter.Stop()
		var list []models.SearchResult
		sims := map[string]float64{}
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, nil, err
			}
			var dist float64
			var pid, title string
			var pdata spanner.NullJSON
			var isAvail spanner.NullBool
			var stock spanner.NullInt64
			var effPrice spanner.NullFloat64
			var variants []*variantRow
			if err := row.Columns(&dist, &pid, &title, &pdata, &isAvail, &stock, &effPrice, &variants); err != nil {
				return nil, nil, err
			}
			var item models.SearchResult
			if pdata.Valid {
				raw, _ := json.Marshal(pdata.Value)
				_ = json.Unmarshal(raw, &item)
			}
			if item.ID == "" {
				item.ID = pid
			}
			if item.Title == "" {
				item.Title = title
			}
			if stock.Valid {
				q := int(stock.Int64)
				item.AvailableQuantity = &q
			}
			if isAvail.Valid || stock.Valid {
				if (isAvail.Valid && !isAvail.Bool) || (stock.Valid && stock.Int64 <= 0) {
					item.Availability = "OUT_OF_STOCK"
				} else {
					item.Availability = "IN_STOCK"
				}
			}
			if effPrice.Valid {
				applyEffectivePrice(&item, effPrice.Float64, f.PriceTable)
			}
			attachVariants(&item, toVariants(variants))
			sim := clamp(1.0-dist, 0, 1)
			sims[item.ID] = sim
			list = append(list, item)
		}
		return list, sims, nil
	}

	useANN := col == vectorColumnV2 && s.VectorIndexActive()
	res, sims, err := runQuery(useANN)
	if err != nil && useANN && isANNIndexError(err) {
		log.Printf("WARN: SimilarProducts ScaNN index fallback (%v); cooling down ANN for %s", err, annCooldownDuration)
		s.lastANNSucceeded.Store(false)
		s.annCooldownUntil.Store(time.Now().Add(annCooldownDuration).UnixNano())
		res, sims, err = runQuery(false)
		if err == nil {
			s.enrichResultsWithAttributes(ctx, res)
		}
		return res, sims, false, err
	}
	if useANN && err == nil {
		s.lastANNSucceeded.Store(true)
	}
	if err == nil {
		s.enrichResultsWithAttributes(ctx, res)
	}
	return res, sims, useANN && err == nil, err
}

func (s *SpannerService) querySimilarVectorMemory(seed seedProductInfo, filters models.SearchFilters) ([]models.SearchResult, map[string]float64) {
	f := normalizeFilters(filters)
	sims := map[string]float64{}
	var out []models.SearchResult
	qTokens := strings.Fields(FoldAccents(strings.ToLower(seed.Title + " " + seed.Category)))

	for _, p := range s.seedProducts {
		if p.ID == seed.ProductID {
			continue
		}
		if f.OnlyInStock && isUnavailable(p.Availability, p.AvailableQuantity) {
			continue
		}
		eff := s.memPricing().effective(f.PriceTable, p.ID, p.PriceInfo.NumericPrice)
		if !s.memPricing().visible(f.PriceTable, p.ID) {
			continue
		}
		matchedVariants, variantOK := s.memoryVariantMatch(p.ID, f)
		if !variantOK {
			continue
		}
		matchedAttrs, attrOK := s.memoryAttrMatch(p.ID, f.Attrs)
		if !attrOK {
			continue
		}
		if !matchesChannel(f.Channel, p.Channels) || !matchesCatalog(f, p) ||
			!s.deliverableInMemory(p.ID, f.Zip5, f.MaxSLAHours) {
			continue
		}

		docText := FoldAccents(strings.ToLower(p.Title + " " + p.Description + " " + strings.Join(p.Brands, " ") + " " + strings.Join(p.Categories, " ")))
		var sim float64
		if len(seed.VecV1) == 0 {
			// Lexical token overlap fallback when seed embedding is NULL (ADR-016).
			hits := 0
			for _, tok := range qTokens {
				if len(tok) >= 2 && strings.Contains(docText, tok) {
					hits++
				}
			}
			if len(qTokens) > 0 {
				sim = clamp(float64(hits)/float64(len(qTokens)), 0.05, 1.0)
			}
		} else {
			pVec := DeterministicEmbedding(docText, config.LegacyEmbeddingDimension)
			sim = clamp((CosineSimilarity(seed.VecV1, pVec)+1.0)/2.0, 0, 1)
			if len(p.Categories) > 0 && strings.EqualFold(p.Categories[0], seed.Category) {
				sim = clamp(sim+0.15, 0, 1)
			}
		}

		cp := p
		applyEffectivePrice(&cp, eff, f.PriceTable)
		attachVariants(&cp, matchedVariants)
		attachDynamicAttributes(&cp, matchedAttrs)
		sims[cp.ID] = sim
		out = append(out, cp)
	}
	return out, sims
}

// SimilarProducts computes item-to-item recommendations (`similar`, `substitutes`, `co_viewed`)
// blending semantic vector similarity, session co-visitation, and price proximity (ADR-016, Issue #44).
func (s *SpannerService) SimilarProducts(ctx context.Context, sessionSvc *SessionService, opts SimilarOptions) (models.SimilarProductsResponse, error) {
	start := time.Now()
	mode, err := ValidateSimilarMode(opts.Mode)
	if err != nil {
		return models.SimilarProductsResponse{}, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultSimilarLimit
	}
	if limit > MaxSimilarLimit {
		limit = MaxSimilarLimit
	}
	tenant := strings.TrimSpace(opts.TenantID)
	if tenant == "" {
		tenant = "default"
	}
	zip5, err := NormalizeCEP(opts.CEP)
	if err != nil {
		return models.SimilarProductsResponse{}, err
	}
	if err := ValidatePricing(opts.PriceTable, ""); err != nil {
		return models.SimilarProductsResponse{}, err
	}

	seed, err := s.lookupSeedProduct(ctx, opts.SeedID, tenant, opts.PriceTable)
	if err != nil {
		return models.SimilarProductsResponse{}, err
	}

	onlyInStock := opts.OnlyInStock
	if mode == SimilarModeSubstitutes {
		onlyInStock = true
	}
	filters := models.SearchFilters{
		TenantID:    tenant,
		Locale:      strings.TrimSpace(opts.Locale),
		Channel:     strings.ToLower(strings.TrimSpace(opts.Channel)),
		PriceTable:  strings.TrimSpace(opts.PriceTable),
		Zip5:        zip5,
		OnlyInStock: onlyInStock,
	}
	if mode == SimilarModeSubstitutes && seed.Category != "" {
		filters.Category = seed.Category
	}

	var (
		candidates      []models.SearchResult
		vecSims         map[string]float64
		vecIdxUsed      bool
		vecErr          error
		coWeights       map[string]float64
		coCounts        map[string]int64
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if s.client != nil {
			candidates, vecSims, vecIdxUsed, vecErr = s.querySimilarVectorSpanner(gctx, seed, filters, MaxSimilarLimit)
		} else {
			candidates, vecSims = s.querySimilarVectorMemory(seed, filters)
		}
		return nil
	})
	g.Go(func() error {
		if sessionSvc != nil {
			if cw, cc, cerr := sessionSvc.CoVisitation(gctx, tenant, seed.ProductID, 50); cerr == nil {
				coWeights, coCounts = cw, cc
			}
		}
		return nil
	})
	_ = g.Wait()
	if vecErr != nil {
		return models.SimilarProductsResponse{}, vecErr
	}

	maxCoWeight := 0.0
	for _, w := range coWeights {
		if w > maxCoWeight {
			maxCoWeight = w
		}
	}

	// Score and rank candidates according to ADR-016 mode weights.
	for i := range candidates {
		id := candidates[i].ID
		sim := vecSims[id]
		covis := 0.0
		if maxCoWeight > 0 {
			covis = clamp(coWeights[id]/maxCoWeight, 0, 1)
		}
		priceProx := 0.0
		if seed.Price > 0 {
			priceProx = math.Max(0, 1.0-math.Abs(candidates[i].PriceInfo.NumericPrice-seed.Price)/math.Max(seed.Price, 1.0))
		}
		var blended float64
		switch mode {
		case SimilarModeSubstitutes:
			blended = 0.70*sim + 0.20*priceProx + 0.10*covis
			reason := fmt.Sprintf("In-stock in %s", seed.Category)
			if priceProx >= 0.75 {
				reason += " · similar price"
			} else if candidates[i].PriceInfo.NumericPrice < seed.Price {
				reason += " · lower price"
			}
			candidates[i].SubstituteReason = reason
		case SimilarModeCoViewed:
			blended = 0.70*covis + 0.30*sim
		default:
			blended = 0.75*sim + 0.25*covis
		}
		candidates[i].SimilarityScore = round2(blended*100) / 100
		if candidates[i].SimilarityScore == 0 && blended > 0 {
			candidates[i].SimilarityScore = math.Round(blended*1000) / 1000
		}
		candidates[i].CoViewCount = coCounts[id]
		candidates[i].Score = map[string]float64{
			"similarity":      sim,
			"co_visitation":   covis,
			"price_proximity": priceProx,
			"final_score":     blended,
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		ui := isUnavailable(candidates[i].Availability, candidates[i].AvailableQuantity)
		uj := isUnavailable(candidates[j].Availability, candidates[j].AvailableQuantity)
		if ui != uj {
			return !ui
		}
		if mode == SimilarModeCoViewed && candidates[i].CoViewCount != candidates[j].CoViewCount {
			return candidates[i].CoViewCount > candidates[j].CoViewCount
		}
		si, sj := candidates[i].Score["final_score"], candidates[j].Score["final_score"]
		if si != sj {
			return si > sj
		}
		return candidates[i].ID < candidates[j].ID
	})

	totalFound := len(candidates)
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	if candidates == nil {
		candidates = []models.SearchResult{}
	}

	if zip5 != "" && len(candidates) > 0 {
		ids := make([]string, 0, len(candidates))
		for _, c := range candidates {
			ids = append(ids, c.ID)
		}
		if promises, perr := s.DeliveryPromises(ctx, ids, zip5, 0); perr == nil {
			for i := range candidates {
				if p, ok := promises[candidates[i].ID]; ok {
					cp := *p
					cp.CEP = opts.CEP
					candidates[i].DeliveryPromise = &cp
				}
			}
		}
	}

	return models.SimilarProductsResponse{
		SeedProductID:   seed.ProductID,
		Mode:            mode,
		Results:         candidates,
		TotalFound:      totalFound,
		LatencyMs:       float64(time.Since(start).Microseconds()) / 1000.0,
		BackendMode:     s.BackendMode(),
		VectorIndexUsed: vecIdxUsed,
	}, nil
}

// CheckSpannerSimilar validates both the item-to-item neighbor SQL and session co-visitation self-join (cmd/sqlcheck, ADR-016 #44).
func (s *SpannerService) CheckSpannerSimilar(ctx context.Context, sessionSvc *SessionService) (int, error) {
	if s.client == nil {
		return 0, fmt.Errorf("no Spanner client (memory mode)")
	}
	resp, err := s.SimilarProducts(ctx, sessionSvc, SimilarOptions{
		SeedID: "prod-004",
		Mode:   SimilarModeSubstitutes,
		Limit:  4,
	})
	if err != nil {
		return 0, err
	}
	return len(resp.Results), nil
}
