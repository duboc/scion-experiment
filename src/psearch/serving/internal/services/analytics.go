package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"psearch/serving-go/internal/models"
)

// Closed-loop search telemetry, click attribution, online A/B KPIs & LTR signal rollup (ADR-015, Issue #43).

const (
	impressionBufferSize    = 2048
	defaultSummaryHours     = 168 // 7 days
	maxSummaryHours         = 2160
	defaultRollupWindowDays = 30
)

var experimentIDRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{1,64}$`)

// ValidateExperimentID checks optional experiment_id values (ADR-015, #43).
func ValidateExperimentID(expID string) error {
	expID = strings.TrimSpace(expID)
	if expID == "" {
		return nil
	}
	if !experimentIDRe.MatchString(expID) {
		return fmt.Errorf("invalid experiment_id %q (must match ^[a-zA-Z0-9._-]{1,64}$)", expID)
	}
	return nil
}

// NewQueryID generates an opaque unique query_id (impression_id) for a search response.
func NewQueryID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("imp-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(b[:]))
}

// SearchImpression mirrors one row of search_impressions (migration 026).
type SearchImpression struct {
	ImpressionID  string    `json:"impression_id"`
	TenantID      string    `json:"tenant_id"`
	QueryRaw      string    `json:"query_raw"`
	QueryNorm     string    `json:"query_norm"`
	ExperimentID  string    `json:"experiment_id,omitempty"`
	RetrievalMode string    `json:"retrieval_mode,omitempty"`
	TotalFound    int64     `json:"total_found"`
	LatencyMs     float64   `json:"latency_ms"`
	ResultIDs     []string  `json:"result_ids"`
	CreatedAt     time.Time `json:"created_at"`
}

// SearchConversion mirrors one row of search_conversions (migration 026).
type SearchConversion struct {
	TenantID      string    `json:"tenant_id"`
	EventID       string    `json:"event_id"`
	ImpressionID  string    `json:"impression_id"`
	SessionID     string    `json:"session_id,omitempty"`
	EventType     string    `json:"event_type"` // click | add_to_cart | purchase
	ProductID     string    `json:"product_id"`
	Position      int64     `json:"position"` // 1-based rank position
	ExperimentID  string    `json:"experiment_id,omitempty"`
	RevenueMicros int64     `json:"revenue_micros"`
	CreatedAt     time.Time `json:"created_at"`
}

// AnalyticsStore abstracts Spanner vs in-memory telemetry storage.
type AnalyticsStore interface {
	RecordImpression(ctx context.Context, imp SearchImpression) error
	RecordConversion(ctx context.Context, conv SearchConversion) (duplicate bool, err error)
	ListWindow(ctx context.Context, tenant string, since time.Time) ([]SearchImpression, []SearchConversion, error)
}

// MemoryAnalyticsStore keeps impressions and idempotent conversions in memory.
type MemoryAnalyticsStore struct {
	mu          sync.RWMutex
	impressions []SearchImpression
	conversions []SearchConversion
	seenEvents  map[string]bool // tenant|event_id -> true
}

func NewMemoryAnalyticsStore() *MemoryAnalyticsStore {
	return &MemoryAnalyticsStore{
		seenEvents: map[string]bool{},
	}
}

func (m *MemoryAnalyticsStore) RecordImpression(_ context.Context, imp SearchImpression) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if imp.CreatedAt.IsZero() {
		imp.CreatedAt = time.Now().UTC()
	}
	cp := imp
	cp.ResultIDs = append([]string(nil), imp.ResultIDs...)
	m.impressions = append(m.impressions, cp)
	if len(m.impressions) > 10000 {
		m.impressions = m.impressions[len(m.impressions)-10000:]
	}
	return nil
}

func (m *MemoryAnalyticsStore) RecordConversion(_ context.Context, conv SearchConversion) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := conv.TenantID + "|" + conv.EventID
	if m.seenEvents[key] {
		return true, nil
	}
	m.seenEvents[key] = true
	if conv.CreatedAt.IsZero() {
		conv.CreatedAt = time.Now().UTC()
	}
	m.conversions = append(m.conversions, conv)
	if len(m.conversions) > 20000 {
		m.conversions = m.conversions[len(m.conversions)-20000:]
	}
	return false, nil
}

func (m *MemoryAnalyticsStore) ListWindow(_ context.Context, tenant string, since time.Time) ([]SearchImpression, []SearchConversion, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var imps []SearchImpression
	for _, imp := range m.impressions {
		if imp.TenantID == tenant && !imp.CreatedAt.Before(since) {
			cp := imp
			cp.ResultIDs = append([]string(nil), imp.ResultIDs...)
			imps = append(imps, cp)
		}
	}
	var convs []SearchConversion
	for _, c := range m.conversions {
		if c.TenantID == tenant && !c.CreatedAt.Before(since) {
			convs = append(convs, c)
		}
	}
	return imps, convs, nil
}

// SpannerAnalyticsStore persists impressions & conversions in Cloud Spanner (migration 026).
type SpannerAnalyticsStore struct {
	Client *spanner.Client
}

func (s SpannerAnalyticsStore) RecordImpression(ctx context.Context, imp SearchImpression) error {
	if s.Client == nil {
		return nil
	}
	mut := spanner.InsertOrUpdate("search_impressions",
		[]string{"tenant_id", "impression_id", "query_raw", "query_norm", "experiment_id", "retrieval_mode", "total_found", "latency_ms", "result_ids", "created_at"},
		[]interface{}{
			imp.TenantID,
			imp.ImpressionID,
			imp.QueryRaw,
			imp.QueryNorm,
			imp.ExperimentID,
			imp.RetrievalMode,
			imp.TotalFound,
			imp.LatencyMs,
			nonNilStrings(imp.ResultIDs),
			spanner.CommitTimestamp,
		},
	)
	_, err := s.Client.Apply(ctx, []*spanner.Mutation{mut})
	return err
}

func (s SpannerAnalyticsStore) RecordConversion(ctx context.Context, conv SearchConversion) (bool, error) {
	if s.Client == nil {
		return false, errors.New("no Spanner client")
	}
	mut := spanner.Insert("search_conversions",
		[]string{"tenant_id", "event_id", "impression_id", "session_id", "event_type", "product_id", "position", "experiment_id", "revenue_micros", "created_at"},
		[]interface{}{
			conv.TenantID,
			conv.EventID,
			conv.ImpressionID,
			conv.SessionID,
			conv.EventType,
			conv.ProductID,
			conv.Position,
			conv.ExperimentID,
			conv.RevenueMicros,
			spanner.CommitTimestamp,
		},
	)
	_, err := s.Client.Apply(ctx, []*spanner.Mutation{mut})
	if err != nil {
		if spanner.ErrCode(err) == codes.AlreadyExists || strings.Contains(strings.ToLower(err.Error()), "alreadyexists") || strings.Contains(strings.ToLower(err.Error()), "already exists") {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

func (s SpannerAnalyticsStore) ListWindow(ctx context.Context, tenant string, since time.Time) ([]SearchImpression, []SearchConversion, error) {
	if s.Client == nil {
		return nil, nil, errors.New("no Spanner client")
	}
	var imps []SearchImpression
	impIter := s.Client.Single().Query(ctx, spanner.Statement{
		SQL: `SELECT impression_id, IFNULL(query_raw, ''), IFNULL(query_norm, ''), IFNULL(experiment_id, ''),
				IFNULL(retrieval_mode, ''), total_found, IFNULL(latency_ms, 0.0), result_ids, created_at
			FROM search_impressions
			WHERE tenant_id = @tenant_id AND created_at >= @since
			ORDER BY created_at DESC
			LIMIT 5000`,
		Params: map[string]interface{}{"tenant_id": tenant, "since": since},
	})
	defer impIter.Stop()
	for {
		row, err := impIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		var imp SearchImpression
		imp.TenantID = tenant
		if err := row.Columns(&imp.ImpressionID, &imp.QueryRaw, &imp.QueryNorm, &imp.ExperimentID, &imp.RetrievalMode, &imp.TotalFound, &imp.LatencyMs, &imp.ResultIDs, &imp.CreatedAt); err != nil {
			return nil, nil, err
		}
		imps = append(imps, imp)
	}

	var convs []SearchConversion
	convIter := s.Client.Single().Query(ctx, spanner.Statement{
		SQL: `SELECT event_id, impression_id, IFNULL(session_id, ''), event_type, product_id,
				position, IFNULL(experiment_id, ''), revenue_micros, created_at
			FROM search_conversions
			WHERE tenant_id = @tenant_id AND created_at >= @since
			ORDER BY created_at DESC
			LIMIT 10000`,
		Params: map[string]interface{}{"tenant_id": tenant, "since": since},
	})
	defer convIter.Stop()
	for {
		row, err := convIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		var c SearchConversion
		c.TenantID = tenant
		if err := row.Columns(&c.EventID, &c.ImpressionID, &c.SessionID, &c.EventType, &c.ProductID, &c.Position, &c.ExperimentID, &c.RevenueMicros, &c.CreatedAt); err != nil {
			return nil, nil, err
		}
		convs = append(convs, c)
	}
	return imps, convs, nil
}

// AnalyticsService coordinates non-blocking impression logging, idempotent conversion attribution,
// KPI summary calculation, and Laplace-smoothed LTR signal rollup (ADR-015, Issue #43).
type AnalyticsService struct {
	store      AnalyticsStore
	memMirror  *MemoryAnalyticsStore
	spannerSvc *SpannerService
	sessionSvc *SessionService
	impCh      chan SearchImpression
}

func NewAnalyticsService(store AnalyticsStore, spannerSvc *SpannerService, sessionSvc *SessionService) *AnalyticsService {
	mem := NewMemoryAnalyticsStore()
	if ms, ok := store.(*MemoryAnalyticsStore); ok {
		mem = ms
	}
	svc := &AnalyticsService{
		store:      store,
		memMirror:  mem,
		spannerSvc: spannerSvc,
		sessionSvc: sessionSvc,
		impCh:      make(chan SearchImpression, impressionBufferSize),
	}
	go svc.impressionWorker()
	return svc
}

func (a *AnalyticsService) impressionWorker() {
	for imp := range a.impCh {
		if a.store == nil {
			continue
		}
		if _, isMem := a.store.(*MemoryAnalyticsStore); isMem {
			continue // already written synchronously to memMirror
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := a.store.RecordImpression(ctx, imp); err != nil {
			log.Printf("WARN: async search impression write failed: %v", err)
		}
		cancel()
	}
}

// NormalizeQueryForAnalytics lowercases, trims, and accent-folds a search query for KPI grouping.
func NormalizeQueryForAnalytics(q string) string {
	return strings.Join(strings.Fields(FoldAccents(strings.ToLower(strings.TrimSpace(q)))), " ")
}

// EnqueueImpression logs a search impression without blocking the search response path (ADR-015).
func (a *AnalyticsService) EnqueueImpression(imp SearchImpression) {
	if a == nil {
		return
	}
	if imp.TenantID == "" {
		imp.TenantID = "default"
	}
	if imp.ImpressionID == "" {
		imp.ImpressionID = NewQueryID()
	}
	if imp.CreatedAt.IsZero() {
		imp.CreatedAt = time.Now().UTC()
	}
	if imp.QueryNorm == "" {
		imp.QueryNorm = NormalizeQueryForAnalytics(imp.QueryRaw)
	}
	_ = a.memMirror.RecordImpression(context.Background(), imp)
	select {
	case a.impCh <- imp:
	default:
		// Bounded channel full: never block /api/search tail latency.
	}
}

func deriveEventID(tenant, queryID, eventType, productID string, position int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%s|%d", tenant, queryID, eventType, productID, position)))
	return "evt-" + hex.EncodeToString(h[:12])
}

// RecordEvent validates and persists a click, add_to_cart, or purchase attribution event (ADR-015, #43).
func (a *AnalyticsService) RecordEvent(ctx context.Context, req models.AnalyticsEventRequest) (models.AnalyticsEventResponse, error) {
	tenant := strings.TrimSpace(req.TenantID)
	if tenant == "" {
		tenant = "default"
	}
	queryID := strings.TrimSpace(req.QueryID)
	if queryID == "" {
		queryID = strings.TrimSpace(req.ImpressionID)
	}
	if queryID == "" || len(queryID) > 128 {
		return models.AnalyticsEventResponse{}, errors.New("query_id is required (max 128 chars)")
	}
	eventType := strings.ToLower(strings.TrimSpace(req.EventType))
	switch eventType {
	case "click", "add_to_cart", "purchase":
	default:
		return models.AnalyticsEventResponse{}, fmt.Errorf("invalid event_type %q (want click | add_to_cart | purchase)", req.EventType)
	}
	productID := strings.TrimSpace(req.ProductID)
	if productID == "" || len(productID) > 1024 {
		return models.AnalyticsEventResponse{}, errors.New("product_id is required")
	}
	pos := req.Position
	if pos == 0 && (eventType == "add_to_cart" || eventType == "purchase") {
		pos = 1
	}
	if pos < 1 || pos > 1000 {
		return models.AnalyticsEventResponse{}, errors.New("position must be between 1 and 1000")
	}
	expID := strings.TrimSpace(req.ExperimentID)
	if err := ValidateExperimentID(expID); err != nil {
		return models.AnalyticsEventResponse{}, err
	}
	if req.RevenueMicros < 0 {
		return models.AnalyticsEventResponse{}, errors.New("revenue_micros must be >= 0")
	}
	eventID := strings.TrimSpace(req.EventID)
	if eventID == "" {
		eventID = deriveEventID(tenant, queryID, eventType, productID, pos)
	} else if len(eventID) > 64 {
		return models.AnalyticsEventResponse{}, errors.New("event_id must be <= 64 chars")
	}

	conv := SearchConversion{
		TenantID:      tenant,
		EventID:       eventID,
		ImpressionID:  queryID,
		SessionID:     strings.TrimSpace(req.SessionID),
		EventType:     eventType,
		ProductID:     productID,
		Position:      int64(pos),
		ExperimentID:  expID,
		RevenueMicros: req.RevenueMicros,
		CreatedAt:     time.Now().UTC(),
	}

	dup, err := a.store.RecordConversion(ctx, conv)
	if err != nil {
		return models.AnalyticsEventResponse{}, err
	}
	if _, isMem := a.store.(*MemoryAnalyticsStore); !isMem {
		_, _ = a.memMirror.RecordConversion(ctx, conv)
	}

	// Forward click/purchase to SessionService when session_id is supplied (ADR-015 §3.1).
	if !dup && a.sessionSvc != nil && conv.SessionID != "" && ValidSessionID(conv.SessionID) &&
		(eventType == "click" || eventType == "purchase") {
		_, _ = a.sessionSvc.Record(ctx, SessionEvent{
			TenantID:  tenant,
			SessionID: conv.SessionID,
			ProductID: productID,
			EventType: eventType,
		})
	}

	return models.AnalyticsEventResponse{
		EventID:       eventID,
		QueryID:       queryID,
		TenantID:      tenant,
		EventType:     eventType,
		ProductID:     productID,
		Position:      pos,
		ExperimentID:  expID,
		RevenueMicros: req.RevenueMicros,
		Duplicate:     dup,
		Recorded:      !dup,
	}, nil
}

// Summary computes tenant-scoped search KPIs, zero-result query backlog, MRR@10, and A/B breakdown (ADR-015, #43).
func (a *AnalyticsService) Summary(ctx context.Context, tenant string, windowHours int) (models.AnalyticsSummaryResponse, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "default"
	}
	if windowHours <= 0 {
		windowHours = defaultSummaryHours
	}
	if windowHours > maxSummaryHours {
		windowHours = maxSummaryHours
	}
	since := time.Now().UTC().Add(-time.Duration(windowHours) * time.Hour)

	imps, convs, err := a.store.ListWindow(ctx, tenant, since)
	if err != nil {
		// Fall back to in-memory mirror if Spanner telemetry tables are not yet migrated.
		imps, convs, _ = a.memMirror.ListWindow(ctx, tenant, since)
	}

	return ComputeAnalyticsSummary(tenant, windowHours, imps, convs), nil
}

// ComputeAnalyticsSummary is a pure function computing CTR, MRR@10, conversion rate,
// top queries, top zero-result queries, and A/B experiment breakdown from impressions and conversions.
func ComputeAnalyticsSummary(tenant string, windowHours int, imps []SearchImpression, convs []SearchConversion) models.AnalyticsSummaryResponse {
	impByID := make(map[string]SearchImpression, len(imps))
	for _, imp := range imps {
		impByID[imp.ImpressionID] = imp
	}

	// Map impression_id -> best clicked position (1..10), hasClick, hasPurchase, revenue
	type impFeedback struct {
		hasClick    bool
		hasPurchase bool
		bestPos10   int64
		clicks      int64
		purchases   int64
		revenue     int64
		expID       string
	}
	fbByImp := map[string]*impFeedback{}
	getFB := func(impID string) *impFeedback {
		fb, ok := fbByImp[impID]
		if !ok {
			fb = &impFeedback{}
			if imp, exists := impByID[impID]; exists {
				fb.expID = imp.ExperimentID
			}
			fbByImp[impID] = fb
		}
		return fb
	}

	var totalClicks, totalConversions, totalRevenue int64
	for _, c := range convs {
		fb := getFB(c.ImpressionID)
		if fb.expID == "" && c.ExperimentID != "" {
			fb.expID = c.ExperimentID
		}
		switch c.EventType {
		case "click":
			totalClicks++
			fb.hasClick = true
			fb.clicks++
			if c.Position >= 1 && c.Position <= 10 && (fb.bestPos10 == 0 || c.Position < fb.bestPos10) {
				fb.bestPos10 = c.Position
			}
		case "purchase":
			totalConversions++
			fb.hasPurchase = true
			fb.purchases++
			fb.revenue += c.RevenueMicros
			totalRevenue += c.RevenueMicros
		case "add_to_cart":
			fb.revenue += c.RevenueMicros
		}
	}

	var (
		totalSearches      = int64(len(imps))
		zeroResultSearches int64
		latencySum         float64
		clickedImps        int64
		convertedImps      int64
		mrrSum             float64
		mrrDenom           int64
	)

	type queryAgg struct {
		count  int64
		clicks int64
	}
	queryCounts := map[string]*queryAgg{}
	zeroQueryCounts := map[string]int64{}

	type expAgg struct {
		searches   int64
		zeroResult int64
		clicks     int64
		clickedImp int64
		purchases  int64
		convImp    int64
		mrrSum     float64
		mrrDenom   int64
		latSum     float64
		revenue    int64
	}
	expMap := map[string]*expAgg{}
	getExp := func(expID string) *expAgg {
		if expID == "" {
			expID = "default"
		}
		ea, ok := expMap[expID]
		if !ok {
			ea = &expAgg{}
			expMap[expID] = ea
		}
		return ea
	}

	for _, imp := range imps {
		latencySum += imp.LatencyMs
		qNorm := imp.QueryNorm
		if qNorm == "" {
			qNorm = NormalizeQueryForAnalytics(imp.QueryRaw)
		}
		fb := fbByImp[imp.ImpressionID]
		expKey := imp.ExperimentID
		if expKey == "" && fb != nil && fb.expID != "" {
			expKey = fb.expID
		}
		ea := getExp(expKey)
		ea.searches++
		ea.latSum += imp.LatencyMs

		if imp.TotalFound == 0 {
			zeroResultSearches++
			ea.zeroResult++
			if qNorm != "" {
				zeroQueryCounts[qNorm]++
			}
		} else if qNorm != "" {
			qa := queryCounts[qNorm]
			if qa == nil {
				qa = &queryAgg{}
				queryCounts[qNorm] = qa
			}
			qa.count++
			if fb != nil && fb.hasClick {
				qa.clicks++
			}
		}

		if fb != nil {
			ea.clicks += fb.clicks
			ea.purchases += fb.purchases
			ea.revenue += fb.revenue
			if fb.hasClick {
				clickedImps++
				ea.clickedImp++
			}
			if fb.hasPurchase {
				convertedImps++
				ea.convImp++
			}
			if fb.bestPos10 > 0 {
				rr := 1.0 / float64(fb.bestPos10)
				mrrSum += rr
				mrrDenom++
				ea.mrrSum += rr
				ea.mrrDenom++
			} else if fb.hasClick {
				mrrDenom++
				ea.mrrDenom++
			}
		}
	}

	// Also account for conversion events referencing impressions that haven't flushed yet or came from external query_ids.
	for impID, fb := range fbByImp {
		if _, seen := impByID[impID]; seen {
			continue
		}
		totalSearches++
		ea := getExp(fb.expID)
		ea.searches++
		ea.clicks += fb.clicks
		ea.purchases += fb.purchases
		ea.revenue += fb.revenue
		if fb.hasClick {
			clickedImps++
			ea.clickedImp++
		}
		if fb.hasPurchase {
			convertedImps++
			ea.convImp++
		}
		if fb.bestPos10 > 0 {
			rr := 1.0 / float64(fb.bestPos10)
			mrrSum += rr
			mrrDenom++
			ea.mrrSum += rr
			ea.mrrDenom++
		}
	}

	var zeroRate, avgLat, ctr, convRate, mrrAt10 float64
	if totalSearches > 0 {
		zeroRate = round4(float64(zeroResultSearches) / float64(totalSearches))
		avgLat = round2(latencySum / float64(totalSearches))
		ctr = round4(clamp(float64(clickedImps)/float64(totalSearches), 0, 1))
		convRate = round4(clamp(float64(convertedImps)/float64(totalSearches), 0, 1))
	}
	if mrrDenom > 0 {
		mrrAt10 = round4(mrrSum / float64(mrrDenom))
	}

	topQueries := make([]models.QueryMetric, 0, len(queryCounts))
	for q, qa := range queryCounts {
		qCTR := 0.0
		if qa.count > 0 {
			qCTR = round4(float64(qa.clicks) / float64(qa.count))
		}
		topQueries = append(topQueries, models.QueryMetric{Query: q, Count: qa.count, CTR: qCTR})
	}
	sort.Slice(topQueries, func(i, j int) bool {
		if topQueries[i].Count != topQueries[j].Count {
			return topQueries[i].Count > topQueries[j].Count
		}
		return topQueries[i].Query < topQueries[j].Query
	})
	if len(topQueries) > 10 {
		topQueries = topQueries[:10]
	}

	topZero := make([]models.QueryMetric, 0, len(zeroQueryCounts))
	for q, cnt := range zeroQueryCounts {
		topZero = append(topZero, models.QueryMetric{Query: q, Count: cnt})
	}
	sort.Slice(topZero, func(i, j int) bool {
		if topZero[i].Count != topZero[j].Count {
			return topZero[i].Count > topZero[j].Count
		}
		return topZero[i].Query < topZero[j].Query
	})
	if len(topZero) > 10 {
		topZero = topZero[:10]
	}

	experiments := make([]models.ExperimentSummary, 0, len(expMap))
	for expID, ea := range expMap {
		var eZeroRate, eCTR, eConvRate, eMRR, eLat float64
		if ea.searches > 0 {
			eZeroRate = round4(float64(ea.zeroResult) / float64(ea.searches))
			eCTR = round4(clamp(float64(ea.clickedImp)/float64(ea.searches), 0, 1))
			eConvRate = round4(clamp(float64(ea.convImp)/float64(ea.searches), 0, 1))
			eLat = round2(ea.latSum / float64(ea.searches))
		}
		if ea.mrrDenom > 0 {
			eMRR = round4(ea.mrrSum / float64(ea.mrrDenom))
		}
		experiments = append(experiments, models.ExperimentSummary{
			ExperimentID:            expID,
			Searches:                ea.searches,
			TotalSearches:           ea.searches,
			ZeroResultSearches:      ea.zeroResult,
			ZeroResultRate:          eZeroRate,
			Clicks:                  ea.clicks,
			Conversions:             ea.purchases,
			ClickThroughRate:        eCTR,
			CTR:                     eCTR,
			ConversionRate:          eConvRate,
			MRRAt10:                 eMRR,
			AvgLatencyMs:            eLat,
			AttributedRevenueMicros: ea.revenue,
		})
	}
	sort.Slice(experiments, func(i, j int) bool {
		if experiments[i].Searches != experiments[j].Searches {
			return experiments[i].Searches > experiments[j].Searches
		}
		return experiments[i].ExperimentID < experiments[j].ExperimentID
	})

	return models.AnalyticsSummaryResponse{
		TenantID:                tenant,
		WindowHours:             windowHours,
		TotalSearches:           totalSearches,
		ZeroResultSearches:      zeroResultSearches,
		ZeroResultRate:          zeroRate,
		AvgLatencyMs:            avgLat,
		TotalClicks:             totalClicks,
		TotalConversions:        totalConversions,
		ClickThroughRate:        ctr,
		CTR:                     ctr,
		ConversionRate:          convRate,
		MRRAt10:                 mrrAt10,
		AttributedRevenueMicros: totalRevenue,
		TopQueries:              topQueries,
		TopZeroResultQueries:    topZero,
		Experiments:             experiments,
	}
}

func round4(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return math.Round(v*10000) / 10000
}

// RollupProductSignals aggregates recent search_impressions and search_conversions into
// Laplace-smoothed product engagement signals `(clicks + 1) / (impressions + 20)` and updates
// Spanner / in-memory product rows (ADR-015 §3.2, Issue #43).
func (a *AnalyticsService) RollupProductSignals(ctx context.Context, tenant string, windowDays int) (models.AnalyticsRollupResponse, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = "default"
	}
	if windowDays <= 0 {
		windowDays = defaultRollupWindowDays
	}
	if windowDays > 90 {
		windowDays = 90
	}
	since := time.Now().UTC().Add(-time.Duration(windowDays) * 24 * time.Hour)

	imps, convs, err := a.store.ListWindow(ctx, tenant, since)
	if err != nil {
		imps, convs, _ = a.memMirror.ListWindow(ctx, tenant, since)
	}

	type prodExposure struct {
		impressions int64
		clicks      int64
		purchases   int64
	}
	byProd := map[string]*prodExposure{}
	getProd := func(pid string) *prodExposure {
		pe, ok := byProd[pid]
		if !ok {
			pe = &prodExposure{}
			byProd[pid] = pe
		}
		return pe
	}

	for _, imp := range imps {
		for _, pid := range imp.ResultIDs {
			if pid != "" {
				getProd(pid).impressions++
			}
		}
	}
	for _, c := range convs {
		if c.ProductID == "" {
			continue
		}
		pe := getProd(c.ProductID)
		switch c.EventType {
		case "click":
			pe.clicks++
		case "purchase":
			pe.purchases++
		}
	}

	updated := map[string]models.BoostSignals{}
	var muts []*spanner.Mutation

	if a.spannerSvc != nil {
		a.spannerSvc.attrMu.Lock()
		for i := range a.spannerSvc.seedProducts {
			p := &a.spannerSvc.seedProducts[i]
			pe, ok := byProd[p.ID]
			if !ok {
				continue
			}
			var sig models.BoostSignals
			if p.Signals != nil {
				sig = *p.Signals
			}
			sig.Clicks += pe.clicks
			sig.Orders += pe.purchases
			// Laplace-smoothed CTR and conversion rate: (events + 1) / (impressions + 20)
			denom := float64(pe.impressions + 20)
			sig.CTR = round4(clamp(float64(pe.clicks+1)/denom, 0, 1))
			sig.ConversionRate = round4(clamp(float64(pe.purchases+1)/denom, 0, 1))
			cpSig := sig
			p.Signals = &cpSig
			updated[p.ID] = cpSig
		}
		a.spannerSvc.attrMu.Unlock()
	}

	for pid, pe := range byProd {
		if _, already := updated[pid]; !already {
			denom := float64(pe.impressions + 20)
			sig := models.BoostSignals{
				Clicks:         pe.clicks,
				Orders:         pe.purchases,
				CTR:            round4(clamp(float64(pe.clicks+1)/denom, 0, 1)),
				ConversionRate: round4(clamp(float64(pe.purchases+1)/denom, 0, 1)),
			}
			updated[pid] = sig
		}
		if a.spannerSvc != nil && a.spannerSvc.Client() != nil {
			sig := updated[pid]
			muts = append(muts, spanner.Update("products",
				[]string{"product_id", "clicks", "orders", "ctr", "conversion_rate"},
				[]interface{}{pid, sig.Clicks, sig.Orders, sig.CTR, sig.ConversionRate},
			))
		}
	}

	if a.spannerSvc != nil && a.spannerSvc.Client() != nil && len(muts) > 0 {
		for i := 0; i < len(muts); i += 200 {
			end := i + 200
			if end > len(muts) {
				end = len(muts)
			}
			if _, err := a.spannerSvc.Client().Apply(ctx, muts[i:end]); err != nil {
				log.Printf("WARN: analytics rollup Spanner apply: %v", err)
				break
			}
		}
	}

	return models.AnalyticsRollupResponse{
		TenantID:        tenant,
		WindowDays:      windowDays,
		ProductsUpdated: len(updated),
		UpdatedSignals:  updated,
	}, nil
}

// CheckSpannerAnalytics validates search_impressions and search_conversions read queries (cmd/sqlcheck, ADR-015 #43).
func CheckSpannerAnalytics(ctx context.Context, client *spanner.Client) (int, int, error) {
	if client == nil {
		return 0, 0, errors.New("no Spanner client (memory mode)")
	}
	store := SpannerAnalyticsStore{Client: client}
	imps, convs, err := store.ListWindow(ctx, "default", time.Now().UTC().Add(-24*time.Hour))
	if err != nil {
		return 0, 0, err
	}
	return len(imps), len(convs), nil
}
