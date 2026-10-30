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
	DefaultRollupPriorK     = 100.0 // Issue #47: Bayesian prior strength k in (events + k*prior)/(impressions + k)
)

// RollupOptions configures POST /api/analytics/rollup (ADR-015, Issues #43/#47).
type RollupOptions struct {
	TenantID    string  `json:"tenant_id"`
	WindowDays  int     `json:"window_days"`
	PriorWeight float64 `json:"prior_weight"`
	DryRun      bool    `json:"dry_run"`
}

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

// ErrInvalidRollupParam indicates an invalid prior_weight/k or window_days parameter (maps to HTTP 400; #47 fix).
var ErrInvalidRollupParam = errors.New("invalid rollup parameter")

// ValidateRollupParams checks window_days in [1, 90] and prior_weight/k finite in (0, 10000] (#47 fix).
func ValidateRollupParams(windowDays int, priorWeight float64) error {
	if windowDays < 1 || windowDays > 90 {
		return fmt.Errorf("%w: window_days must be between 1 and 90", ErrInvalidRollupParam)
	}
	if math.IsNaN(priorWeight) || math.IsInf(priorWeight, 0) || priorWeight <= 0 || priorWeight > 10000 {
		return fmt.Errorf("%w: prior_weight (k) must be a finite number in (0, 10000]", ErrInvalidRollupParam)
	}
	return nil
}

// AnalyticsService coordinates non-blocking impression logging, idempotent conversion attribution,
// KPI summary calculation, and Bayesian prior-blended LTR signal rollup (ADR-015, Issues #43/#47).
type AnalyticsService struct {
	store      AnalyticsStore
	memMirror  *MemoryAnalyticsStore
	spannerSvc *SpannerService
	sessionSvc *SessionService
	impCh      chan SearchImpression
	baselineMu sync.RWMutex
	baselines  map[string]models.BoostSignals // frozen baseline signals captured at first live rollup (#47 fix)
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
		baselines:  map[string]models.BoostSignals{},
	}
	go svc.impressionWorker()
	return svc
}

// HasMemoryBaselineForTest reports whether a frozen baseline snapshot exists in memory for productID (#47 fix).
func (a *AnalyticsService) HasMemoryBaselineForTest(productID string) bool {
	a.baselineMu.RLock()
	defer a.baselineMu.RUnlock()
	_, ok := a.baselines[productID]
	return ok
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

// RollupProductSignals wraps RollupProductSignalsWithOptions using default prior weight k=100 and DryRun=false.
func (a *AnalyticsService) RollupProductSignals(ctx context.Context, tenant string, windowDays int) (models.AnalyticsRollupResponse, error) {
	return a.RollupProductSignalsWithOptions(ctx, RollupOptions{
		TenantID:    tenant,
		WindowDays:  windowDays,
		PriorWeight: DefaultRollupPriorK,
		DryRun:      false,
	})
}

// RollupProductSignalsWithOptions aggregates recent search_impressions and search_conversions into
// Bayesian-blended product engagement signals `(events + k * baseline) / (impressions + k)` as an
// idempotent window overlay over a frozen baseline (ADR-015, Issue #47):
//   - Products with zero new impressions (pe.impressions == 0) are NEVER touched.
//   - On the first live rollup touching a product, its current (clicks, orders, ctr, conversion_rate)
//     are snapshotted into baseline_*; all subsequent rollups compute strictly from the frozen baseline
//     so re-running rollup over the same window never double-counts or compounds drift.
//   - When opts.DryRun is true, returns the proposed diffs and UpdatedSignals without writing to
//     memory or Spanner and without snapshotting the baseline.
func (a *AnalyticsService) RollupProductSignalsWithOptions(ctx context.Context, opts RollupOptions) (models.AnalyticsRollupResponse, error) {
	tenant := strings.TrimSpace(opts.TenantID)
	if tenant == "" {
		tenant = "default"
	}
	windowDays := opts.WindowDays
	if windowDays == 0 {
		windowDays = defaultRollupWindowDays
	}
	priorK := opts.PriorWeight
	if priorK == 0 {
		priorK = DefaultRollupPriorK
	}
	if err := ValidateRollupParams(windowDays, priorK); err != nil {
		return models.AnalyticsRollupResponse{}, err
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
		seenInImp := map[string]bool{}
		for _, pid := range imp.ResultIDs {
			if pid != "" && !seenInImp[pid] {
				seenInImp[pid] = true
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

	// Only consider products that have at least 1 search impression in the window (#47).
	var activeIDs []string
	for pid, pe := range byProd {
		if pe.impressions > 0 {
			activeIDs = append(activeIDs, pid)
		}
	}
	sort.Strings(activeIDs)

	if len(activeIDs) == 0 {
		return models.AnalyticsRollupResponse{
			TenantID:        tenant,
			WindowDays:      windowDays,
			PriorWeight:     priorK,
			DryRun:          opts.DryRun,
			ProductsUpdated: 0,
			UpdatedSignals:  map[string]models.BoostSignals{},
			Diffs:           []models.SignalRollupDiff{},
		}, nil
	}

	computeFromBaseline := func(pid string, base models.BoostSignals, pe *prodExposure) (models.BoostSignals, models.SignalRollupDiff) {
		baseCTR := base.CTR
		if baseCTR <= 0 {
			baseCTR = 0.05
		}
		baseConv := base.ConversionRate
		if baseConv <= 0 {
			baseConv = 0.01
		}
		denom := float64(pe.impressions) + priorK
		obsClicks := pe.clicks
		if obsClicks > pe.impressions {
			obsClicks = pe.impressions
		}
		obsPurchases := pe.purchases
		if obsPurchases > pe.impressions {
			obsPurchases = pe.impressions
		}
		newCTR := round4(clamp((float64(obsClicks)+priorK*baseCTR)/denom, 0, 1))
		newConv := round4(clamp((float64(obsPurchases)+priorK*baseConv)/denom, 0, 1))
		newSig := base
		newSig.Clicks = base.Clicks + pe.clicks
		newSig.Orders = base.Orders + pe.purchases
		newSig.CTR = newCTR
		newSig.ConversionRate = newConv
		diff := models.SignalRollupDiff{
			ProductID:           pid,
			Impressions:         pe.impressions,
			AttributedClicks:    pe.clicks,
			AttributedPurchases: pe.purchases,
			PriorClicks:         base.Clicks,
			NewClicks:           newSig.Clicks,
			PriorOrders:         base.Orders,
			NewOrders:           newSig.Orders,
			PriorCTR:            base.CTR,
			NewCTR:              newSig.CTR,
			PriorConversionRate: base.ConversionRate,
			NewConversionRate:   newSig.ConversionRate,
		}
		return newSig, diff
	}

	// Spanner mode: read/snapshot frozen baseline_* columns and update derived signals in ReadWriteTransaction (#47 fix).
	if a.spannerSvc != nil && a.spannerSvc.Client() != nil {
		client := a.spannerSvc.Client()
		updated := make(map[string]models.BoostSignals, len(activeIDs))
		diffs := make([]models.SignalRollupDiff, 0, len(activeIDs))

		if opts.DryRun {
			iter := client.Single().Query(ctx, spanner.Statement{
				SQL: `SELECT product_id,
						COALESCE(baseline_clicks, clicks, 0),
						COALESCE(baseline_orders, orders, 0),
						COALESCE(baseline_ctr, ctr, 0.05),
						COALESCE(baseline_conversion_rate, conversion_rate, 0.01),
						IFNULL(discount_pct, 0.0), IFNULL(popularity_score, 0.0), IFNULL(margin_pct, 0.0), IFNULL(recency_score, 0.0)
					FROM products
					WHERE product_id IN UNNEST(@ids)
					ORDER BY product_id`,
				Params: map[string]interface{}{"ids": activeIDs},
			})
			defer iter.Stop()
			for {
				row, rerr := iter.Next()
				if rerr != nil {
					break
				}
				var pid string
				var base models.BoostSignals
				if row.Columns(&pid, &base.Clicks, &base.Orders, &base.CTR, &base.ConversionRate, &base.DiscountPct, &base.PopularityScore, &base.MarginPct, &base.RecencyScore) == nil && pid != "" {
					if pe := byProd[pid]; pe != nil {
						newSig, diff := computeFromBaseline(pid, base, pe)
						updated[pid] = newSig
						diffs = append(diffs, diff)
					}
				}
			}
			return models.AnalyticsRollupResponse{
				TenantID:        tenant,
				WindowDays:      windowDays,
				PriorWeight:     priorK,
				DryRun:          true,
				ProductsUpdated: len(updated),
				UpdatedSignals:  updated,
				Diffs:           diffs,
			}, nil
		}

		for i := 0; i < len(activeIDs); i += 100 {
			end := i + 100
			if end > len(activeIDs) {
				end = len(activeIDs)
			}
			chunkIDs := activeIDs[i:end]
			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
				iter := txn.Query(ctx, spanner.Statement{
					SQL: `SELECT product_id,
							IFNULL(clicks, 0), IFNULL(orders, 0), IFNULL(ctr, 0.05), IFNULL(conversion_rate, 0.01),
							baseline_clicks, baseline_orders, baseline_ctr, baseline_conversion_rate,
							IFNULL(discount_pct, 0.0), IFNULL(popularity_score, 0.0), IFNULL(margin_pct, 0.0), IFNULL(recency_score, 0.0)
						FROM products
						WHERE product_id IN UNNEST(@ids)
						ORDER BY product_id`,
					Params: map[string]interface{}{"ids": chunkIDs},
				})
				defer iter.Stop()
				var muts []*spanner.Mutation
				for {
					row, rerr := iter.Next()
					if rerr == iterator.Done {
						break
					}
					if rerr != nil {
						return rerr
					}
					var pid string
					var cur models.BoostSignals
					var bClicks, bOrders spanner.NullInt64
					var bCTR, bConv spanner.NullFloat64
					if err := row.Columns(&pid, &cur.Clicks, &cur.Orders, &cur.CTR, &cur.ConversionRate,
						&bClicks, &bOrders, &bCTR, &bConv,
						&cur.DiscountPct, &cur.PopularityScore, &cur.MarginPct, &cur.RecencyScore); err != nil {
						return err
					}
					pe := byProd[pid]
					if pe == nil {
						continue
					}
					needSnapshot := !bCTR.Valid || !bConv.Valid || !bClicks.Valid || !bOrders.Valid
					base := cur
					if !needSnapshot {
						base.Clicks = bClicks.Int64
						base.Orders = bOrders.Int64
						base.CTR = bCTR.Float64
						base.ConversionRate = bConv.Float64
					}
					newSig, diff := computeFromBaseline(pid, base, pe)
					updated[pid] = newSig
					diffs = append(diffs, diff)
					if needSnapshot {
						muts = append(muts, spanner.Update("products",
							[]string{"product_id", "baseline_clicks", "baseline_orders", "baseline_ctr", "baseline_conversion_rate", "clicks", "orders", "ctr", "conversion_rate"},
							[]interface{}{pid, base.Clicks, base.Orders, base.CTR, base.ConversionRate, newSig.Clicks, newSig.Orders, newSig.CTR, newSig.ConversionRate},
						))
					} else {
						muts = append(muts, spanner.Update("products",
							[]string{"product_id", "clicks", "orders", "ctr", "conversion_rate"},
							[]interface{}{pid, newSig.Clicks, newSig.Orders, newSig.CTR, newSig.ConversionRate},
						))
					}
				}
				return txn.BufferWrite(muts)
			})
			if err != nil {
				log.Printf("WARN: analytics rollup Spanner ReadWriteTransaction failed: %v", err)
				break
			}
		}

		return models.AnalyticsRollupResponse{
			TenantID:        tenant,
			WindowDays:      windowDays,
			PriorWeight:     priorK,
			DryRun:          false,
			ProductsUpdated: len(updated),
			UpdatedSignals:  updated,
			Diffs:           diffs,
		}, nil
	}

	// Memory mode: snapshot immutable baseline on first live rollup only; dry_run never snapshots baseline (#47 fix).
	currentSeeds := map[string]models.BoostSignals{}
	if a.spannerSvc != nil {
		a.spannerSvc.attrMu.RLock()
		for i := range a.spannerSvc.seedProducts {
			p := &a.spannerSvc.seedProducts[i]
			if p.Signals != nil {
				currentSeeds[p.ID] = *p.Signals
			}
		}
		a.spannerSvc.attrMu.RUnlock()
	}

	updated := make(map[string]models.BoostSignals, len(activeIDs))
	diffs := make([]models.SignalRollupDiff, 0, len(activeIDs))

	a.baselineMu.Lock()
	for _, pid := range activeIDs {
		pe := byProd[pid]
		base, hasBase := a.baselines[pid]
		if !hasBase {
			if cur, ok := currentSeeds[pid]; ok {
				base = cur
			} else {
				base = models.BoostSignals{CTR: 0.05, ConversionRate: 0.01}
			}
			if !opts.DryRun {
				a.baselines[pid] = base
			}
		}
		newSig, diff := computeFromBaseline(pid, base, pe)
		updated[pid] = newSig
		diffs = append(diffs, diff)
	}
	a.baselineMu.Unlock()

	if !opts.DryRun && a.spannerSvc != nil {
		a.spannerSvc.attrMu.Lock()
		for i := range a.spannerSvc.seedProducts {
			pid := a.spannerSvc.seedProducts[i].ID
			if sig, ok := updated[pid]; ok {
				cpSig := sig
				a.spannerSvc.seedProducts[i].Signals = &cpSig
			}
		}
		a.spannerSvc.attrMu.Unlock()
	}

	return models.AnalyticsRollupResponse{
		TenantID:        tenant,
		WindowDays:      windowDays,
		PriorWeight:     priorK,
		DryRun:          opts.DryRun,
		ProductsUpdated: len(updated),
		UpdatedSignals:  updated,
		Diffs:           diffs,
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
