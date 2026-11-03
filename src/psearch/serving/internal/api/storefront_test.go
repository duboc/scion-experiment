package api

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Issue #30 regression: the embedded storefront's inline JS must parse. A missing
// closing brace (shipped in rev 00024/00025) left the page stuck on "Loading catalog...".
func TestStorefrontInlineJSParses(t *testing.T) {
	scripts := regexp.MustCompile(`(?s)<script[^>]*>(.*?)</script>`).FindAllStringSubmatch(defaultStorefrontHTML, -1)
	if len(scripts) == 0 {
		t.Fatal("storefront has no inline <script>")
	}
	var js strings.Builder
	for _, m := range scripts {
		js.WriteString(m[1])
		js.WriteString("\n")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping JS syntax check")
	}
	path := filepath.Join(t.TempDir(), "storefront.js")
	if err := os.WriteFile(path, []byte(js.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(node, "--check", path).CombinedOutput(); err != nil {
		t.Fatalf("storefront inline JS has a syntax error:\n%s", out)
	}
}

func TestStorefrontShowAllCatalog(t *testing.T) {
	for _, frag := range []string{`id="show-all-btn"`, "Show All Catalog", "function showAllCatalog()", "&limit=100"} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
	if strings.Contains(defaultStorefrontHTML, `value="running shoes"`) {
		t.Error("initial query must be empty so the catalog loads on page load")
	}
}

// #30: empty / "*" queries browse the full catalog (filters still apply).
func TestCatalogBrowse(t *testing.T) {
	r := newTestRouter()
	for _, q := range []string{"q=&limit=100", "q=*&limit=100"} {
		resp := doSearch(t, r, q)
		if len(resp.Results) != 15 || resp.DidYouMean != "" || resp.QueryRelaxed {
			t.Fatalf("%s: results=%d did_you_mean=%q relaxed=%v", q, len(resp.Results), resp.DidYouMean, resp.QueryRelaxed)
		}
		if resp.Facets == nil || len(resp.Facets.Categories) == 0 {
			t.Fatalf("%s: catalog browse must return facets", q)
		}
		last := resp.Results[len(resp.Results)-1]
		if last.Availability != "OUT_OF_STOCK" {
			t.Fatalf("%s: out-of-stock products must be listed last", q)
		}
	}
	if pt := doSearch(t, r, "q=*&limit=100&locale=pt-BR"); len(pt.Results) != 5 {
		t.Fatalf("browse must honor filters: pt-BR=%d, want 5", len(pt.Results))
	}
}

// #33 (ADR-010): photo search UI posts the resized image inline to /api/search.
func TestStorefrontPhotoSearch(t *testing.T) {
	for _, frag := range []string{
		`id="image-file"`, `accept="image/jpeg,image/png,image/webp"`, `id="image-btn"`, `id="image-chip"`,
		"function onImagePicked(", "function clearQueryImage()", "function imageSearchBody(",
		"canvas.toDataURL('image/jpeg'", "image_base64: queryImage", "method: 'POST'", "data.image_query",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
	if strings.Contains(defaultStorefrontHTML, "image_url") {
		t.Error("storefront must send inline bytes, never an image URL (SSRF)")
	}
}

// #34: facet items are clickable and re-run the search with the clicked filter;
// facet values are rendered as text (never innerHTML).
func TestStorefrontClickableFacets(t *testing.T) {
	for _, frag := range []string{
		"function toggleFacet(kind, val)", "function renderFacetGroup(", "function renderActiveFilters()", "function priceBounds(",
		"'&category=' + encodeURIComponent(facetFilters.category)", "'&brand=' + encodeURIComponent(facetFilters.brand)",
		"'&min_price=' + pb.min", "'&max_price=' + pb.max", `id="active-filters"`, ".facet-active", "runSearch({ keepFacets: true })",
		"renderFacetGroup('facets-cat-list', facets.categories, 'category')", "renderFacetGroup('facets-avail-list', facets.availability, 'availability')",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
	for _, bad := range []string{"renderFacetList", "-list').innerHTML"} {
		if strings.Contains(defaultStorefrontHTML, bad) {
			t.Errorf("facets must not be rendered via innerHTML (%q found)", bad)
		}
	}
}

// #40 (ADR-012): pagination ('Showing X-Y of Z'), Prev/Next + numbered buttons,
// page size 12/24/48, extended sort modes, and reset page=1 on query/filter change.
func TestStorefrontPaginationAndSort(t *testing.T) {
	for _, frag := range []string{
		`id="page_size"`,
		`<option value="12">12 per page</option>`,
		`<option value="24" selected>24 per page</option>`,
		`<option value="48">48 per page</option>`,
		`<option value="relevance">Relevance</option>`,
		`<option value="price_asc">Price: Low to High</option>`,
		`<option value="price_desc">Price: High to Low</option>`,
		`<option value="popularity">Best Selling</option>`,
		`<option value="rating_desc">Top Rated</option>`,
		`id="pagination-bar"`,
		"function goToPage(",
		"function renderPagination(",
		"'Showing ' + startItem + '-' + endItem + ' of ' + totalFound + ' results'",
		"if (!opts.keepPage) {\n        currentPage = 1;",
		"'&page=' + encodeURIComponent(currentPage)",
		"'&offset=' + encodeURIComponent((currentPage - 1) * pageSize)",
		`id="ann-badge"`,
		"data.vector_index_used",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
}

// #41 (ADR-013): multi-select checkbox facets + active filter pills + Clear all
// sending brands/categories/colors/sizes/price_ranges arrays.
func TestStorefrontDisjunctiveMultiSelectFacets(t *testing.T) {
	for _, frag := range []string{
		"function getFacetArray(",
		"function syncFacetDimension(",
		"function clearAllFacets()",
		"cb.type = 'checkbox'",
		"cb.className = 'facet-checkbox'",
		"clearBtn.textContent = 'Clear all'",
		`id="sidebar-clear-facets"`,
		"clearBtn.id = 'clear-all-filters-btn'",
		"'&categories=' + encodeURIComponent(activeCategories.join(','))",
		"'&brands=' + encodeURIComponent(activeBrands.join(','))",
		"'&colors=' + encodeURIComponent(activeColors.join(','))",
		"'&sizes=' + encodeURIComponent(activeSizes.join(','))",
		"'&price_ranges=' + encodeURIComponent(activePriceRanges.join(','))",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
}

// #42 (ADR-014): auto-rendered facets.attributes groups + spec badges (attr.<name>).
func TestStorefrontDynamicAttributeFacetsAndSpecs(t *testing.T) {
	for _, frag := range []string{
		`id="facets-dynamic-groups"`,
		"function humanizeAttrName(",
		"function renderDynamicAttrFacets(",
		"function extractCardSpecs(",
		"renderDynamicAttrFacets(facets.attributes)",
		"'&attr.' + encodeURIComponent(attrName) + '=' + encodeURIComponent(vals.join(','))",
		"if (Object.keys(attrsMap).length > 0) body.attrs = attrsMap;",
		`class="spec-badges"`,
		`data-testid="product-spec-badges"`,
		`badge.setAttribute('data-testid', 'spec-badge')`,
		"const topSpecs = extractCardSpecs(p).slice(0, 3);",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
}

// #44 (ADR-016): Similar Products / Frequently Viewed Together carousels and
// 'See In-Stock Alternatives' on out-of-stock cards.
func TestStorefrontSimilarAndSubstitutes(t *testing.T) {
	for _, frag := range []string{
		`id="product-details-modal"`,
		`data-testid="similar-products-carousel"`,
		`data-testid="co-viewed-carousel"`,
		"Similar Products",
		"Frequently Viewed Together",
		"See In-Stock Alternatives",
		`data-testid="see-substitutes-btn"`,
		`data-testid="substitutes-panel"`,
		"function buildSimilarUrl(",
		"function openProductDetailsModal(",
		"function loadInStockSubstitutes(",
		"buildSimilarUrl(p.id, 'similar', 6)",
		"buildSimilarUrl(p.id, 'co_viewed', 6)",
		"buildSimilarUrl(productId, 'substitutes', 4)",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
}

// #43 (ADR-015): click/cart beacons with query_id/position/experiment_id,
// Experiment selector, and Search Analytics drawer.
func TestStorefrontAnalyticsAndAttribution(t *testing.T) {
	for _, frag := range []string{
		`id="experiment_id"`,
		`data-testid="experiment-select"`,
		`<option value="control">control</option>`,
		`<option value="ltr_v2">ltr_v2</option>`,
		`id="open-analytics-btn"`,
		`id="analytics-drawer"`,
		`data-testid="search-analytics-drawer"`,
		`data-testid="analytics-ctr"`,
		`data-testid="analytics-mrr10"`,
		`data-testid="top-zero-result-queries-section"`,
		`data-testid="ab-experiment-comparison-section"`,
		`data-testid="add-to-cart-btn"`,
		"function recordAnalyticsEvent(",
		"function toggleAnalyticsDrawer(",
		"function loadAnalyticsSummary()",
		"'/api/analytics/events'",
		"'/api/analytics/summary?tenant_id='",
		"'&experiment_id=' + encodeURIComponent(experimentId)",
		"query_id: lastQueryId || ''",
		"position: typeof position === 'number' && position > 0 ? position : 1",
		"experiment_id: expId",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}
}

func srgbRelLumChannel(c uint64) float64 {
	v := float64(c) / 255.0
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

func hexRelLuminance(t *testing.T, hexColor string) float64 {
	t.Helper()
	h := strings.TrimPrefix(strings.TrimSpace(hexColor), "#")
	if len(h) != 6 {
		t.Fatalf("expected 6-digit hex color, got %q", hexColor)
	}
	r, err1 := strconv.ParseUint(h[0:2], 16, 8)
	g, err2 := strconv.ParseUint(h[2:4], 16, 8)
	b, err3 := strconv.ParseUint(h[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		t.Fatalf("invalid hex color %q", hexColor)
	}
	return 0.2126*srgbRelLumChannel(r) + 0.7152*srgbRelLumChannel(g) + 0.0722*srgbRelLumChannel(b)
}

func wcagContrastRatio(t *testing.T, fg, bg string) float64 {
	t.Helper()
	l1 := hexRelLuminance(t, fg)
	l2 := hexRelLuminance(t, bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// #49: Brasileirão football club themes in the embedded storefront.
func TestStorefrontFootballThemes(t *testing.T) {
	// Default :root theme must remain unchanged.
	wantDefaultRoot := `:root { --bg: #0f172a; --card: #1e293b; --accent: #38bdf8; --text: #f8fafc; --muted: #94a3b8; --green: #22c55e;`
	if !strings.Contains(defaultStorefrontHTML, wantDefaultRoot) {
		t.Errorf("default :root theme must remain unchanged, want %q", wantDefaultRoot)
	}

	// Theme picker, persistence, URL override, and focus visibility must be present.
	for _, frag := range []string{
		`id="theme-select"`,
		`data-testid="theme-select"`,
		`data-testid="theme-picker"`,
		"Tema / Time do coração",
		`<option value="padrao">Padrão</option>`,
		"const THEME_STORAGE_KEY = 'psearch_storefront_theme'",
		"function applyClubTheme(slug, persist)",
		"function initClubTheme()",
		"localStorage.setItem(THEME_STORAGE_KEY, target)",
		"localStorage.getItem(THEME_STORAGE_KEY)",
		"params.get('theme')",
		"document.documentElement.setAttribute('data-theme', target)",
		"document.body.setAttribute('data-theme', target)",
		":focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}

	// All required Brasileirão clubs (plus Bahia, Fortaleza, Athletico-PR) must be defined.
	clubs := []struct {
		slug string
		name string
	}{
		{"padrao", "Padrão"},
		{"flamengo", "Flamengo"},
		{"palmeiras", "Palmeiras"},
		{"corinthians", "Corinthians"},
		{"sao-paulo", "São Paulo"},
		{"santos", "Santos"},
		{"vasco", "Vasco"},
		{"fluminense", "Fluminense"},
		{"botafogo", "Botafogo"},
		{"gremio", "Grêmio"},
		{"internacional", "Internacional"},
		{"atletico-mg", "Atlético-MG"},
		{"cruzeiro", "Cruzeiro"},
		{"bahia", "Bahia"},
		{"fortaleza", "Fortaleza"},
		{"athletico-pr", "Athletico-PR"},
	}

	varRe := regexp.MustCompile(`--([a-z-]+):\s*(#[0-9a-fA-F]{6})`)
	for _, club := range clubs {
		cssRulePrefix := `[data-theme="` + club.slug + `"]`
		if !strings.Contains(defaultStorefrontHTML, cssRulePrefix) {
			t.Errorf("missing CSS rule %s", cssRulePrefix)
		}
		optFrag := `<option value="` + club.slug + `">` + club.name + `</option>`
		if !strings.Contains(defaultStorefrontHTML, optFrag) {
			t.Errorf("missing theme option %s", optFrag)
		}
		jsEntry := `'` + club.slug + `': { name: '` + club.name + `'`
		if !strings.Contains(defaultStorefrontHTML, jsEntry) {
			t.Errorf("missing CLUB_THEMES entry %s", jsEntry)
		}

		// Extract CSS variables for this theme and verify WCAG AA >= 4.5:1 contrast.
		ruleRe := regexp.MustCompile(`\[data-theme="` + regexp.QuoteMeta(club.slug) + `"\]\s*\{([^}]+)\}`)
		m := ruleRe.FindStringSubmatch(defaultStorefrontHTML)
		if m == nil {
			t.Fatalf("could not parse CSS block for theme %q", club.slug)
		}
		vars := make(map[string]string)
		for _, vm := range varRe.FindAllStringSubmatch(m[1], -1) {
			vars[vm[1]] = vm[2]
		}
		for _, reqVar := range []string{"bg", "card", "accent", "text", "muted", "green", "btn-text", "chip-bg", "chip-text", "oos"} {
			if vars[reqVar] == "" {
				t.Fatalf("theme %q missing CSS variable --%s", club.slug, reqVar)
			}
		}

		pairs := []struct {
			label string
			fg    string
			bg    string
		}{
			{"text/bg", vars["text"], vars["bg"]},
			{"text/card", vars["text"], vars["card"]},
			{"muted/bg", vars["muted"], vars["bg"]},
			{"muted/card", vars["muted"], vars["card"]},
			{"accent/bg", vars["accent"], vars["bg"]},
			{"accent/card", vars["accent"], vars["card"]},
			{"btn-text/accent", vars["btn-text"], vars["accent"]},
			{"chip-text/chip-bg", vars["chip-text"], vars["chip-bg"]},
			{"oos/card", vars["oos"], vars["card"]},
		}
		for _, p := range pairs {
			ratio := wcagContrastRatio(t, p.fg, p.bg)
			if ratio < 4.5 {
				t.Errorf("theme %q pair %s (%s on %s) contrast = %.2f:1, want >= 4.5:1 (WCAG AA)", club.slug, p.label, p.fg, p.bg, ratio)
			}
		}
	}

	// Zero-dependency check: no external HTTP/HTTPS assets, CDN fonts, or crest images.
	if strings.Contains(defaultStorefrontHTML, "http://") || strings.Contains(defaultStorefrontHTML, "https://") {
		t.Error("storefront must remain zero-dependency with no external URLs, CDNs, or crest images")
	}
}

// #50: World country flag-colour themes (prefixed 'pais-') grouped via <optgroup> in #theme-select.
func TestStorefrontCountryThemes(t *testing.T) {
	for _, frag := range []string{
		`<optgroup label="Times do Brasileirão"`,
		`<optgroup label="Países"`,
		"if (t.tertiary) {",
		"linear-gradient(135deg, ' + t.primary + ' 0% 33%, ' + t.secondary + ' 33% 66%, ' + t.tertiary + ' 66% 100%)'",
	} {
		if !strings.Contains(defaultStorefrontHTML, frag) {
			t.Errorf("storefront missing %q", frag)
		}
	}

	countries := []struct {
		slug string
		name string
	}{
		{"pais-brasil", "Brasil"},
		{"pais-argentina", "Argentina"},
		{"pais-estados-unidos", "Estados Unidos"},
		{"pais-mexico", "México"},
		{"pais-canada", "Canadá"},
		{"pais-reino-unido", "Reino Unido"},
		{"pais-franca", "França"},
		{"pais-alemanha", "Alemanha"},
		{"pais-italia", "Itália"},
		{"pais-espanha", "Espanha"},
		{"pais-portugal", "Portugal"},
		{"pais-holanda", "Holanda"},
		{"pais-belgica", "Bélgica"},
		{"pais-japao", "Japão"},
		{"pais-coreia-do-sul", "Coreia do Sul"},
		{"pais-china", "China"},
		{"pais-india", "Índia"},
		{"pais-australia", "Austrália"},
		{"pais-africa-do-sul", "África do Sul"},
		{"pais-nigeria", "Nigéria"},
		{"pais-uruguai", "Uruguai"},
		{"pais-colombia", "Colômbia"},
		{"pais-chile", "Chile"},
		{"pais-croacia", "Croácia"},
		{"pais-marrocos", "Marrocos"},
		{"pais-egito", "Egito"},
		{"pais-arabia-saudita", "Arábia Saudita"},
	}

	varRe := regexp.MustCompile(`--([a-z-]+):\s*(#[0-9a-fA-F]{6})`)
	for _, c := range countries {
		if !strings.HasPrefix(c.slug, "pais-") {
			t.Fatalf("country slug %q must be prefixed with 'pais-'", c.slug)
		}
		cssRulePrefix := `[data-theme="` + c.slug + `"]`
		if !strings.Contains(defaultStorefrontHTML, cssRulePrefix) {
			t.Errorf("missing CSS rule %s", cssRulePrefix)
		}
		optFrag := `<option value="` + c.slug + `">` + c.name + `</option>`
		if !strings.Contains(defaultStorefrontHTML, optFrag) {
			t.Errorf("missing country theme option %s", optFrag)
		}
		jsEntry := `'` + c.slug + `': { name: '` + c.name + `'`
		if !strings.Contains(defaultStorefrontHTML, jsEntry) {
			t.Errorf("missing CLUB_THEMES entry %s", jsEntry)
		}

		// Extract CSS variables for this country theme and verify WCAG AA >= 4.5:1 contrast.
		ruleRe := regexp.MustCompile(`\[data-theme="` + regexp.QuoteMeta(c.slug) + `"\]\s*\{([^}]+)\}`)
		m := ruleRe.FindStringSubmatch(defaultStorefrontHTML)
		if m == nil {
			t.Fatalf("could not parse CSS block for country theme %q", c.slug)
		}
		vars := make(map[string]string)
		for _, vm := range varRe.FindAllStringSubmatch(m[1], -1) {
			vars[vm[1]] = vm[2]
		}
		for _, reqVar := range []string{"bg", "card", "accent", "text", "muted", "green", "btn-text", "chip-bg", "chip-text", "oos"} {
			if vars[reqVar] == "" {
				t.Fatalf("country theme %q missing CSS variable --%s", c.slug, reqVar)
			}
		}

		pairs := []struct {
			label string
			fg    string
			bg    string
		}{
			{"text/bg", vars["text"], vars["bg"]},
			{"text/card", vars["text"], vars["card"]},
			{"muted/bg", vars["muted"], vars["bg"]},
			{"muted/card", vars["muted"], vars["card"]},
			{"accent/bg", vars["accent"], vars["bg"]},
			{"accent/card", vars["accent"], vars["card"]},
			{"btn-text/accent", vars["btn-text"], vars["accent"]},
			{"chip-text/chip-bg", vars["chip-text"], vars["chip-bg"]},
			{"oos/card", vars["oos"], vars["card"]},
		}
		for _, p := range pairs {
			ratio := wcagContrastRatio(t, p.fg, p.bg)
			if ratio < 4.5 {
				t.Errorf("country theme %q pair %s (%s on %s) contrast = %.2f:1, want >= 4.5:1 (WCAG AA)", c.slug, p.label, p.fg, p.bg, ratio)
			}
		}
	}
}
