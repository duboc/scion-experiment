package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
	"psearch/serving-go/internal/models"
)

// Facet SQL expressions (ADR-003 / Issue #5). Price ranges follow ADR-003.
const (
	facetCategorySQL     = `IFNULL(category, '')`
	facetBrandSQL        = `IFNULL(brand, '')`
	facetPriceTmpl       = `CASE WHEN EFF IS NULL OR EFF < 100 THEN '0-100' WHEN EFF < 500 THEN '100-500' WHEN EFF < 1500 THEN '500-1500' ELSE '1500+' END`
	facetAvailabilitySQL = `IF(IFNULL(is_available, TRUE) AND IFNULL(stock, 1) > 0, 'IN_STOCK', 'OUT_OF_STOCK')`
)

// Facets aggregates categorical, price-range and availability counts over the
// candidate product IDs. In Spanner mode the four GROUP BY queries run
// concurrently (errgroup); in memory mode the same buckets are computed in Go.
// facetPriceSQL buckets the buyer's effective (contract) price (ADR-008, #22).
var facetPriceSQL = strings.ReplaceAll(facetPriceTmpl, "EFF", effectivePriceSQL("products"))

// Facets aggregates over the candidate IDs; priceTable/tenant select the
// contract price used for price_ranges (empty = list price).
func (s *SpannerService) Facets(ctx context.Context, ids []string, priceTable, tenant string) (*models.Facets, error) {
	return s.facetsWithQuery(ctx, "", ids, priceTable, tenant)
}

func (s *SpannerService) facetsWithQuery(ctx context.Context, query string, ids []string, priceTable, tenant string) (*models.Facets, error) {
	if tenant == "" {
		tenant = "default"
	}
	f := &models.Facets{
		Categories: []models.FacetBucket{}, Brands: []models.FacetBucket{},
		PriceRanges: []models.FacetBucket{}, Availability: []models.FacetBucket{},
	}
	if len(ids) == 0 {
		return f, nil
	}
	if s.client == nil {
		mf := s.memoryFacets(ids, priceTable)
		mf.Colors, mf.Sizes, _ = s.variantFacets(ctx, ids)
		mf.Attributes, _ = s.discoverDynamicFacets(ctx, query, 0, 0.5, models.SearchFilters{PriceTable: priceTable, TenantID: tenant}, ids)
		return mf, nil
	}
	// Issue #66 Phase B: merge the 4 products-table GROUP BY facet queries
	// (category, brand, price_range, availability) into 1 combined Spanner query
	// and call discoverDynamicFacets strictly once.
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		f.Categories, f.Brands, f.PriceRanges, f.Availability, err = s.queryCombinedFacets(gctx, ids, priceTable, tenant)
		return
	})
	g.Go(func() (err error) {
		f.Colors, f.Sizes, err = s.variantFacets(gctx, ids)
		return
	})
	g.Go(func() error {
		if attrs, aerr := s.discoverDynamicFacets(gctx, query, 0, 0.5, models.SearchFilters{PriceTable: priceTable, TenantID: tenant}, ids); aerr == nil {
			f.Attributes = attrs
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return f, nil
}

// combinedFacetSQL merges the 4 products-table GROUP BY facet aggregations
// (category, brand, price_range, availability) into a single Spanner query over
// product_id IN UNNEST(@ids) to cut Spanner session & CPU overhead by 4x (Issue #66 Phase B).
func combinedFacetSQL() string {
	return `WITH cand AS (
			SELECT
				` + facetCategorySQL + ` AS category_val,
				` + facetBrandSQL + ` AS brand_val,
				` + facetPriceSQL + ` AS price_val,
				` + facetAvailabilitySQL + ` AS avail_val
			FROM products
			WHERE product_id IN UNNEST(@ids)
		)
		SELECT 'category' AS dim, category_val AS value, COUNT(*) AS count FROM cand WHERE category_val != '' GROUP BY value
		UNION ALL
		SELECT 'brand' AS dim, brand_val AS value, COUNT(*) AS count FROM cand WHERE brand_val != '' GROUP BY value
		UNION ALL
		SELECT 'price_range' AS dim, price_val AS value, COUNT(*) AS count FROM cand WHERE price_val != '' GROUP BY value
		UNION ALL
		SELECT 'availability' AS dim, avail_val AS value, COUNT(*) AS count FROM cand WHERE avail_val != '' GROUP BY value
		ORDER BY dim, count DESC, value`
}

func (s *SpannerService) queryCombinedFacets(ctx context.Context, ids []string, priceTable, tenant string) (cats, brands, prices, avail []models.FacetBucket, err error) {
	cats = []models.FacetBucket{}
	brands = []models.FacetBucket{}
	prices = []models.FacetBucket{}
	avail = []models.FacetBucket{}
	if len(ids) == 0 {
		return cats, brands, prices, avail, nil
	}
	sql := specializeSQLForFilters(combinedFacetSQL(), models.SearchFilters{PriceTable: priceTable})
	params := map[string]interface{}{"ids": ids}
	if strings.Contains(sql, "@price_table") {
		params["price_table"], params["tenant_id"] = priceTable, tenant
	}
	iter := s.client.Single().Query(ctx, spanner.Statement{SQL: sql, Params: params})
	defer iter.Stop()
	for {
		row, rerr := iter.Next()
		if rerr == iterator.Done {
			return cats, brands, prices, avail, nil
		}
		if rerr != nil {
			return nil, nil, nil, nil, rerr
		}
		var dim string
		var value spanner.NullString
		var count int64
		if err := row.Columns(&dim, &value, &count); err != nil {
			return nil, nil, nil, nil, err
		}
		if value.StringVal == "" {
			continue
		}
		b := models.FacetBucket{Value: value.StringVal, Count: count}
		switch dim {
		case "category":
			cats = append(cats, b)
		case "brand":
			brands = append(brands, b)
		case "price_range":
			prices = append(prices, b)
		case "availability":
			avail = append(avail, b)
		}
	}
}

// facetSQL renders a GROUP BY facet query; expr is always a package constant.
func facetSQL(expr string) string {
	return `SELECT ` + expr + ` AS value, COUNT(*) AS count
		FROM products
		WHERE product_id IN UNNEST(@ids)
		GROUP BY value
		ORDER BY count DESC, value`
}

func (s *SpannerService) queryFacet(ctx context.Context, expr string, ids []string, priceTable, tenant string) ([]models.FacetBucket, error) {
	sql := specializeSQLForFilters(facetSQL(expr), models.SearchFilters{PriceTable: priceTable})
	params := map[string]interface{}{"ids": ids}
	if strings.Contains(sql, "@price_table") {
		params["price_table"], params["tenant_id"] = priceTable, tenant
	}
	iter := s.client.Single().Query(ctx, spanner.Statement{SQL: sql, Params: params})
	defer iter.Stop()
	out := []models.FacetBucket{}
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var value spanner.NullString
		var count int64
		if err := row.Columns(&value, &count); err != nil {
			return nil, err
		}
		if value.StringVal == "" {
			continue
		}
		out = append(out, models.FacetBucket{Value: value.StringVal, Count: count})
	}
}

func (s *SpannerService) memoryFacets(ids []string, priceTable string) *models.Facets {
	pricing := s.memPricing()
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	cats, brands, prices, avail := map[string]int64{}, map[string]int64{}, map[string]int64{}, map[string]int64{}
	for _, p := range s.seedProducts {
		if !want[p.ID] {
			continue
		}
		if len(p.Categories) > 0 {
			cats[p.Categories[0]]++
		}
		if len(p.Brands) > 0 {
			brands[p.Brands[0]]++
		}
		prices[PriceBucket(pricing.effective(priceTable, p.ID, p.PriceInfo.NumericPrice))]++
		if isUnavailable(p.Availability, p.AvailableQuantity) {
			avail["OUT_OF_STOCK"]++
		} else {
			avail["IN_STOCK"]++
		}
	}
	return &models.Facets{Categories: buckets(cats), Brands: buckets(brands), PriceRanges: buckets(prices), Availability: buckets(avail)}
}

// PriceBucket maps a price to its ADR-003 range.
func PriceBucket(price float64) string {
	switch {
	case price < 100:
		return "0-100"
	case price < 500:
		return "100-500"
	case price < 1500:
		return "500-1500"
	default:
		return "1500+"
	}
}

// buckets sorts like the SQL: count DESC, value ASC.
func buckets(m map[string]int64) []models.FacetBucket {
	out := make([]models.FacetBucket, 0, len(m))
	for v, c := range m {
		out = append(out, models.FacetBucket{Value: v, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	return out
}

const (
	MaxFacetValuesPerDimension = 20
	MaxFacetValueLength        = 128
)

var validPriceRangeBuckets = map[string]string{
	"0-100":    "0-100",
	"100-500":  "100-500",
	"500-1500": "500-1500",
	"1500+":    "1500+",
}

// ParseAndValidateFacetSlice merges singular, repeated query params, and JSON slice values,
// splits comma-separated lists, deduplicates case-insensitively, and enforces ADR-013 bounds.
func ParseAndValidateFacetSlice(dimName string, maxLen int, sources ...[]string) ([]string, error) {
	if maxLen <= 0 {
		maxLen = MaxFacetValueLength
	}
	var out []string
	seen := map[string]bool{}
	for _, src := range sources {
		for _, raw := range src {
			for _, part := range strings.Split(raw, ",") {
				v := strings.TrimSpace(part)
				if v == "" {
					continue
				}
				if len(v) > maxLen {
					return nil, fmt.Errorf("%s value exceeds max length %d", dimName, maxLen)
				}
				key := strings.ToLower(v)
				if !seen[key] {
					seen[key] = true
					out = append(out, v)
				}
			}
		}
	}
	if len(out) > MaxFacetValuesPerDimension {
		return nil, fmt.Errorf("too many %s values (max %d)", dimName, MaxFacetValuesPerDimension)
	}
	return out, nil
}

// ValidatePriceRanges parses and validates disjunctive price_ranges buckets (ADR-013, #41).
func ValidatePriceRanges(sources ...[]string) ([]string, error) {
	vals, err := ParseAndValidateFacetSlice("price_ranges", 32, sources...)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		canon, ok := validPriceRangeBuckets[v]
		if !ok {
			return nil, fmt.Errorf("invalid price_ranges bucket %q (want 0-100 | 100-500 | 500-1500 | 1500+)", v)
		}
		out = append(out, canon)
	}
	return out, nil
}

type facetDimension int

const (
	dimCategory facetDimension = iota
	dimBrand
	dimPriceRange
	dimAvailability
	dimColor
	dimSize
)

func hasActiveFacetFilters(f models.SearchFilters) bool {
	return f.Category != "" || len(f.Categories) > 0 ||
		f.Brand != "" || len(f.Brands) > 0 ||
		f.Color != "" || len(f.Colors) > 0 ||
		f.Size != "" || len(f.Sizes) > 0 ||
		len(f.PriceRanges) > 0 || f.OnlyInStock ||
		len(f.Attrs) > 0
}

func isDimensionActive(f models.SearchFilters, dim facetDimension) bool {
	switch dim {
	case dimCategory:
		return f.Category != "" || len(f.Categories) > 0
	case dimBrand:
		return f.Brand != "" || len(f.Brands) > 0
	case dimPriceRange:
		return len(f.PriceRanges) > 0
	case dimAvailability:
		return f.OnlyInStock
	case dimColor:
		return f.Color != "" || len(f.Colors) > 0
	case dimSize:
		return f.Size != "" || len(f.Sizes) > 0
	default:
		return false
	}
}

func withoutDimension(f models.SearchFilters, dim facetDimension) models.SearchFilters {
	cp := f
	cp.SkipAds = true
	switch dim {
	case dimCategory:
		cp.Category = ""
		cp.Categories = nil
	case dimBrand:
		cp.Brand = ""
		cp.Brands = nil
	case dimPriceRange:
		cp.PriceRanges = nil
	case dimAvailability:
		cp.OnlyInStock = false
	case dimColor:
		cp.Color = ""
		cp.Colors = nil
	case dimSize:
		cp.Size = ""
		cp.Sizes = nil
	}
	return cp
}

func selectedForDimension(f models.SearchFilters, dim facetDimension) []string {
	switch dim {
	case dimCategory:
		if len(f.Categories) > 0 {
			return f.Categories
		}
		if f.Category != "" {
			return []string{f.Category}
		}
	case dimBrand:
		if len(f.Brands) > 0 {
			return f.Brands
		}
		if f.Brand != "" {
			return []string{f.Brand}
		}
	case dimPriceRange:
		return f.PriceRanges
	case dimAvailability:
		if f.OnlyInStock {
			return []string{"IN_STOCK"}
		}
	case dimColor:
		if len(f.Colors) > 0 {
			return f.Colors
		}
		if f.Color != "" {
			return []string{f.Color}
		}
	case dimSize:
		if len(f.Sizes) > 0 {
			return f.Sizes
		}
		if f.Size != "" {
			return []string{f.Size}
		}
	}
	return nil
}

func markSelectedBuckets(b []models.FacetBucket, selected []string) []models.FacetBucket {
	if len(selected) == 0 {
		return b
	}
	matched := make([]bool, len(selected))
	for i := range b {
		for j, sel := range selected {
			if strings.EqualFold(b[i].Value, sel) {
				b[i].Selected = true
				matched[j] = true
			}
		}
	}
	for j, sel := range selected {
		if !matched[j] && sel != "" {
			b = append(b, models.FacetBucket{Value: sel, Count: 0, Selected: true})
		}
	}
	return b
}

const maxFacetCacheEntries = 512

func cloneFacets(in *models.Facets) *models.Facets {
	if in == nil {
		return nil
	}
	out := &models.Facets{
		Categories:   append([]models.FacetBucket(nil), in.Categories...),
		Brands:       append([]models.FacetBucket(nil), in.Brands...),
		PriceRanges:  append([]models.FacetBucket(nil), in.PriceRanges...),
		Availability: append([]models.FacetBucket(nil), in.Availability...),
	}
	if in.Colors != nil {
		out.Colors = append([]models.FacetBucket(nil), in.Colors...)
	}
	if in.Sizes != nil {
		out.Sizes = append([]models.FacetBucket(nil), in.Sizes...)
	}
	if len(in.Attributes) > 0 {
		out.Attributes = make([]models.DynamicFacet, len(in.Attributes))
		for i, df := range in.Attributes {
			out.Attributes[i] = models.DynamicFacet{
				Name:    df.Name,
				Label:   df.Label,
				Buckets: append([]models.FacetBucket(nil), df.Buckets...),
			}
		}
	}
	return out
}

func (s *SpannerService) facetCacheKey(query string, f models.SearchFilters, ids []string) string {
	base := s.countCacheKey(query, Expansion{}, f)
	idBytes, _ := json.Marshal(ids)
	return base + "|ids:" + string(idBytes)
}

func (s *SpannerService) getCachedFacets(key string) (*models.Facets, bool) {
	if s == nil || s.config == nil || s.config.EffectiveFacetCacheTTL() <= 0 {
		return nil, false
	}
	s.facetCacheMu.RLock()
	defer s.facetCacheMu.RUnlock()
	if s.facetCache == nil {
		return nil, false
	}
	e, ok := s.facetCache[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return cloneFacets(e.facets), true
}

func (s *SpannerService) setCachedFacets(key string, f *models.Facets) {
	if s == nil || s.config == nil || f == nil {
		return
	}
	ttl := s.config.EffectiveFacetCacheTTL()
	if ttl <= 0 {
		return
	}
	cp := cloneFacets(f)
	now := time.Now()
	s.facetCacheMu.Lock()
	defer s.facetCacheMu.Unlock()
	if s.facetCache == nil {
		s.facetCache = map[string]facetCacheEntry{}
	}
	if _, exists := s.facetCache[key]; !exists && len(s.facetCache) >= maxFacetCacheEntries {
		var oldestKey string
		var oldestExp time.Time
		for k, v := range s.facetCache {
			if now.After(v.expiresAt) {
				delete(s.facetCache, k)
				continue
			}
			if oldestKey == "" || v.expiresAt.Before(oldestExp) {
				oldestKey = k
				oldestExp = v.expiresAt
			}
		}
		if len(s.facetCache) >= maxFacetCacheEntries && oldestKey != "" {
			delete(s.facetCache, oldestKey)
		}
	}
	s.facetCache[key] = facetCacheEntry{
		facets:    cp,
		expiresAt: now.Add(ttl),
	}
}

// InvalidateFacetCache clears the per-instance facet TTL cache (called when attributes change; #66 Phase B).
func (s *SpannerService) InvalidateFacetCache() {
	if s == nil {
		return
	}
	s.facetCacheMu.Lock()
	s.facetCache = nil
	s.facetCacheMu.Unlock()
}

// DisjunctiveFacets computes exclude-own-dimension facet counts (ADR-013, Issue #41).
// When no facet dimension filter is active, it uses the zero-overhead fast path over filteredIDs.
// When dimension D is active, counts for D are aggregated over candidates matching the query
// and all active filters EXCEPT D, so sibling options retain their true counts.
// Results are cached per instance for FACET_CACHE_TTL (default 30s; Issue #66 Phase B).
func (s *SpannerService) DisjunctiveFacets(ctx context.Context, query string, minScore, alpha float64, filters models.SearchFilters, filteredIDs []string) (*models.Facets, error) {
	nf := normalizeFilters(filters)
	cacheKey := s.facetCacheKey(query, nf, filteredIDs)
	if cached, ok := s.getCachedFacets(cacheKey); ok {
		return cached, nil
	}
	if !hasActiveFacetFilters(nf) {
		f, err := s.facetsWithQuery(ctx, query, filteredIDs, nf.PriceTable, nf.TenantID)
		if err == nil && f != nil {
			s.setCachedFacets(cacheKey, f)
		}
		return f, err
	}

	const facetCandidateCap = 100
	idsForDim := func(gctx context.Context, dim facetDimension) ([]string, error) {
		if !isDimensionActive(nf, dim) {
			return filteredIDs, nil
		}
		out, err := s.HybridSearchDetailed(gctx, query, facetCandidateCap, minScore, alpha, withoutDimension(filters, dim))
		if err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(out.Results))
		for _, r := range out.Results {
			ids = append(ids, r.ID)
		}
		return ids, nil
	}

	var (
		catIDs   []string
		brandIDs []string
		priceIDs []string
		availIDs []string
		colorIDs []string
		sizeIDs  []string
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { catIDs, err = idsForDim(gctx, dimCategory); return })
	g.Go(func() (err error) { brandIDs, err = idsForDim(gctx, dimBrand); return })
	g.Go(func() (err error) { priceIDs, err = idsForDim(gctx, dimPriceRange); return })
	g.Go(func() (err error) { availIDs, err = idsForDim(gctx, dimAvailability); return })
	g.Go(func() (err error) { colorIDs, err = idsForDim(gctx, dimColor); return })
	g.Go(func() (err error) { sizeIDs, err = idsForDim(gctx, dimSize); return })
	if err := g.Wait(); err != nil {
		return nil, err
	}

	out := &models.Facets{
		Categories:   []models.FacetBucket{},
		Brands:       []models.FacetBucket{},
		PriceRanges:  []models.FacetBucket{},
		Availability: []models.FacetBucket{},
		Colors:       []models.FacetBucket{},
		Sizes:        []models.FacetBucket{},
	}

	if s.client == nil {
		if len(catIDs) > 0 {
			out.Categories = s.memoryFacets(catIDs, nf.PriceTable).Categories
		}
		if len(brandIDs) > 0 {
			out.Brands = s.memoryFacets(brandIDs, nf.PriceTable).Brands
		}
		if len(priceIDs) > 0 {
			out.PriceRanges = s.memoryFacets(priceIDs, nf.PriceTable).PriceRanges
		}
		if len(availIDs) > 0 {
			out.Availability = s.memoryFacets(availIDs, nf.PriceTable).Availability
		}
		if len(colorIDs) > 0 {
			out.Colors, _, _ = s.variantFacets(ctx, colorIDs)
		}
		if len(sizeIDs) > 0 {
			_, out.Sizes, _ = s.variantFacets(ctx, sizeIDs)
		}
	} else {
		g2, gctx2 := errgroup.WithContext(ctx)
		if len(catIDs) > 0 {
			g2.Go(func() (err error) {
				out.Categories, err = s.queryFacet(gctx2, facetCategorySQL, catIDs, nf.PriceTable, nf.TenantID)
				return
			})
		}
		if len(brandIDs) > 0 {
			g2.Go(func() (err error) {
				out.Brands, err = s.queryFacet(gctx2, facetBrandSQL, brandIDs, nf.PriceTable, nf.TenantID)
				return
			})
		}
		if len(priceIDs) > 0 {
			g2.Go(func() (err error) {
				out.PriceRanges, err = s.queryFacet(gctx2, facetPriceSQL, priceIDs, nf.PriceTable, nf.TenantID)
				return
			})
		}
		if len(availIDs) > 0 {
			g2.Go(func() (err error) {
				out.Availability, err = s.queryFacet(gctx2, facetAvailabilitySQL, availIDs, nf.PriceTable, nf.TenantID)
				return
			})
		}
		if len(colorIDs) > 0 {
			g2.Go(func() (err error) {
				out.Colors, _, err = s.variantFacets(gctx2, colorIDs)
				return
			})
		}
		if len(sizeIDs) > 0 {
			g2.Go(func() (err error) {
				_, out.Sizes, err = s.variantFacets(gctx2, sizeIDs)
				return
			})
		}
		if err := g2.Wait(); err != nil {
			return nil, err
		}
	}

	out.Categories = markSelectedBuckets(out.Categories, selectedForDimension(filters, dimCategory))
	out.Brands = markSelectedBuckets(out.Brands, selectedForDimension(filters, dimBrand))
	out.PriceRanges = markSelectedBuckets(out.PriceRanges, selectedForDimension(filters, dimPriceRange))
	out.Availability = markSelectedBuckets(out.Availability, selectedForDimension(filters, dimAvailability))
	out.Colors = markSelectedBuckets(out.Colors, selectedForDimension(filters, dimColor))
	out.Sizes = markSelectedBuckets(out.Sizes, selectedForDimension(filters, dimSize))
	if attrs, aerr := s.discoverDynamicFacets(ctx, query, minScore, alpha, nf, filteredIDs); aerr == nil {
		out.Attributes = attrs
	}
	s.setCachedFacets(cacheKey, out)
	return out, nil
}
