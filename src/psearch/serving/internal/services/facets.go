package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
		return mf, nil
	}
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) {
		f.Categories, err = s.queryFacet(gctx, facetCategorySQL, ids, priceTable, tenant)
		return
	})
	g.Go(func() (err error) { f.Brands, err = s.queryFacet(gctx, facetBrandSQL, ids, priceTable, tenant); return })
	g.Go(func() (err error) {
		f.PriceRanges, err = s.queryFacet(gctx, facetPriceSQL, ids, priceTable, tenant)
		return
	})
	g.Go(func() (err error) {
		f.Availability, err = s.queryFacet(gctx, facetAvailabilitySQL, ids, priceTable, tenant)
		return
	})
	g.Go(func() (err error) {
		f.Colors, f.Sizes, err = s.variantFacets(gctx, ids)
		return
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return f, nil
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
	params := map[string]interface{}{"ids": ids}
	if strings.Contains(expr, "@price_table") {
		params["price_table"], params["tenant_id"] = priceTable, tenant
	}
	iter := s.client.Single().Query(ctx, spanner.Statement{SQL: facetSQL(expr), Params: params})
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
		len(f.PriceRanges) > 0 || f.OnlyInStock
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

// DisjunctiveFacets computes exclude-own-dimension facet counts (ADR-013, Issue #41).
// When no facet dimension filter is active, it uses the zero-overhead fast path over filteredIDs.
// When dimension D is active, counts for D are aggregated over candidates matching the query
// and all active filters EXCEPT D, so sibling options retain their true counts.
func (s *SpannerService) DisjunctiveFacets(ctx context.Context, query string, minScore, alpha float64, filters models.SearchFilters, filteredIDs []string) (*models.Facets, error) {
	nf := normalizeFilters(filters)
	if !hasActiveFacetFilters(nf) {
		return s.Facets(ctx, filteredIDs, nf.PriceTable, nf.TenantID)
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
	return out, nil
}
