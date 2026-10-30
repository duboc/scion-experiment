package models

type SearchRequest struct {
	Query             string   `json:"query" form:"q"`
	Limit             *int     `json:"limit,omitempty" form:"limit"`
	MinScore          *float64 `json:"min_score,omitempty" form:"min_score"`
	Alpha             *float64 `json:"alpha,omitempty" form:"alpha"`
	Category          string   `json:"category,omitempty" form:"category"`
	Categories        []string `json:"categories,omitempty" form:"categories"` // ADR-013 (#41): disjunctive multi-select categories
	Brand             string   `json:"brand,omitempty" form:"brand"`
	Brands            []string `json:"brands,omitempty" form:"brands"` // ADR-013 (#41): disjunctive multi-select brands
	OnlyInStock       bool     `json:"only_in_stock,omitempty" form:"only_in_stock"`
	Channel           string   `json:"channel,omitempty" form:"channel"`
	TenantID          string   `json:"tenant_id,omitempty" form:"tenant_id"`
	Locale            string   `json:"locale,omitempty" form:"locale"`
	CEP               string   `json:"cep,omitempty" form:"cep"`
	LTR               bool     `json:"ltr,omitempty" form:"ltr"`
	MaxSLAHours       int64    `json:"max_sla_hours,omitempty" form:"max_sla_hours"`
	MinPrice          float64  `json:"min_price,omitempty" form:"min_price"`
	MaxPrice          float64  `json:"max_price,omitempty" form:"max_price"`
	PriceRanges       []string `json:"price_ranges,omitempty" form:"price_ranges"` // ADR-013 (#41): disjunctive price range buckets ("0-100", "100-500", "500-1500", "1500+")
	ParseIntent       *bool    `json:"parse_intent,omitempty" form:"parse_intent"`
	PriceTable        string   `json:"price_table_id,omitempty" form:"price_table_id"`
	Color             string   `json:"color,omitempty" form:"color"`
	Colors            []string `json:"colors,omitempty" form:"colors"` // ADR-013 (#41): disjunctive multi-select SKU colors
	Size              string              `json:"size,omitempty" form:"size"`
	Sizes             []string            `json:"sizes,omitempty" form:"sizes"` // ADR-013 (#41): disjunctive multi-select SKU sizes
	Attrs             map[string][]string `json:"attrs,omitempty" form:"-"`     // ADR-014 (#42): dynamic attribute filters (key -> disjunctive values)
	SortBy            string   `json:"sort_by,omitempty" form:"sort_by"`
	SessionID         string   `json:"session_id,omitempty" form:"session_id"`
	SessionProductIDs []string `json:"session_product_ids,omitempty" form:"session_product_ids"` // default true (ADR-007); false disables
	ImageBase64       string   `json:"image_base64,omitempty" form:"-"`                          // ADR-010 (#33): inline query photo (base64 or data URL); URLs are never fetched
	ImageMimeType     string   `json:"image_mime_type,omitempty" form:"-"`                       // image/jpeg | image/png | image/webp
	ImageWeight       *float64 `json:"image_weight,omitempty" form:"-"`                          // image share of the fused query vector (default 0.6)
	Ads               *bool    `json:"ads,omitempty" form:"ads"`                                 // ADR-011 (#35): default true; false disables sponsored product injection
	Page              int                 `json:"page,omitempty" form:"page"`               // ADR-011 (#35): 1-based page number (ads injected on page 1 only)
	Offset            int                 `json:"offset,omitempty" form:"offset"`           // ADR-011 (#35): item offset (ads injected when offset == 0 only)
	ExperimentID      string              `json:"experiment_id,omitempty" form:"experiment_id"` // ADR-015 (#43): optional A/B experiment variant tag
}

// SearchFilters are the catalog filters applied server-side (ADR-002, Issues #3/#4/#5).
type SearchFilters struct {
	OnlyInStock bool
	Channel     string
	Category    string   // exact (case-insensitive) category match; empty = all (Issue #5 facet drill-down)
	Categories  []string // ADR-013 (#41): disjunctive categories (OR within dimension)
	Brand       string   // exact (case-insensitive) brand match; empty = all
	Brands      []string // ADR-013 (#41): disjunctive brands (OR within dimension)
	TenantID    string   // ADR-002 / Issue #4; empty = "default"
	Locale      string   // ADR-002 / Issue #4 (pt-BR, en-US, es-MX); empty = all locales
	Zip5        string   // ADR-005 / Issue #8: 5-digit CEP prefix; empty = no delivery filter
	MaxSLAHours int64    // ADR-005: maximum delivery SLA in hours; 0 = any
	RelaxText   bool     // ADR-008 (#21): run the text leg with OR between terms
	PriceTable  string   // ADR-008 (#22): contract price table; "" = list price
	Color       string   // ADR-009 (#32): in-stock child SKU color; "" = any
	Colors      []string // ADR-013 (#41): disjunctive SKU colors (OR within dimension)
	Size        string              // ADR-009 (#32): in-stock child SKU size; "" = any
	Sizes       []string            // ADR-013 (#41): disjunctive SKU sizes (OR within dimension)
	PriceRanges []string            // ADR-013 (#41): disjunctive price range buckets ("0-100", "100-500", "500-1500", "1500+")
	Attrs       map[string][]string // ADR-014 (#42): dynamic attribute filters (key -> disjunctive values)
	SortBy      string   // ADR-008/012 (#22/#40): relevance | price_asc | price_desc | popularity | rating_desc
	MinPrice    float64  // ADR-007/008: minimum price (0 = none)
	MaxPrice    float64  // ADR-007/008: maximum price (0 = none)
	// ADR-010 (#33): query image vector (gemini-embedding-2, same space as
	// embedding_v2) and its share of the fused vector. Not a catalog filter; it
	// rides along so recovery/relaxation re-runs keep the image.
	ImageVector []float32
	ImageWeight float64
}

type SearchResponse struct {
	Query            string         `json:"query"`
	EffectiveAlpha   float64        `json:"effective_alpha"`
	Results          []SearchResult `json:"results"`
	TotalFound       int            `json:"total_found"`
	Page             int            `json:"page"`        // ADR-012 (#40): 1-based current page
	PageSize         int            `json:"page_size"`   // ADR-012 (#40): effective page size (limit)
	Offset           int            `json:"offset"`      // ADR-012 (#40): 0-based item offset
	TotalPages       int            `json:"total_pages"` // ADR-012 (#40): ceiling(total_found / page_size)
	HasNext          bool           `json:"has_next"`    // ADR-012 (#40): true when offset + page_size < total_found
	Facets           *Facets        `json:"facets,omitempty"`
	LatencyMs        float64        `json:"latency_ms"`
	BackendMode      string         `json:"backend_mode"`
	EmbeddingVersion string         `json:"embedding_version,omitempty"`
	RetrievalMode    string         `json:"retrieval_mode,omitempty"`
	ImageQuery       bool           `json:"image_query,omitempty"`     // ADR-010 (#33): a query photo drove the vector leg
	ImageWeight      float64        `json:"image_weight,omitempty"`    // effective image share of the fused vector
	ImageCacheHit    bool           `json:"image_cache_hit,omitempty"` // image vector served from the LRU
	CacheHit         bool           `json:"cache_hit,omitempty"`
	FacetsLatencyMs  float64        `json:"facets_latency_ms,omitempty"`
	Candidates       int            `json:"candidates,omitempty"`     // size of the facet candidate set (top 100)
	TenantID         string         `json:"tenant_id,omitempty"`      // effective tenant (ADR-002)
	Locale           string         `json:"locale,omitempty"`         // locale filter applied ("" = all)
	ExpandedTerms    []string       `json:"expanded_terms,omitempty"` // synonyms added at reduced weight (Issue #7)
	AppliedRules     []string       `json:"applied_rules,omitempty"`  // merchandising rule_ids applied (Issue #6)
	LTR              bool           `json:"ltr,omitempty"`            // stage-2 re-ranking applied (Issue #10)
	LTRLatencyUS     int64          `json:"ltr_latency_us,omitempty"` // stage-2 in-memory latency (microseconds)
	LTRModel         string         `json:"ltr_model,omitempty"`
	ParsedQuery      *ParsedQuery   `json:"parsed_query,omitempty"` // Issue #19 (ADR-007)
	DidYouMean       string         `json:"did_you_mean,omitempty"` // Issue #21 (ADR-008)
	AutoCorrected    bool           `json:"auto_corrected,omitempty"`
	QueryRelaxed     bool           `json:"query_relaxed,omitempty"`
	PriceTableID     string         `json:"price_table_id,omitempty"` // Issue #22
	SortBy           string         `json:"sort_by,omitempty"`
	SessionID        string         `json:"session_id,omitempty"` // Issue #20 (ADR-007)
	Personalized     bool           `json:"personalized,omitempty"`
	SponsoredCount   int            `json:"sponsored_count"`   // ADR-011 (#35): number of sponsored items injected
	VectorIndexUsed  bool           `json:"vector_index_used"` // ADR-012 (#39): true when ScaNN vector index served the vector leg
	QueryID          string         `json:"query_id,omitempty"` // ADR-015 (#43): unique search impression ID for attribution
	ExperimentID     string         `json:"experiment_id,omitempty"` // ADR-015 (#43): active A/B experiment tag
}

type FacetBucket struct {
	Value    string `json:"value"`
	Count    int64  `json:"count"`
	Selected bool   `json:"selected,omitempty"` // ADR-013 (#41): true when value is actively selected in the request
}

type DynamicFacet struct {
	Name    string        `json:"name"`
	Label   string        `json:"label"`
	Buckets []FacetBucket `json:"buckets"`
}

type Facets struct {
	Categories   []FacetBucket  `json:"categories"`
	Brands       []FacetBucket  `json:"brands"`
	PriceRanges  []FacetBucket  `json:"price_ranges"`
	Availability []FacetBucket  `json:"availability"`
	Colors       []FacetBucket  `json:"colors,omitempty"`     // in-stock variant colors (#32)
	Sizes        []FacetBucket  `json:"sizes,omitempty"`      // in-stock variant sizes (#32)
	Attributes   []DynamicFacet `json:"attributes,omitempty"` // auto-discovered dynamic product attributes (#42, ADR-014)
}

type SearchResult struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Title             string             `json:"title"`
	Description       string             `json:"description,omitempty"`
	Brands            []string           `json:"brands"`
	Categories        []string           `json:"categories"`
	PriceInfo         PriceInfo          `json:"priceInfo"`
	ColorInfo         *ColorInfo         `json:"colorInfo,omitempty"`
	Availability      string             `json:"availability"`
	AvailableQuantity *int               `json:"availableQuantity,omitempty"`
	AvailableTime     *string            `json:"availableTime,omitempty"`
	Channels          []string           `json:"channels,omitempty"`
	Signals           *BoostSignals      `json:"boost_signals,omitempty"`
	AvgRating         float64            `json:"avg_rating,omitempty"`   // ADR-012 (#40): average customer rating [0, 5]
	ReviewCount       int64              `json:"review_count,omitempty"` // ADR-012 (#40): customer review count tie-breaker
	Locale            string             `json:"locale,omitempty"`
	TenantID          string             `json:"tenant_id,omitempty"`
	Images            []Image            `json:"images"`
	Sizes             []string           `json:"sizes"`
	RetrievableFields string                 `json:"retrievableFields"`
	Attributes        []Attribute            `json:"attributes"`
	DynamicAttributes map[string]interface{} `json:"dynamic_attributes,omitempty"` // ADR-014 (#42): schema-free product specs map
	URI               string             `json:"uri"`
	Score             map[string]float64 `json:"score"`
	DeliveryPromise   *DeliveryPromise   `json:"deliveryPromise,omitempty"`
	Variants          []Variant          `json:"variants,omitempty"` // child SKUs (ADR-009, #32)
	VariantCount      int                `json:"variant_count,omitempty"`
	AvailableColors   []string           `json:"available_colors,omitempty"`
	AvailableSizes    []string           `json:"available_sizes,omitempty"`
	Sponsored         bool                   `json:"sponsored,omitempty"`          // ADR-011 (#35): true when injected from ad_campaigns
	AdCampaignID      string                 `json:"ad_campaign_id,omitempty"`     // ADR-011 (#35): campaign_id that placed this result
	AdClickID         string                 `json:"ad_click_id,omitempty"`        // ADR-011 (#35): opaque click token for idempotent CPC billing
	SimilarityScore   float64                `json:"similarity_score,omitempty"`   // ADR-016 (#44): item-to-item similarity/blended score
	CoViewCount       int64                  `json:"co_view_count,omitempty"`      // ADR-016 (#44): session co-visitation count
	SubstituteReason  string                 `json:"substitute_reason,omitempty"`  // ADR-016 (#44): human-readable substitute explanation
}

// SimilarProductsResponse is the payload returned by GET /api/products/:id/similar (ADR-016, Issue #44).
type SimilarProductsResponse struct {
	SeedProductID   string         `json:"seed_product_id"`
	Mode            string         `json:"mode"`
	Results         []SearchResult `json:"results"`
	TotalFound      int            `json:"total_found"`
	LatencyMs       float64        `json:"latency_ms"`
	BackendMode     string         `json:"backend_mode"`
	VectorIndexUsed bool           `json:"vector_index_used"`
}

type DeliveryPromise struct {
	CEP          string  `json:"cep"`
	RegionCode   string  `json:"region_code"`
	CDID         string  `json:"cd_id"`
	SellerID     string  `json:"seller_id"`
	SLAHours     int64   `json:"sla_hours"`
	FreeShipping bool    `json:"free_shipping"`
	ShippingCost float64 `json:"shipping_cost"`
	Label        string  `json:"label"` // e.g. "Chega amanhã - CD São Paulo" (ADR-005, Issue #8)
}

type Image struct {
	Height string `json:"height"`
	Width  string `json:"width"`
	URI    string `json:"uri"`
}

type PriceInfo struct {
	Cost               string  `json:"cost"`
	CurrencyCode       string  `json:"currencyCode"`
	OriginalPrice      string  `json:"originalPrice"`
	Price              string  `json:"price"`
	NumericPrice       float64 `json:"numericPrice,omitempty"`
	BasePrice          float64 `json:"basePrice,omitempty"`    // list price before contract pricing (ADR-008)
	PriceTableID       string  `json:"priceTableId,omitempty"` // contract table applied (ADR-008)
	PriceEffectiveTime string  `json:"priceEffectiveTime"`
	PriceExpireTime    string  `json:"priceExpireTime"`
}

type ColorInfo struct {
	ColorFamilies []string `json:"colorFamilies,omitempty"`
	Colors        []string `json:"colors,omitempty"`
}

type AttributeValue struct {
	Indexable  *string   `json:"indexable,omitempty"`
	Searchable *string   `json:"searchable,omitempty"`
	Text       []string  `json:"text,omitempty"`
	Numbers    []float64 `json:"numbers,omitempty"`
}

type Attribute struct {
	Key   string         `json:"key"`
	Value AttributeValue `json:"value"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

// BoostSignals are the 8 business signals used for ranking boosts (ADR-004, Issue #6).
type BoostSignals struct {
	Clicks          int64   `json:"clicks"`
	Orders          int64   `json:"orders"`
	DiscountPct     float64 `json:"discount_pct"`
	PopularityScore float64 `json:"popularity_score"`
	MarginPct       float64 `json:"margin_pct"`
	CTR             float64 `json:"ctr"`
	ConversionRate  float64 `json:"conversion_rate"`
	RecencyScore    float64 `json:"recency_score"`
}

// ParsedQuery reports how a natural-language query was decomposed (ADR-007, Issue #19).
type ParsedQuery struct {
	OriginalQuery    string                 `json:"original_query"`
	CleanQuery       string                 `json:"clean_query"`
	ExtractedFilters map[string]interface{} `json:"extracted_filters"`
	IntentSource     string                 `json:"intent_source"` // parser | cache | gemini | none
}

// Variant is one child SKU of a parent product (ADR-009, Issue #32).
type Variant struct {
	SKUID     string `json:"sku_id"`
	Color     string `json:"color,omitempty"`
	Size      string `json:"size,omitempty"`
	Stock     int64  `json:"stock"`
	Available bool   `json:"available"`
}

// AdCampaign is one row of ad_campaigns (ADR-011, Issue #35).
type AdCampaign struct {
	CampaignID   string  `json:"campaign_id"`
	TenantID     string  `json:"tenant_id"`
	Advertiser   string  `json:"advertiser"`
	ProductID    string  `json:"product_id"`
	CPCMicros    int64   `json:"cpc_micros"`
	BudgetMicros int64   `json:"budget_micros"`
	SpentMicros  int64   `json:"spent_micros"`
	Status       string  `json:"status"` // active | paused | exhausted
	StartAt      *string `json:"start_at,omitempty"`
	EndAt        *string `json:"end_at,omitempty"`
	UpdatedAt    *string `json:"updated_at,omitempty"`
}

// AdClickRequest is the payload for POST /api/ads/click (ADR-011, Issue #35).
type AdClickRequest struct {
	CampaignID string `json:"campaign_id"`
	ClickID    string `json:"click_id"`
}

// AdClickResponse is the result of POST /api/ads/click (ADR-011, Issue #35).
type AdClickResponse struct {
	CampaignID      string `json:"campaign_id"`
	ClickID         string `json:"click_id"`
	ProductID       string `json:"product_id,omitempty"`
	Charged         bool   `json:"charged"`
	CostMicros      int64  `json:"cost_micros"`
	SpentMicros     int64  `json:"spent_micros"`
	BudgetMicros    int64  `json:"budget_micros"`
	RemainingMicros int64  `json:"remaining_micros"`
	Status          string `json:"status"`
	Reason          string `json:"reason,omitempty"`
}

// AnalyticsEventRequest is the payload for POST /api/analytics/events (ADR-015, Issue #43).
type AnalyticsEventRequest struct {
	EventID       string `json:"event_id"`
	QueryID       string `json:"query_id"`
	ImpressionID  string `json:"impression_id,omitempty"`
	SessionID     string `json:"session_id,omitempty"`
	EventType     string `json:"event_type"` // click | add_to_cart | purchase
	ProductID     string `json:"product_id"`
	Position      int    `json:"position"` // 1-based rank position clicked
	ExperimentID  string `json:"experiment_id,omitempty"`
	RevenueMicros int64  `json:"revenue_micros,omitempty"`
	TenantID      string `json:"tenant_id,omitempty"`
}

// AnalyticsEventResponse is the result of POST /api/analytics/events (ADR-015, Issue #43).
type AnalyticsEventResponse struct {
	EventID       string `json:"event_id"`
	QueryID       string `json:"query_id"`
	TenantID      string `json:"tenant_id"`
	EventType     string `json:"event_type"`
	ProductID     string `json:"product_id"`
	Position      int    `json:"position"`
	ExperimentID  string `json:"experiment_id,omitempty"`
	RevenueMicros int64  `json:"revenue_micros"`
	Duplicate     bool   `json:"duplicate"`
	Recorded      bool   `json:"recorded"`
}

// QueryMetric reports query volume and CTR for top queries / zero-result queries (ADR-015, Issue #43).
type QueryMetric struct {
	Query string  `json:"query"`
	Count int64   `json:"count"`
	CTR   float64 `json:"ctr,omitempty"`
}

// ExperimentSummary reports per-experiment A/B search metrics (ADR-015, Issue #43).
type ExperimentSummary struct {
	ExperimentID            string  `json:"experiment_id"`
	Searches                int64   `json:"searches"`
	TotalSearches           int64   `json:"total_searches"`
	ZeroResultSearches      int64   `json:"zero_result_searches"`
	ZeroResultRate          float64 `json:"zero_result_rate"`
	Clicks                  int64   `json:"clicks"`
	Conversions             int64   `json:"conversions"`
	ClickThroughRate        float64 `json:"click_through_rate"`
	CTR                     float64 `json:"ctr"`
	ConversionRate          float64 `json:"conversion_rate"`
	MRRAt10                 float64 `json:"mrr_at_10"`
	AvgLatencyMs            float64 `json:"avg_latency_ms"`
	AttributedRevenueMicros int64   `json:"attributed_revenue_micros"`
}

// AnalyticsSummaryResponse is returned by GET /api/analytics/summary (ADR-015, Issue #43).
type AnalyticsSummaryResponse struct {
	TenantID                string              `json:"tenant_id"`
	WindowHours             int                 `json:"window_hours"`
	TotalSearches           int64               `json:"total_searches"`
	ZeroResultSearches      int64               `json:"zero_result_searches"`
	ZeroResultRate          float64             `json:"zero_result_rate"`
	AvgLatencyMs            float64             `json:"avg_latency_ms"`
	TotalClicks             int64               `json:"total_clicks"`
	TotalConversions        int64               `json:"total_conversions"`
	ClickThroughRate        float64             `json:"click_through_rate"`
	CTR                     float64             `json:"ctr"`
	ConversionRate          float64             `json:"conversion_rate"`
	MRRAt10                 float64             `json:"mrr_at_10"`
	AttributedRevenueMicros int64               `json:"attributed_revenue_micros"`
	TopQueries              []QueryMetric       `json:"top_queries"`
	TopZeroResultQueries    []QueryMetric       `json:"top_zero_result_queries"`
	Experiments             []ExperimentSummary `json:"experiments"`
}

// AnalyticsRollupResponse is returned by POST /api/analytics/rollup (ADR-015, Issue #43).
type AnalyticsRollupResponse struct {
	TenantID        string                  `json:"tenant_id"`
	WindowDays      int                     `json:"window_days"`
	ProductsUpdated int                     `json:"products_updated"`
	UpdatedSignals  map[string]BoostSignals `json:"updated_signals,omitempty"`
}

