package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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


