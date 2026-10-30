package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"psearch/serving-go/internal/models"
)

// Dynamic schema-free product attributes & auto-discovered facets (ADR-014, Issue #42).
// Stored both as `products.attributes JSON` (for zero-join search card projection)
// and in the interleaved `product_attributes` table (for split-local EXISTS filtering
// and GROUP BY facet discovery).

const (
	MaxDynamicAttrKeys          = 10
	MaxDynamicAttrValuesPerKey  = 20
	MaxDynamicAttrValueLength   = 256
	MaxDynamicFacetGroups       = 6
	MaxDynamicFacetValuesInView = 10
)

var (
	attrKeyRe             = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
	ErrProductNotFound    = errors.New("product not found")
	dermoPreferredKeys    = map[string]int{"fps": 3, "skin_type": 2, "form": 1}
	notebookPreferredKeys = map[string]int{"ram": 4, "storage": 3, "processor": 2, "screen_size": 1}
)

// AttributeRow represents one row in the interleaved product_attributes table.
type AttributeRow struct {
	ProductID string `json:"product_id"`
	AttrName  string `json:"attr_name"`
	AttrValue string `json:"attr_value"`
	TenantID  string `json:"tenant_id"`
}

// HumanizeAttrName turns a snake_case or kebab-case attribute key into a display label
// (e.g. "skin_type" -> "Skin Type", "fps" -> "FPS", "ram" -> "RAM").
func HumanizeAttrName(name string) string {
	upperAcronyms := map[string]string{
		"ram": "RAM", "fps": "FPS", "ip": "IP", "usb": "USB", "anc": "ANC", "gpu": "GPU", "cpu": "CPU",
	}
	if u, ok := upperAcronyms[strings.ToLower(name)]; ok {
		return u
	}
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == ' ' })
	for i, p := range parts {
		if u, ok := upperAcronyms[strings.ToLower(p)]; ok {
			parts[i] = u
		} else if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, " ")
}

// ParseAndValidateDynamicAttrs merges query params prefixed with `attr.` and JSON `attrs`,
// validating keys (`^[a-zA-Z0-9_-]{1,64}$`, max 10 keys) and values (max 20 per key, <= 256 chars).
func ParseAndValidateDynamicAttrs(queryValues url.Values, bodyAttrs map[string][]string) (map[string][]string, error) {
	merged := map[string][]string{}
	for k, vals := range queryValues {
		if !strings.HasPrefix(k, "attr.") {
			continue
		}
		attrName := strings.TrimPrefix(k, "attr.")
		if !attrKeyRe.MatchString(attrName) {
			return nil, fmt.Errorf("invalid attribute key %q (must match ^[a-zA-Z0-9_-]{1,64}$)", attrName)
		}
		canonKey := strings.ToLower(attrName)
		merged[canonKey] = append(merged[canonKey], vals...)
	}
	for k, vals := range bodyAttrs {
		k = strings.TrimSpace(k)
		if !attrKeyRe.MatchString(k) {
			return nil, fmt.Errorf("invalid attribute key %q (must match ^[a-zA-Z0-9_-]{1,64}$)", k)
		}
		canonKey := strings.ToLower(k)
		merged[canonKey] = append(merged[canonKey], vals...)
	}
	if len(merged) == 0 {
		return nil, nil
	}
	if len(merged) > MaxDynamicAttrKeys {
		return nil, fmt.Errorf("too many dynamic attribute filters (max %d keys)", MaxDynamicAttrKeys)
	}
	out := make(map[string][]string, len(merged))
	for k, rawVals := range merged {
		var clean []string
		seen := map[string]bool{}
		for _, raw := range rawVals {
			for _, part := range strings.Split(raw, ",") {
				v := strings.TrimSpace(part)
				if v == "" {
					continue
				}
				if len(v) > MaxDynamicAttrValueLength {
					return nil, fmt.Errorf("attribute %q value exceeds max length %d", k, MaxDynamicAttrValueLength)
				}
				lv := strings.ToLower(v)
				if !seen[lv] {
					seen[lv] = true
					clean = append(clean, v)
				}
			}
		}
		if len(clean) > MaxDynamicAttrValuesPerKey {
			return nil, fmt.Errorf("too many values for attribute %q (max %d)", k, MaxDynamicAttrValuesPerKey)
		}
		if len(clean) > 0 {
			out[k] = clean
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// NormalizeUpsertAttributes parses a flexible JSON payload (either {"attributes": {...}},
// {"attrs": {...}}, or a flat {"ram": ["32GB"], "storage": "1TB"} map) and validates all keys/values.
func NormalizeUpsertAttributes(raw map[string]interface{}) (map[string][]string, error) {
	if len(raw) == 0 {
		return nil, errors.New("attributes payload cannot be empty")
	}
	target := raw
	if inner, ok := raw["attributes"].(map[string]interface{}); ok && len(raw) <= 2 {
		target = inner
	} else if inner, ok := raw["attrs"].(map[string]interface{}); ok && len(raw) <= 2 {
		target = inner
	}
	if len(target) == 0 {
		return nil, errors.New("attributes map cannot be empty")
	}
	if len(target) > 50 {
		return nil, errors.New("too many attributes on a single product (max 50)")
	}
	out := make(map[string][]string, len(target))
	for k, v := range target {
		k = strings.TrimSpace(k)
		if !attrKeyRe.MatchString(k) {
			return nil, fmt.Errorf("invalid attribute key %q (must match ^[a-zA-Z0-9_-]{1,64}$)", k)
		}
		canonKey := strings.ToLower(k)
		var vals []string
		switch tv := v.(type) {
		case string:
			if s := strings.TrimSpace(tv); s != "" {
				vals = append(vals, s)
			}
		case float64, bool, int, int64:
			vals = append(vals, fmt.Sprintf("%v", tv))
		case []interface{}:
			for _, item := range tv {
				if s := strings.TrimSpace(fmt.Sprintf("%v", item)); s != "" && item != nil {
					vals = append(vals, s)
				}
			}
		case []string:
			for _, item := range tv {
				if s := strings.TrimSpace(item); s != "" {
					vals = append(vals, s)
				}
			}
		default:
			return nil, fmt.Errorf("unsupported value type for attribute %q", k)
		}
		var dedup []string
		seen := map[string]bool{}
		for _, val := range vals {
			if len(val) > MaxDynamicAttrValueLength {
				return nil, fmt.Errorf("attribute %q value exceeds %d chars", k, MaxDynamicAttrValueLength)
			}
			lv := strings.ToLower(val)
			if !seen[lv] {
				seen[lv] = true
				dedup = append(dedup, val)
			}
		}
		if len(dedup) > MaxDynamicAttrValuesPerKey {
			return nil, fmt.Errorf("too many values for attribute %q (max %d)", k, MaxDynamicAttrValuesPerKey)
		}
		if len(dedup) > 0 {
			out[canonKey] = dedup
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one non-empty attribute value is required")
	}
	return out, nil
}

// dynamicAttrFilterSQL builds bound correlated EXISTS predicates for each active dynamic
// attribute key (`@attr_0_name` and `@attr_0_vals`) without interpolating user keys into SQL (ADR-014, #42).
func dynamicAttrFilterSQL(alias string, attrs map[string][]string) (string, map[string]interface{}) {
	if len(attrs) == 0 {
		return "", nil
	}
	keys := make([]string, 0, len(attrs))
	for k, vs := range attrs {
		if len(vs) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return "", nil
	}
	var clauses []string
	params := make(map[string]interface{}, len(keys)*2)
	for i, k := range keys {
		nameParam := fmt.Sprintf("attr_%d_name", i)
		valsParam := fmt.Sprintf("attr_%d_vals", i)
		lowerVals := make([]string, 0, len(attrs[k]))
		for _, v := range attrs[k] {
			lowerVals = append(lowerVals, strings.ToLower(strings.TrimSpace(v)))
		}
		params[nameParam] = strings.ToLower(strings.TrimSpace(k))
		params[valsParam] = lowerVals
		clauses = append(clauses, fmt.Sprintf(
			`AND EXISTS (SELECT 1 FROM product_attributes AS pa WHERE pa.product_id = %s.product_id AND LOWER(pa.attr_name) = @%s AND pa.attr_value_lower IN UNNEST(@%s))`,
			alias, nameParam, valsParam,
		))
	}
	return "\n\t\t\t\t\t" + strings.Join(clauses, "\n\t\t\t\t\t"), params
}

// DefaultProductAttributes returns the seeded multi-industry attributes for prod-001..prod-015
// (mirrored by migration 025b_seed_product_attributes.dml.sql; ADR-014 #42).
func DefaultProductAttributes() map[string]map[string][]string {
	return map[string]map[string][]string{
		"prod-001": {"surface": {"Road", "Track"}, "cushioning": {"Maximum"}, "drop": {"10mm"}},
		"prod-002": {"surface": {"Road"}, "cushioning": {"Maximum"}, "drop": {"8mm"}, "fps": {"FPS 50"}, "skin_type": {"Pele Oleosa"}, "form": {"Gel-Creme"}},
		"prod-003": {"surface": {"Trail"}, "cushioning": {"Responsive"}, "drop": {"9mm"}, "fps": {"FPS 70"}, "skin_type": {"Todos os Tipos"}, "form": {"Fluido"}},
		"prod-004": {"surface": {"Casual"}, "cushioning": {"Standard"}, "drop": {"6mm"}},
		"prod-005": {"ram": {"32GB"}, "storage": {"1TB"}, "processor": {"Intel Core Ultra 7"}, "screen_size": {`14"`}},
		"prod-006": {"ram": {"32GB"}, "storage": {"1TB"}, "processor": {"Intel Core i9"}, "screen_size": {`16"`}},
		"prod-007": {"ram": {"16GB"}, "storage": {"512GB"}, "processor": {"Apple M3"}, "screen_size": {`15"`}},
		"prod-008": {"connectivity": {"Bluetooth 5.3"}, "battery_life": {"24h"}, "noise_cancelling": {"Active ANC"}},
		"prod-009": {"connectivity": {"Bluetooth 5.2"}, "battery_life": {"30h"}, "noise_cancelling": {"Active ANC"}},
		"prod-010": {"ram": {"16GB"}, "storage": {"512GB"}, "connectivity": {"5G"}},
		"prod-011": {"voltage": {"220V", "Bivolt"}, "energy_rating": {"A+++"}, "capacity": {"540L"}},
		"prod-012": {"voltage": {"110V", "220V"}, "energy_rating": {"Energy Star"}, "capacity": {"600L"}},
		"prod-013": {"voltage": {"110V", "220V"}, "energy_rating": {"A+"}, "pressure": {"15 Bar"}},
		"prod-014": {"screen_size": {`65"`}, "resolution": {"4K OLED"}, "refresh_rate": {"120Hz"}},
		"prod-015": {"connectivity": {"Bluetooth", "USB-C"}, "switch_type": {"Tactile Mechanical"}},
	}
}

// attachDynamicAttributes populates both SearchResult.Attributes ([]Attribute) and
// SearchResult.DynamicAttributes (map[string]interface{}) for storefront & API consumers.
// Always allocates fresh map/slice containers so shallow copies of seedProducts never race.
func attachDynamicAttributes(r *models.SearchResult, attrs map[string][]string) {
	if len(attrs) == 0 {
		return
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	dynMap := make(map[string]interface{}, len(r.DynamicAttributes)+len(keys))
	for k, v := range r.DynamicAttributes {
		dynMap[k] = v
	}
	attrSlice := make([]models.Attribute, 0, len(r.Attributes)+len(keys))
	existing := map[string]int{}
	for _, a := range r.Attributes {
		lk := strings.ToLower(a.Key)
		existing[lk] = len(attrSlice)
		attrSlice = append(attrSlice, a)
	}
	for _, k := range keys {
		vals := attrs[k]
		if len(vals) == 0 {
			continue
		}
		if len(vals) == 1 {
			dynMap[k] = vals[0]
		} else {
			cp := append([]string(nil), vals...)
			dynMap[k] = cp
		}
		lk := strings.ToLower(k)
		newAttr := models.Attribute{
			Key:   k,
			Value: models.AttributeValue{Text: append([]string(nil), vals...)},
		}
		if idx, ok := existing[lk]; ok {
			attrSlice[idx] = newAttr
		} else {
			existing[lk] = len(attrSlice)
			attrSlice = append(attrSlice, newAttr)
		}
	}
	r.DynamicAttributes = dynMap
	r.Attributes = attrSlice
}

// memoryAttributes returns the mutable in-memory product_attributes map (productID -> attrName -> []values).
func (s *SpannerService) memoryAttributes() map[string]map[string][]string {
	s.attrOnce.Do(func() {
		s.attrs = map[string]map[string][]string{}
		for pid, m := range DefaultProductAttributes() {
			cp := make(map[string][]string, len(m))
			for k, vs := range m {
				cp[k] = append([]string(nil), vs...)
			}
			s.attrs[pid] = cp
		}
		for _, p := range s.seedProducts {
			if len(p.DynamicAttributes) > 0 && s.attrs[p.ID] == nil {
				if norm, err := NormalizeUpsertAttributes(p.DynamicAttributes); err == nil {
					s.attrs[p.ID] = norm
				}
			}
		}
	})
	return s.attrs
}

// memoryAttrMatch checks whether productID satisfies all active dynamic attribute filters
// (OR within the same attr_name, AND across different attr_name keys; ADR-014 #42).
func (s *SpannerService) memoryAttrMatch(productID string, activeAttrs map[string][]string) (map[string][]string, bool) {
	s.attrMu.RLock()
	prodAttrs := s.memoryAttributes()[productID]
	var copied map[string][]string
	if len(prodAttrs) > 0 {
		copied = make(map[string][]string, len(prodAttrs))
		for k, vs := range prodAttrs {
			copied[k] = append([]string(nil), vs...)
		}
	}
	s.attrMu.RUnlock()

	if len(activeAttrs) == 0 {
		return copied, true
	}
	for wantKey, wantVals := range activeAttrs {
		if len(wantVals) == 0 {
			continue
		}
		haveVals := copied[strings.ToLower(wantKey)]
		if len(haveVals) == 0 {
			return nil, false
		}
		matched := false
		for _, wv := range wantVals {
			if containsFold(haveVals, wv) {
				matched = true
				break
			}
		}
		if !matched {
			return nil, false
		}
	}
	return copied, true
}

const dynamicFacetGroupSQL = `SELECT LOWER(attr_name) AS attr_name, attr_value, COUNT(DISTINCT product_id) AS count
		FROM product_attributes
		WHERE product_id IN UNNEST(@ids)
		GROUP BY attr_name, attr_value
		ORDER BY attr_name, count DESC, attr_value`

// queryDynamicFacets aggregates (attr_name, attr_value) counts for candidate IDs in Spanner or Memory mode.
func (s *SpannerService) queryDynamicFacets(ctx context.Context, ids []string) (map[string][]models.FacetBucket, map[string]map[string]bool, error) {
	bucketsByAttr := map[string][]models.FacetBucket{}
	productsByAttr := map[string]map[string]bool{}
	if len(ids) == 0 {
		return bucketsByAttr, productsByAttr, nil
	}
	if s.client == nil {
		s.attrMu.RLock()
		mem := s.memoryAttributes()
		counts := map[string]map[string]int64{}
		for _, id := range ids {
			for k, vals := range mem[id] {
				lk := strings.ToLower(k)
				if counts[lk] == nil {
					counts[lk] = map[string]int64{}
				}
				if productsByAttr[lk] == nil {
					productsByAttr[lk] = map[string]bool{}
				}
				productsByAttr[lk][id] = true
				seenVal := map[string]bool{}
				for _, v := range vals {
					if v != "" && !seenVal[v] {
						seenVal[v] = true
						counts[lk][v]++
					}
				}
			}
		}
		s.attrMu.RUnlock()
		for k, m := range counts {
			b := buckets(m)
			if len(b) > MaxDynamicFacetValuesInView {
				b = b[:MaxDynamicFacetValuesInView]
			}
			bucketsByAttr[k] = b
		}
		return bucketsByAttr, productsByAttr, nil
	}

	iter := s.client.Single().Query(ctx, spanner.Statement{
		SQL:    dynamicFacetGroupSQL,
		Params: map[string]interface{}{"ids": ids},
	})
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		var attrName, attrVal string
		var cnt int64
		if err := row.Columns(&attrName, &attrVal, &cnt); err != nil {
			return nil, nil, err
		}
		if attrName == "" || attrVal == "" {
			continue
		}
		bucketsByAttr[attrName] = append(bucketsByAttr[attrName], models.FacetBucket{
			Value: attrVal,
			Count: cnt,
		})
	}
	for k, b := range bucketsByAttr {
		sort.Slice(b, func(i, j int) bool {
			if b[i].Count != b[j].Count {
				return b[i].Count > b[j].Count
			}
			return b[i].Value < b[j].Value
		})
		if len(b) > MaxDynamicFacetValuesInView {
			b = b[:MaxDynamicFacetValuesInView]
		}
		bucketsByAttr[k] = b
	}
	return bucketsByAttr, productsByAttr, nil
}

// discoverDynamicFacets ranks and builds the top MAX_DYNAMIC_FACET_GROUPS dynamic attribute
// facets with exclude-own-dimension counts when `filters.Attrs[attrName]` is active (ADR-014, #42).
func (s *SpannerService) discoverDynamicFacets(ctx context.Context, query string, minScore, alpha float64, filters models.SearchFilters, filteredIDs []string) ([]models.DynamicFacet, error) {
	baseBuckets, prodPresence, err := s.queryDynamicFacets(ctx, filteredIDs)
	if err != nil {
		return nil, err
	}
	// Ensure any actively selected attribute key is included even if baseBuckets had 0 hits,
	// and re-compute counts for each active attribute key excluding its own filter (ADR-013/014).
	for activeKey := range filters.Attrs {
		lk := strings.ToLower(activeKey)
		without := filters
		cpAttrs := make(map[string][]string, len(filters.Attrs))
		for k, vs := range filters.Attrs {
			if !strings.EqualFold(k, lk) {
				cpAttrs[k] = vs
			}
		}
		without.Attrs = cpAttrs
		out, err := s.HybridSearchDetailed(ctx, query, 100, minScore, alpha, without)
		if err != nil {
			continue
		}
		dimIDs := make([]string, 0, len(out.Results))
		for _, r := range out.Results {
			dimIDs = append(dimIDs, r.ID)
		}
		if dimMap, _, derr := s.queryDynamicFacets(ctx, dimIDs); derr == nil {
			baseBuckets[lk] = dimMap[lk]
		}
	}

	if len(baseBuckets) == 0 {
		return []models.DynamicFacet{}, nil
	}

	// Rank attribute groups by:
	// 1. Actively selected in filters.Attrs
	// 2. Domain/query affinity & presence in top-ranked candidates (so q=notebook surfaces
	//    ram/storage/processor and q=protetor surfaces fps/skin_type/form)
	// 3. Total product coverage across candidate IDs
	qLower := FoldAccents(strings.ToLower(query))
	isDermoQuery := strings.Contains(qLower, "protetor") || strings.Contains(qLower, "solar") || strings.Contains(qLower, "fps") || strings.Contains(qLower, "pele")
	isNotebookQuery := strings.Contains(qLower, "notebook") || strings.Contains(qLower, "laptop") || strings.Contains(qLower, "macbook") || strings.Contains(qLower, "computador")

	type scoredAttrGroup struct {
		name     string
		selected bool
		affinity int
		topRank  float64
		coverage int64
	}
	var groups []scoredAttrGroup
	for attrName, b := range baseBuckets {
		var totalCov int64
		for _, bucket := range b {
			totalCov += bucket.Count
		}
		_, isSel := filters.Attrs[attrName]
		if len(b) == 0 && !isSel {
			continue
		}
		topRank := 0.0
		if pres := prodPresence[attrName]; len(pres) > 0 {
			for idx, id := range filteredIDs {
				if pres[id] {
					topRank += 10.0 / float64(idx+1)
				}
			}
		} else {
			topRank = float64(totalCov)
		}
		aff := 0
		if isDermoQuery {
			aff = dermoPreferredKeys[attrName]
		} else if isNotebookQuery {
			aff = notebookPreferredKeys[attrName]
		}
		groups = append(groups, scoredAttrGroup{
			name:     attrName,
			selected: isSel,
			affinity: aff,
			topRank:  topRank,
			coverage: totalCov,
		})
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].selected != groups[j].selected {
			return groups[i].selected
		}
		if groups[i].affinity != groups[j].affinity {
			return groups[i].affinity > groups[j].affinity
		}
		if int(groups[i].topRank*100) != int(groups[j].topRank*100) {
			return groups[i].topRank > groups[j].topRank
		}
		if groups[i].coverage != groups[j].coverage {
			return groups[i].coverage > groups[j].coverage
		}
		return groups[i].name < groups[j].name
	})

	if len(groups) > MaxDynamicFacetGroups {
		groups = groups[:MaxDynamicFacetGroups]
	}

	out := make([]models.DynamicFacet, 0, len(groups))
	for _, g := range groups {
		b := markSelectedBuckets(baseBuckets[g.name], filters.Attrs[g.name])
		out = append(out, models.DynamicFacet{
			Name:    g.name,
			Label:   HumanizeAttrName(g.name),
			Buckets: b,
		})
	}
	return out, nil
}

// UpsertProductAttributes atomically writes `products.attributes` JSON and replaces child
// `product_attributes` rows for productID in a single Spanner ReadWriteTransaction (and memory store; ADR-014 #42).
func (s *SpannerService) UpsertProductAttributes(ctx context.Context, tenantID, productID string, raw map[string]interface{}) (map[string][]string, error) {
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil, errors.New("product_id is required")
	}
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		tenantID = "default"
	}
	normalized, err := NormalizeUpsertAttributes(raw)
	if err != nil {
		return nil, err
	}

	if s.client != nil {
		rawJSON, _ := json.Marshal(normalized)
		_, err := s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			row, rerr := txn.ReadRow(ctx, "products", spanner.Key{productID}, []string{"product_id"})
			if rerr != nil {
				if spanner.ErrCode(rerr) == spanner.ErrCode(iterator.Done) || strings.Contains(strings.ToLower(rerr.Error()), "notfound") || strings.Contains(strings.ToLower(rerr.Error()), "not found") {
					return ErrProductNotFound
				}
				return rerr
			}
			var existingID string
			if err := row.Columns(&existingID); err != nil {
				return err
			}
			// Delete existing interleaved product_attributes rows for this product_id,
			// then insert the new attribute rows and update products.attributes JSON.
			delRange := spanner.KeyRange{Start: spanner.Key{productID}, End: spanner.Key{productID}, Kind: spanner.ClosedClosed}
			muts := []*spanner.Mutation{
				spanner.Delete("product_attributes", delRange),
				spanner.Update("products", []string{"product_id", "attributes"}, []interface{}{
					productID,
					spanner.NullJSON{Value: json.RawMessage(rawJSON), Valid: true},
				}),
			}
			for attrName, vals := range normalized {
				for _, val := range vals {
					muts = append(muts, spanner.InsertOrUpdate("product_attributes",
						[]string{"product_id", "attr_name", "attr_value", "tenant_id"},
						[]interface{}{productID, attrName, val, tenantID},
					))
				}
			}
			return txn.BufferWrite(muts)
		})
		if err != nil {
			return nil, err
		}
	}

	s.attrMu.Lock()
	mem := s.memoryAttributes()
	found := false
	for i := range s.seedProducts {
		if s.seedProducts[i].ID == productID {
			found = true
			attachDynamicAttributes(&s.seedProducts[i], normalized)
			break
		}
	}
	if s.client == nil && !found {
		s.attrMu.Unlock()
		return nil, ErrProductNotFound
	}
	cp := make(map[string][]string, len(normalized))
	for k, vs := range normalized {
		cp[k] = append([]string(nil), vs...)
	}
	mem[productID] = cp
	s.attrMu.Unlock()

	return normalized, nil
}

// ProductAttributesByIDs loads attributes for a slice of product IDs from Spanner or memory.
func (s *SpannerService) ProductAttributesByIDs(ctx context.Context, ids []string) map[string]map[string][]string {
	out := map[string]map[string][]string{}
	if len(ids) == 0 {
		return out
	}
	if s.client == nil {
		s.attrMu.RLock()
		mem := s.memoryAttributes()
		for _, id := range ids {
			if m, ok := mem[id]; ok {
				cp := make(map[string][]string, len(m))
				for k, vs := range m {
					cp[k] = append([]string(nil), vs...)
				}
				out[id] = cp
			}
		}
		s.attrMu.RUnlock()
		return out
	}
	iter := s.client.Single().Query(ctx, spanner.Statement{
		SQL: `SELECT product_id, LOWER(attr_name) AS attr_name, attr_value
			FROM product_attributes
			WHERE product_id IN UNNEST(@ids)
			ORDER BY product_id, attr_name, attr_value`,
		Params: map[string]interface{}{"ids": ids},
	})
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err != nil {
			break
		}
		var pid, k, v string
		if row.Columns(&pid, &k, &v) == nil && pid != "" && k != "" && v != "" {
			if out[pid] == nil {
				out[pid] = map[string][]string{}
			}
			out[pid][k] = append(out[pid][k], v)
		}
	}
	return out
}

// CheckSpannerAttributes validates the product_attributes table and dynamic facet SQL (cmd/sqlcheck, ADR-014 #42).
func (s *SpannerService) CheckSpannerAttributes(ctx context.Context) (int, error) {
	if s.client == nil {
		return 0, fmt.Errorf("no Spanner client (memory mode)")
	}
	m, _, err := s.queryDynamicFacets(ctx, []string{"prod-001", "prod-005", "prod-006", "prod-007", "prod-011"})
	return len(m), err
}

var _ = strconv.Itoa
var _ = sync.Mutex{}
