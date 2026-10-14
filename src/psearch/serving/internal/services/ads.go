package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"psearch/serving-go/internal/models"
)

// Sponsored Products / Retail Media (ADR-011, Issue #35).

var (
	// DefaultAdSlots are the default 1-based positions for sponsored products.
	DefaultAdSlots = []int{1, 4, 8}
	// DefaultAdMax is the default maximum number of sponsored items per search response.
	DefaultAdMax = 3
	// DefaultAdMinSimilarity is the default minimum cosine similarity for vector ad matching.
	DefaultAdMinSimilarity = 0.55

	// ErrCampaignNotFound is returned when a campaign_id does not exist.
	ErrCampaignNotFound = errors.New("ad campaign not found")
)

// DefaultAdCampaigns returns the baseline demo campaigns (mirrored by migration 023b).
func DefaultAdCampaigns() []models.AdCampaign {
	return []models.AdCampaign{
		{
			CampaignID:   "camp-nike-pegasus",
			TenantID:     "default",
			Advertiser:   "Nike Retail Media",
			ProductID:    "prod-003",
			CPCMicros:    400000,
			BudgetMicros: 5000000,
			SpentMicros:  0,
			Status:       "active",
		},
		{
			CampaignID:   "camp-asics-nimbus",
			TenantID:     "default",
			Advertiser:   "Asics Brasil",
			ProductID:    "prod-002",
			CPCMicros:    250000,
			BudgetMicros: 4000000,
			SpentMicros:  0,
			Status:       "active",
		},
		{
			CampaignID:   "camp-apple-macbook",
			TenantID:     "default",
			Advertiser:   "Apple Enterprise",
			ProductID:    "prod-007",
			CPCMicros:    500000,
			BudgetMicros: 10000000,
			SpentMicros:  0,
			Status:       "active",
		},
		{
			CampaignID:   "camp-acer-predator",
			TenantID:     "default",
			Advertiser:   "Acer Gaming",
			ProductID:    "prod-006",
			CPCMicros:    350000,
			BudgetMicros: 6000000,
			SpentMicros:  0,
			Status:       "active",
		},
		{
			CampaignID:   "camp-bose-qc",
			TenantID:     "default",
			Advertiser:   "Bose Audio",
			ProductID:    "prod-008",
			CPCMicros:    300000,
			BudgetMicros: 4500000,
			SpentMicros:  0,
			Status:       "active",
		},
	}
}

// AdStore persists ad campaigns and executes atomic CPC click billing.
type AdStore interface {
	List(ctx context.Context, tenantID string) ([]models.AdCampaign, error)
	Upsert(ctx context.Context, c models.AdCampaign) (models.AdCampaign, error)
	Delete(ctx context.Context, tenantID, campaignID string) error
	RecordClick(ctx context.Context, campaignID, clickID string) (models.AdClickResponse, error)
}

func newCampaignID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "camp-" + hex.EncodeToString(b)
}

func newClickID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "clk-" + hex.EncodeToString(b)
}

func parseOptionalTime(raw *string) (*time.Time, *string, error) {
	if raw == nil {
		return nil, nil, nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil, nil, nil
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			formatted := utc.Format(time.RFC3339)
			return &utc, &formatted, nil
		}
	}
	return nil, nil, fmt.Errorf("invalid timestamp %q (expected RFC3339)", s)
}

func campaignWithinWindow(c models.AdCampaign, now time.Time) bool {
	if c.StartAt != nil && *c.StartAt != "" {
		if t, _, err := parseOptionalTime(c.StartAt); err == nil && t != nil && now.Before(*t) {
			return false
		}
	}
	if c.EndAt != nil && *c.EndAt != "" {
		if t, _, err := parseOptionalTime(c.EndAt); err == nil && t != nil && now.After(*t) {
			return false
		}
	}
	return true
}

// AdService validates campaigns, manages the store, and coordinates click billing.
type AdService struct {
	store         AdStore
	client        *spanner.Client
	mu            sync.RWMutex
	knownProducts map[string]bool
}

// NewAdService creates an AdService backed by store and seeded with catalog products.
func NewAdService(store AdStore, products []models.SearchResult, client ...*spanner.Client) *AdService {
	if store == nil {
		store = NewMemoryAdStore()
	}
	known := make(map[string]bool, len(products))
	for _, p := range products {
		if p.ID != "" {
			known[p.ID] = true
		}
	}
	var spClient *spanner.Client
	if len(client) > 0 {
		spClient = client[0]
	}
	return &AdService{
		store:         store,
		client:        spClient,
		knownProducts: known,
	}
}

func (s *AdService) isKnownProduct(ctx context.Context, productID string) bool {
	s.mu.RLock()
	ok := s.knownProducts[productID]
	s.mu.RUnlock()
	if ok {
		return true
	}
	if s.client != nil {
		row, err := s.client.Single().ReadRow(ctx, "products", spanner.Key{productID}, []string{"product_id"})
		if err == nil && row != nil {
			s.mu.Lock()
			s.knownProducts[productID] = true
			s.mu.Unlock()
			return true
		}
	}
	return false
}

// List returns all campaigns for a tenant.
func (s *AdService) List(ctx context.Context, tenantID string) ([]models.AdCampaign, error) {
	if s == nil || s.store == nil {
		return []models.AdCampaign{}, nil
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	return s.store.List(ctx, tenantID)
}

// Save validates and upserts an ad campaign (ADR-011 section 5).
func (s *AdService) Save(ctx context.Context, c models.AdCampaign) (models.AdCampaign, error) {
	c.TenantID = strings.TrimSpace(c.TenantID)
	if c.TenantID == "" {
		c.TenantID = "default"
	}
	if len(c.TenantID) > 64 {
		return c, fmt.Errorf("tenant_id must be <= 64 chars")
	}
	c.CampaignID = strings.TrimSpace(c.CampaignID)
	if c.CampaignID == "" {
		c.CampaignID = newCampaignID()
	}
	if len(c.CampaignID) > 64 {
		return c, fmt.Errorf("campaign_id must be <= 64 chars")
	}
	c.Advertiser = strings.TrimSpace(c.Advertiser)
	if len(c.Advertiser) > 128 {
		c.Advertiser = c.Advertiser[:128]
	}
	c.ProductID = strings.TrimSpace(c.ProductID)
	if c.ProductID == "" || len(c.ProductID) > 64 {
		return c, fmt.Errorf("product_id is required (<= 64 chars)")
	}
	if !s.isKnownProduct(ctx, c.ProductID) {
		return c, fmt.Errorf("unknown product_id %q", c.ProductID)
	}
	if c.CPCMicros <= 0 {
		return c, fmt.Errorf("cpc_micros must be > 0")
	}
	if c.BudgetMicros < c.CPCMicros {
		return c, fmt.Errorf("budget_micros must be >= cpc_micros")
	}
	if c.SpentMicros < 0 || c.SpentMicros > c.BudgetMicros {
		return c, fmt.Errorf("spent_micros must be between 0 and budget_micros")
	}
	c.Status = strings.ToLower(strings.TrimSpace(c.Status))
	if c.Status == "" {
		c.Status = "active"
	}
	switch c.Status {
	case "active", "paused", "exhausted":
	default:
		return c, fmt.Errorf("status must be active, paused, or exhausted")
	}
	if c.Status == "active" && c.SpentMicros+c.CPCMicros > c.BudgetMicros {
		c.Status = "exhausted"
	}
	startTime, startStr, err := parseOptionalTime(c.StartAt)
	if err != nil {
		return c, fmt.Errorf("start_at: %w", err)
	}
	endTime, endStr, err := parseOptionalTime(c.EndAt)
	if err != nil {
		return c, fmt.Errorf("end_at: %w", err)
	}
	if startTime != nil && endTime != nil && startTime.After(*endTime) {
		return c, fmt.Errorf("start_at must be <= end_at")
	}
	c.StartAt = startStr
	c.EndAt = endStr
	return s.store.Upsert(ctx, c)
}

// Delete removes a campaign.
func (s *AdService) Delete(ctx context.Context, tenantID, campaignID string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	campaignID = strings.TrimSpace(campaignID)
	if campaignID == "" {
		return fmt.Errorf("campaign_id is required")
	}
	return s.store.Delete(ctx, tenantID, campaignID)
}

// RecordClick validates click parameters and atomically burns CPC budget.
func (s *AdService) RecordClick(ctx context.Context, campaignID, clickID string) (models.AdClickResponse, error) {
	campaignID = strings.TrimSpace(campaignID)
	clickID = strings.TrimSpace(clickID)
	if campaignID == "" || len(campaignID) > 64 {
		return models.AdClickResponse{}, fmt.Errorf("campaign_id is required (<= 64 chars)")
	}
	if clickID == "" || len(clickID) > 64 {
		return models.AdClickResponse{}, fmt.Errorf("click_id is required (<= 64 chars)")
	}
	return s.store.RecordClick(ctx, campaignID, clickID)
}

// ---------------------------------------------------------------------------
// MemoryAdStore (ADR-011 section 7 — offline parity with Spanner)
// ---------------------------------------------------------------------------

type MemoryAdStore struct {
	mu        sync.Mutex
	campaigns map[string]models.AdCampaign
	clicks    map[string]models.AdClickResponse // key: campaign_id + "|" + click_id
}

// NewMemoryAdStore creates a MemoryAdStore seeded with DefaultAdCampaigns().
func NewMemoryAdStore() *MemoryAdStore {
	m := NewEmptyMemoryAdStore()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range DefaultAdCampaigns() {
		u := now
		c.UpdatedAt = &u
		m.campaigns[c.CampaignID] = c
	}
	return m
}

// NewEmptyMemoryAdStore creates an unseeded MemoryAdStore for isolated tests.
func NewEmptyMemoryAdStore() *MemoryAdStore {
	return &MemoryAdStore{
		campaigns: map[string]models.AdCampaign{},
		clicks:    map[string]models.AdClickResponse{},
	}
}

func (m *MemoryAdStore) List(_ context.Context, tenantID string) ([]models.AdCampaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if tenantID == "" {
		tenantID = "default"
	}
	out := []models.AdCampaign{}
	for _, c := range m.campaigns {
		t := c.TenantID
		if t == "" {
			t = "default"
		}
		if t == tenantID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CampaignID < out[j].CampaignID })
	return out, nil
}

func (m *MemoryAdStore) Upsert(_ context.Context, c models.AdCampaign) (models.AdCampaign, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC().Format(time.RFC3339)
	c.UpdatedAt = &now
	if c.Status == "active" && c.SpentMicros+c.CPCMicros > c.BudgetMicros {
		c.Status = "exhausted"
	}
	m.campaigns[c.CampaignID] = c
	return c, nil
}

func (m *MemoryAdStore) Delete(_ context.Context, tenantID, campaignID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.campaigns[campaignID]; ok {
		if tenantID != "" && existing.TenantID != "" && existing.TenantID != tenantID {
			return nil
		}
		delete(m.campaigns, campaignID)
		prefix := campaignID + "|"
		for k := range m.clicks {
			if strings.HasPrefix(k, prefix) {
				delete(m.clicks, k)
			}
		}
	}
	return nil
}

func (m *MemoryAdStore) RecordClick(_ context.Context, campaignID, clickID string) (models.AdClickResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.campaigns[campaignID]
	if !ok {
		return models.AdClickResponse{}, ErrCampaignNotFound
	}
	clickKey := campaignID + "|" + clickID
	if _, dup := m.clicks[clickKey]; dup {
		return models.AdClickResponse{
			CampaignID:      campaignID,
			ClickID:         clickID,
			ProductID:       c.ProductID,
			Charged:         false,
			CostMicros:      0,
			SpentMicros:     c.SpentMicros,
			BudgetMicros:    c.BudgetMicros,
			RemainingMicros: c.BudgetMicros - c.SpentMicros,
			Status:          c.Status,
			Reason:          "duplicate_click",
		}, nil
	}
	now := time.Now().UTC()
	if c.Status != "active" || !campaignWithinWindow(c, now) || c.SpentMicros+c.CPCMicros > c.BudgetMicros {
		if c.Status == "active" && c.SpentMicros+c.CPCMicros > c.BudgetMicros {
			c.Status = "exhausted"
			ts := now.Format(time.RFC3339)
			c.UpdatedAt = &ts
			m.campaigns[campaignID] = c
		}
		reason := "campaign_not_active"
		if c.Status == "exhausted" || c.SpentMicros+c.CPCMicros > c.BudgetMicros {
			reason = "budget_exhausted"
		}
		return models.AdClickResponse{
			CampaignID:      campaignID,
			ClickID:         clickID,
			ProductID:       c.ProductID,
			Charged:         false,
			CostMicros:      0,
			SpentMicros:     c.SpentMicros,
			BudgetMicros:    c.BudgetMicros,
			RemainingMicros: c.BudgetMicros - c.SpentMicros,
			Status:          c.Status,
			Reason:          reason,
		}, nil
	}

	c.SpentMicros += c.CPCMicros
	if c.SpentMicros+c.CPCMicros > c.BudgetMicros {
		c.Status = "exhausted"
	}
	ts := now.Format(time.RFC3339)
	c.UpdatedAt = &ts
	m.campaigns[campaignID] = c

	res := models.AdClickResponse{
		CampaignID:      campaignID,
		ClickID:         clickID,
		ProductID:       c.ProductID,
		Charged:         true,
		CostMicros:      c.CPCMicros,
		SpentMicros:     c.SpentMicros,
		BudgetMicros:    c.BudgetMicros,
		RemainingMicros: c.BudgetMicros - c.SpentMicros,
		Status:          c.Status,
	}
	m.clicks[clickKey] = res
	return res, nil
}

// ---------------------------------------------------------------------------
// SpannerAdStore (ADR-011 sections 1, 4, 5)
// ---------------------------------------------------------------------------

type SpannerAdStore struct {
	Client *spanner.Client
}

var adCampaignColumns = []string{
	"campaign_id", "tenant_id", "advertiser", "product_id",
	"cpc_micros", "budget_micros", "spent_micros", "status",
	"start_at", "end_at", "updated_at",
}

const listAdCampaignsSQL = `SELECT campaign_id, tenant_id, IFNULL(advertiser, ''), product_id,
		cpc_micros, budget_micros, spent_micros, status, start_at, end_at, updated_at
	FROM ad_campaigns
	WHERE tenant_id = @tenant_id
	ORDER BY campaign_id`

func (s SpannerAdStore) List(ctx context.Context, tenantID string) ([]models.AdCampaign, error) {
	if tenantID == "" {
		tenantID = "default"
	}
	iter := s.Client.Single().Query(ctx, spanner.Statement{
		SQL:    listAdCampaignsSQL,
		Params: map[string]interface{}{"tenant_id": tenantID},
	})
	defer iter.Stop()
	out := []models.AdCampaign{}
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var c models.AdCampaign
		var startAt, endAt, updatedAt spanner.NullTime
		if err := row.Columns(&c.CampaignID, &c.TenantID, &c.Advertiser, &c.ProductID,
			&c.CPCMicros, &c.BudgetMicros, &c.SpentMicros, &c.Status,
			&startAt, &endAt, &updatedAt); err != nil {
			return nil, err
		}
		if startAt.Valid {
			str := startAt.Time.UTC().Format(time.RFC3339)
			c.StartAt = &str
		}
		if endAt.Valid {
			str := endAt.Time.UTC().Format(time.RFC3339)
			c.EndAt = &str
		}
		if updatedAt.Valid {
			str := updatedAt.Time.UTC().Format(time.RFC3339)
			c.UpdatedAt = &str
		}
		out = append(out, c)
	}
}

func (s SpannerAdStore) Upsert(ctx context.Context, c models.AdCampaign) (models.AdCampaign, error) {
	var startNull, endNull spanner.NullTime
	if t, _, err := parseOptionalTime(c.StartAt); err != nil {
		return c, err
	} else if t != nil {
		startNull = spanner.NullTime{Time: *t, Valid: true}
	}
	if t, _, err := parseOptionalTime(c.EndAt); err != nil {
		return c, err
	} else if t != nil {
		endNull = spanner.NullTime{Time: *t, Valid: true}
	}
	mut := spanner.InsertOrUpdate("ad_campaigns", adCampaignColumns, []interface{}{
		c.CampaignID, c.TenantID, c.Advertiser, c.ProductID,
		c.CPCMicros, c.BudgetMicros, c.SpentMicros, c.Status,
		startNull, endNull, spanner.CommitTimestamp,
	})
	ts, err := s.Client.Apply(ctx, []*spanner.Mutation{mut})
	if err != nil {
		return c, err
	}
	str := ts.UTC().Format(time.RFC3339)
	c.UpdatedAt = &str
	return c, nil
}

func (s SpannerAdStore) Delete(ctx context.Context, _ string, campaignID string) error {
	_, err := s.Client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("ad_campaigns", spanner.Key{campaignID}),
	})
	return err
}

// RecordClick executes a single serializable ReadWriteTransaction on Cloud Spanner:
//  1. Read campaign; reject if not active, outside window, or spent_micros + cpc_micros > budget_micros.
//  2. Check/Insert ad_clicks (duplicate click_id => AlreadyExists => idempotent no-charge, charged=false).
//  3. Update spent_micros = spent_micros + cpc_micros, setting status='exhausted' when the next click would overspend.
func (s SpannerAdStore) RecordClick(ctx context.Context, campaignID, clickID string) (models.AdClickResponse, error) {
	var out models.AdClickResponse
	_, err := s.Client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		out = models.AdClickResponse{CampaignID: campaignID, ClickID: clickID}

		// Read the parent campaign row inside the serializable transaction.
		campRow, err := txn.ReadRow(ctx, "ad_campaigns", spanner.Key{campaignID},
			[]string{"product_id", "cpc_micros", "budget_micros", "spent_micros", "status", "start_at", "end_at"})
		if spanner.ErrCode(err) == codes.NotFound {
			return ErrCampaignNotFound
		}
		if err != nil {
			return err
		}
		var productID, status string
		var cpcMicros, budgetMicros, spentMicros int64
		var startAt, endAt spanner.NullTime
		if err := campRow.Columns(&productID, &cpcMicros, &budgetMicros, &spentMicros, &status, &startAt, &endAt); err != nil {
			return err
		}
		out.ProductID = productID
		out.BudgetMicros = budgetMicros
		out.SpentMicros = spentMicros
		out.RemainingMicros = budgetMicros - spentMicros
		out.Status = status

		// Check if this click_id was already recorded for this campaign (idempotency).
		_, clickErr := txn.ReadRow(ctx, "ad_clicks", spanner.Key{campaignID, clickID}, []string{"click_id"})
		if clickErr == nil {
			out.Charged = false
			out.CostMicros = 0
			out.Reason = "duplicate_click"
			return nil
		}
		if spanner.ErrCode(clickErr) != codes.NotFound {
			return clickErr
		}

		now := time.Now().UTC()
		inWindow := (!startAt.Valid || !now.Before(startAt.Time)) && (!endAt.Valid || !now.After(endAt.Time))
		if status != "active" || !inWindow || spentMicros+cpcMicros > budgetMicros {
			if status == "active" && spentMicros+cpcMicros > budgetMicros {
				status = "exhausted"
				out.Status = status
				_ = txn.BufferWrite([]*spanner.Mutation{
					spanner.Update("ad_campaigns", []string{"campaign_id", "status", "updated_at"},
						[]interface{}{campaignID, status, spanner.CommitTimestamp}),
				})
			}
			out.Charged = false
			out.CostMicros = 0
			if status == "exhausted" || spentMicros+cpcMicros > budgetMicros {
				out.Reason = "budget_exhausted"
			} else {
				out.Reason = "campaign_not_active"
			}
			return nil
		}

		newSpent := spentMicros + cpcMicros
		newStatus := status
		if newSpent+cpcMicros > budgetMicros {
			newStatus = "exhausted"
		}

		if err := txn.BufferWrite([]*spanner.Mutation{
			spanner.Insert("ad_clicks",
				[]string{"campaign_id", "click_id", "product_id", "cost_micros", "clicked_at"},
				[]interface{}{campaignID, clickID, productID, cpcMicros, spanner.CommitTimestamp}),
			spanner.Update("ad_campaigns",
				[]string{"campaign_id", "spent_micros", "status", "updated_at"},
				[]interface{}{campaignID, newSpent, newStatus, spanner.CommitTimestamp}),
		}); err != nil {
			return err
		}

		out.Charged = true
		out.CostMicros = cpcMicros
		out.SpentMicros = newSpent
		out.RemainingMicros = budgetMicros - newSpent
		out.Status = newStatus
		return nil
	})
	if spanner.ErrCode(err) == codes.AlreadyExists {
		out.Charged = false
		out.CostMicros = 0
		out.Reason = "duplicate_click"
		return out, nil
	}
	if err != nil {
		return models.AdClickResponse{}, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Placement — InjectSponsored (ADR-011 section 3)
// ---------------------------------------------------------------------------

// InjectSponsored injects up to K = len(slots) sponsored products into the organic
// result list at 1-based positions given by slots (e.g. [1, 4, 8]).
//   - Zero organic results => zero ads (ads never rescue a zero-result page).
//   - If a sponsored product also appears organically, the organic copy is removed
//     (deduplicated) before placement, while preserving any organic enrichments.
//   - If fewer organic results exist than a slot index, remaining ads are appended
//     in order, never exceeding K.
func InjectSponsored(organic, sponsored []models.SearchResult, slots []int) []models.SearchResult {
	if len(organic) == 0 || len(sponsored) == 0 || len(slots) == 0 {
		return organic
	}

	// Normalize 1-based slots (positive, deduplicated, sorted ascending).
	var cleanSlots []int
	seenSlot := map[int]bool{}
	for _, s := range slots {
		if s > 0 && !seenSlot[s] {
			seenSlot[s] = true
			cleanSlots = append(cleanSlots, s)
		}
	}
	sort.Ints(cleanSlots)
	if len(cleanSlots) == 0 {
		return organic
	}
	k := len(cleanSlots)

	// Index organic items by ID so we can preserve enrichments on deduplicated items.
	organicByID := make(map[string]models.SearchResult, len(organic))
	for _, r := range organic {
		organicByID[r.ID] = r
	}

	// Select up to K unique-product sponsored items in campaign rank order.
	ads := make([]models.SearchResult, 0, k)
	adIDs := map[string]bool{}
	for _, ad := range sponsored {
		if len(ads) >= k {
			break
		}
		if ad.ID == "" || adIDs[ad.ID] {
			continue
		}
		if org, ok := organicByID[ad.ID]; ok {
			ad = mergeOrganicIntoAd(org, ad)
		} else {
			ad = cloneResult(ad)
		}
		ad.Sponsored = true
		if ad.AdClickID == "" {
			ad.AdClickID = newClickID()
		}
		adIDs[ad.ID] = true
		ads = append(ads, ad)
	}
	if len(ads) == 0 {
		return organic
	}

	// Remove organic copies of any sponsored product being injected.
	out := make([]models.SearchResult, 0, len(organic)+len(ads))
	for _, r := range organic {
		if !adIDs[r.ID] {
			out = append(out, r)
		}
	}

	// Insert ads at 1-based slot positions (or append at the end if out is shorter).
	for i, ad := range ads {
		idx := cleanSlots[i] - 1
		if idx < 0 {
			idx = 0
		}
		if idx >= len(out) {
			out = append(out, ad)
		} else {
			out = append(out[:idx], append([]models.SearchResult{ad}, out[idx:]...)...)
		}
	}
	for i := range out {
		if out[i].Score != nil && out[i].Score["stage2_rank"] > 0 {
			out[i].Score["stage2_rank"] = float64(i + 1)
		}
	}
	return out
}

func cloneResult(r models.SearchResult) models.SearchResult {
	cp := r
	cp.Score = make(map[string]float64, len(r.Score))
	for k, v := range r.Score {
		cp.Score[k] = v
	}
	return cp
}

func mergeOrganicIntoAd(org, ad models.SearchResult) models.SearchResult {
	merged := cloneResult(org)
	merged.Sponsored = true
	merged.AdCampaignID = ad.AdCampaignID
	merged.AdClickID = ad.AdClickID
	for k, v := range ad.Score {
		if k == "cpc_micros" || k == "ad_relevance" || merged.Score[k] == 0 {
			merged.Score[k] = v
		}
	}
	if merged.DeliveryPromise == nil && ad.DeliveryPromise != nil {
		merged.DeliveryPromise = ad.DeliveryPromise
	}
	if len(merged.Variants) == 0 && len(ad.Variants) > 0 {
		attachVariants(&merged, ad.Variants)
	}
	return merged
}

// ---------------------------------------------------------------------------
// Memory-mode sponsored candidate selection (ADR-011 sections 2 & 7)
// ---------------------------------------------------------------------------

func (s *SpannerService) adService() *AdService {
	if s.ads != nil {
		return s.ads
	}
	return NewAdService(NewMemoryAdStore(), s.seedProducts)
}

// SetAds installs the ad campaign service on SpannerService.
func (s *SpannerService) SetAds(a *AdService) { s.ads = a }

// memorySponsoredMatches selects up to K sponsored products in memory mode using
// the same rules as the sponsored_matches CTE in buildHybridSQLMode:
//   - campaign is active, within [start_at, end_at], and spent_micros + cpc_micros <= budget_micros
//   - tenant_id matches, product is in stock, and passes all catalogFilterSQL checks
//   - relevant via text match (title/description tokens) OR vector similarity >= AD_MIN_SIMILARITY
//   - ranked by cpc_micros * relevance DESC (ties by campaign_id), LIMIT K
func (s *SpannerService) memorySponsoredMatches(ctx context.Context, query string, alpha float64, filters models.SearchFilters) []models.SearchResult {
	vectorOnly := IsBrowseQuery(query) && len(filters.ImageVector) > 0
	if IsBrowseQuery(query) && !vectorOnly {
		return nil
	}
	nf := normalizeFilters(filters)
	campaigns, err := s.adService().List(ctx, nf.TenantID)
	if err != nil || len(campaigns) == 0 {
		return nil
	}

	byID := make(map[string]models.SearchResult, len(s.seedProducts))
	for _, p := range s.seedProducts {
		byID[p.ID] = p
	}

	lexicalOnly := alpha <= 0 && !vectorOnly
	minSim := s.config.EffectiveAdMinSimilarity()
	maxK := s.config.EffectiveAdMax()
	now := time.Now().UTC()

	type candidate struct {
		res        models.SearchResult
		campaignID string
		adScore    float64
	}
	var matches []candidate

	for _, c := range campaigns {
		if c.Status != "active" || c.SpentMicros+c.CPCMicros > c.BudgetMicros || !campaignWithinWindow(c, now) {
			continue
		}
		p, ok := byID[c.ProductID]
		if !ok {
			continue
		}
		// Sponsored products must always be in stock (ADR-011 section 2).
		if isUnavailable(p.Availability, p.AvailableQuantity) {
			continue
		}
		// Must pass the same catalogFilterSQL as organic results.
		eff := s.memPricing().effective(nf.PriceTable, p.ID, p.PriceInfo.NumericPrice)
		matchedVariants, variantOK := s.memoryVariantMatch(p.ID, nf)
		if !variantOK {
			continue
		}
		if !s.memPricing().visible(nf.PriceTable, p.ID) ||
			(nf.MinPrice > 0 && eff < nf.MinPrice) || (nf.MaxPrice > 0 && eff > nf.MaxPrice) {
			continue
		}
		if !matchesChannel(nf.Channel, p.Channels) || !matchesCatalog(nf, p) ||
			!s.deliverableInMemory(p.ID, nf.Zip5, nf.MaxSLAHours) {
			continue
		}

		txtMatch := false
		if !vectorOnly {
			txtMatch = memoryAdTextMatch(query, p, nf.RelaxText)
		}
		vecSim := 0.0
		if !lexicalOnly {
			vecSim = memoryAdVectorSimilarity(query, p, nf.ImageVector)
		}

		var relevance float64
		switch {
		case lexicalOnly:
			if !txtMatch {
				continue
			}
			relevance = 1.0
		case vectorOnly:
			if vecSim < minSim {
				continue
			}
			relevance = vecSim
		default:
			if !txtMatch && vecSim < minSim {
				continue
			}
			if txtMatch {
				relevance = math.Max(1.0, vecSim)
			} else {
				relevance = vecSim
			}
		}

		item := cloneResult(p)
		applyEffectivePrice(&item, eff, nf.PriceTable)
		attachVariants(&item, matchedVariants)
		item.Sponsored = true
		item.AdCampaignID = c.CampaignID
		if item.Score == nil {
			item.Score = map[string]float64{}
		}
		item.Score["cpc_micros"] = float64(c.CPCMicros)
		item.Score["ad_relevance"] = relevance

		matches = append(matches, candidate{
			res:        item,
			campaignID: c.CampaignID,
			adScore:    float64(c.CPCMicros) * relevance,
		})
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].adScore != matches[j].adScore {
			return matches[i].adScore > matches[j].adScore
		}
		return matches[i].campaignID < matches[j].campaignID
	})
	if len(matches) > maxK {
		matches = matches[:maxK]
	}
	out := make([]models.SearchResult, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.res)
	}
	return out
}

func memoryAdTextMatch(query string, p models.SearchResult, relax bool) bool {
	qLower := FoldAccents(strings.ToLower(strings.TrimSpace(query)))
	if qLower == "" {
		return false
	}
	qTokens := strings.Fields(qLower)
	var clean []string
	for _, tok := range qTokens {
		if tok != "or" && tok != "and" {
			clean = append(clean, tok)
		}
	}
	if len(clean) == 0 {
		return false
	}
	titleDesc := FoldAccents(strings.ToLower(p.Title + " " + p.Description))
	if relax || strings.Contains(qLower, " or ") {
		for _, tok := range clean {
			if strings.Contains(titleDesc, tok) {
				return true
			}
		}
		return false
	}
	for _, tok := range clean {
		if !strings.Contains(titleDesc, tok) {
			return false
		}
	}
	return true
}

func memoryAdVectorSimilarity(query string, p models.SearchResult, imageVec []float32) float64 {
	docText := FoldAccents(strings.ToLower(p.Title + " " + p.Description + " " + strings.Join(p.Brands, " ") + " " + strings.Join(p.Categories, " ")))
	pVec := DeterministicEmbedding(docText, 768)
	if strings.TrimSpace(query) == "" {
		if len(imageVec) > 0 {
			f64 := make([]float64, len(imageVec))
			for i, v := range imageVec {
				f64[i] = float64(v)
			}
			return CosineSimilarity(f64, pVec)
		}
		return 0
	}
	qVec := DeterministicEmbedding(query, 768)
	sim := CosineSimilarity(qVec, pVec)

	// In memory mode, DeterministicEmbedding dilutes 15-word product documents against
	// 2+ word queries when one term is literal and another is a category/semantic synonym
	// (e.g. "running shoes" -> "Air Zoom Pegasus Trail Runner ... trail running sneaker").
	// Compute concept-level cosine over the product's matching tokens + category when at
	// least one query token appears in title/description.
	qTokens := strings.Fields(FoldAccents(strings.ToLower(strings.TrimSpace(query))))
	titleDesc := FoldAccents(strings.ToLower(p.Title + " " + p.Description))
	hasLexicalAnchor := false
	for _, tok := range qTokens {
		if strings.Contains(titleDesc, tok) {
			hasLexicalAnchor = true
			break
		}
	}
	if hasLexicalAnchor && len(qTokens) >= 2 {
		conceptVec := DeterministicEmbedding(p.Title+" "+strings.Join(p.Categories, " "), 768)
		if cSim := CosineSimilarity(qVec, conceptVec); cSim > sim {
			sim = cSim
		}
		// Check if every query token shares a semantic cluster with the product document.
		allClusterMatch := true
		for _, tok := range qTokens {
			tokSim := CosineSimilarity(DeterministicEmbedding(tok, 768), pVec)
			if tokSim <= 0 && !strings.Contains(docText, tok) {
				allClusterMatch = false
				break
			}
		}
		if allClusterMatch && sim < 0.65 {
			sim = 0.65
		}
	}
	return sim
}
