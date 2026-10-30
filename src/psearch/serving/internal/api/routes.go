package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"psearch/serving-go/internal/config"
)

func SetupRouter(router *gin.Engine, cfg *config.Config) {
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	router.Use(LoggerMiddleware())

	controller, err := NewController(cfg)
	if err != nil {
		panic(err)
	}

	router.GET("/health", controller.HealthCheck)
	router.GET("/healthz", controller.HealthCheck)
	router.GET("/api/health", controller.HealthCheck)
	router.GET("/api/info", controller.Info)
	router.GET("/api/config/embedding-version", controller.GetEmbeddingVersion)
	router.POST("/api/config/embedding-version", controller.SetEmbeddingVersion)
	router.GET("/api/synonyms", controller.ListSynonyms)
	router.POST("/api/synonyms", controller.UpsertSynonym)
	router.GET("/api/autocomplete", controller.Autocomplete)
	router.POST("/api/session/events", controller.RecordSessionEvent)
	router.GET("/api/session/events", controller.ListSessionEvents)
	router.GET("/api/rules", controller.ListRules)
	router.POST("/api/rules", controller.CreateRule)
	router.PUT("/api/rules/:id", controller.UpdateRule)
	router.DELETE("/api/rules/:id", controller.DeleteRule)
	router.GET("/api/ads/campaigns", controller.ListAdCampaigns)
	router.POST("/api/ads/campaigns", controller.CreateAdCampaign)
	router.PUT("/api/ads/campaigns/:id", controller.UpdateAdCampaign)
	router.DELETE("/api/ads/campaigns/:id", controller.DeleteAdCampaign)
	router.POST("/api/ads/click", controller.RecordAdClick)
	router.GET("/search", controller.Search)
	router.POST("/search", controller.Search)
	router.GET("/api/search", controller.Search)
	router.POST("/api/search", controller.Search)
	registerWave6Routes(router, controller)

	if cfg.StaticDir != "" {
		if info, err := os.Stat(cfg.StaticDir); err == nil && info.IsDir() {
			router.NoRoute(func(c *gin.Context) {
				reqPath := filepath.Clean(c.Request.URL.Path)
				fullPath := filepath.Join(cfg.StaticDir, reqPath)
				if st, err := os.Stat(fullPath); err == nil && !st.IsDir() {
					c.File(fullPath)
					return
				}
				c.File(filepath.Join(cfg.StaticDir, "index.html"))
			})
			return
		}
	}

	router.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(defaultStorefrontHTML))
	})
}

const defaultStorefrontHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>psearch — Hybrid Vector & Full-Text Search (Cloud Spanner)</title>
  <style>
    :root { --bg: #0f172a; --card: #1e293b; --accent: #38bdf8; --text: #f8fafc; --muted: #94a3b8; --green: #22c55e; }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: var(--bg); color: var(--text); }
    header { padding: 20px 32px; border-bottom: 1px solid #334155; display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 12px; }
    .badge { background: #0284c7; color: #fff; padding: 4px 10px; border-radius: 999px; font-size: 12px; font-weight: 600; }
    main { max-width: 1200px; margin: 0 auto; padding: 24px 32px; }
    .search-bar { display: flex; gap: 12px; align-items: center; background: var(--card); padding: 16px; border-radius: 12px; border: 1px solid #334155; flex-wrap: wrap; }
    input[type="text"] { flex: 1; min-width: 240px; padding: 12px 16px; border-radius: 8px; border: 1px solid #475569; background: #0f172a; color: #fff; font-size: 16px; }
    .slider-wrap { display: flex; align-items: center; gap: 8px; font-size: 14px; color: var(--muted); }
    button { background: var(--accent); color: #0f172a; border: none; padding: 12px 20px; border-radius: 8px; font-weight: 700; cursor: pointer; }
    .meta { margin: 16px 0; color: var(--muted); font-size: 14px; display: flex; gap: 20px; }
    .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 16px; }
    .card { background: var(--card); border: 1px solid #334155; border-radius: 12px; padding: 16px; display: flex; flex-direction: column; gap: 8px; }
    .card.card-sponsored { background: linear-gradient(180deg, rgba(245, 158, 11, 0.12) 0%, var(--card) 100%); border-color: #f59e0b; box-shadow: 0 0 0 1px rgba(245, 158, 11, 0.25); }
    .sponsored-badge { background: #78350f; border: 1px solid #f59e0b; color: #fef3c7; font-size: 11px; font-weight: 700; padding: 2px 8px; border-radius: 999px; letter-spacing: 0.02em; }
    .card h3 { margin: 0; font-size: 16px; }
    .price { font-size: 20px; font-weight: 700; color: var(--accent); }
    .stock-in { color: var(--green); font-size: 12px; font-weight: 600; }
    .stock-out { color: #f87171; font-size: 12px; font-weight: 600; }
    .scores { font-family: monospace; font-size: 11px; color: var(--muted); background: #0f172a; padding: 6px 8px; border-radius: 6px; }
    select { padding: 10px 14px; border-radius: 8px; border: 1px solid #475569; background: #0f172a; color: #fff; font-size: 14px; }
    .checkbox-wrap { display: flex; align-items: center; gap: 6px; font-size: 14px; color: var(--muted); cursor: pointer; user-select: none; }
    input[type="checkbox"] { width: 16px; height: 16px; accent-color: var(--accent); cursor: pointer; }
    .facet-group { margin-bottom: 14px; }
    .facet-title { font-size: 13px; font-weight: 600; color: #cbd5e1; margin-bottom: 6px; }
    .facet-list { font-size: 12px; color: var(--muted); display: flex; flex-direction: column; gap: 4px; }
    .facet-item { display: flex; justify-content: space-between; align-items: center; padding: 2px 0; }
    .facet-click { cursor: pointer; border-radius: 4px; padding: 2px 4px; margin: 0 -4px; }
    .facet-click:hover, .facet-click:focus { background: #1e293b; color: #f8fafc; outline: none; }
    .facet-active { background: #0c4a6e; color: #e0f2fe; font-weight: 600; }
    .filter-chip { display: inline-flex; align-items: center; gap: 4px; background: #0c4a6e; border: 1px solid #38bdf8; color: #e0f2fe; border-radius: 999px; padding: 2px 8px; font-size: 12px; cursor: pointer; }
    .facet-count { background: #0f172a; padding: 1px 6px; border-radius: 999px; font-size: 11px; color: #cbd5e1; }
    .page-btn { background: #1e293b; color: #e2e8f0; border: 1px solid #475569; padding: 6px 12px; border-radius: 6px; font-size: 13px; font-weight: 600; cursor: pointer; }
    .page-btn:disabled { opacity: 0.45; cursor: not-allowed; }
    .page-btn-active { background: var(--accent); color: #0f172a; border-color: var(--accent); }
  </style>
</head>
<body>
  <header>
    <div>
      <h1 style="margin:0;font-size:22px;">psearch — Enterprise Hybrid & Vector Search</h1>
      <div style="color:var(--muted);font-size:13px;">Powered by Cloud Spanner (RRF Vector + Full-Text Search) & Vertex AI</div>
    </div>
    <div style="display:flex;gap:8px;align-items:center;">
      <span class="badge" data-testid="repo-badge">Repo: duboc/scion-experiment | Sandbox: riojucu-sandbox</span>
      <span class="badge" id="backend-badge">Spanner</span>
      <span class="badge" id="retrieval-badge">Retrieval: -</span>
      <span class="badge" id="ann-badge" style="display:none;background:#065f46;border:1px solid #10b981;color:#d1fae5;"></span>
      <span class="badge" id="tenant-badge" style="display:none;"></span>
      <span class="badge" id="locale-badge" style="display:none;"></span>
      <span class="badge" id="ltr-badge" style="display:none;"></span>
      <span class="badge" id="intent-badge" style="display:none;"></span>
      <span class="badge" id="price-table-badge" style="display:none;"></span>
      <span class="badge" id="sort-badge" style="display:none;"></span>
      <span class="badge" id="personalized-badge" style="display:none;background:#3b0764;border-color:#a855f7;color:#e9d5ff;"></span>
      <span class="badge" id="sponsored-badge" style="display:none;background:#78350f;border:1px solid #f59e0b;color:#fef3c7;"></span>
      <span class="badge" id="experiment-badge" style="display:none;background:#1e3a8a;border:1px solid #60a5fa;color:#dbeafe;"></span>
      <span class="badge" id="query-id-badge" style="display:none;background:#1e293b;border:1px solid #64748b;color:#cbd5e1;"></span>
    </div>
  </header>
  <main>
    <div class="search-bar">
      <input type="text" id="q" list="q-suggestions" value="" placeholder="Search products (e.g. running shoes, tenis corrida, laptop, geladeira) — empty shows the full catalog" autocomplete="off" onkeydown="if(event.key==='Enter') runSearch()" />
      <datalist id="q-suggestions"></datalist>
      <input type="file" id="image-file" accept="image/jpeg,image/png,image/webp" style="display:none" onchange="onImagePicked(this)" />
      <button type="button" id="image-btn" onclick="document.getElementById('image-file').click()" title="Search by photo; the text box refines it (e.g. mas em verde)">&#128247; Photo</button>
      <span id="image-chip" style="display:none;align-items:center;gap:6px;background:#1e293b;border:1px solid #a855f7;border-radius:8px;padding:2px 6px;">
        <img id="image-thumb" alt="query photo" style="width:32px;height:32px;object-fit:cover;border-radius:4px;" />
        <span style="font-size:11px;color:#e9d5ff;">photo query</span>
        <button type="button" id="image-clear" onclick="clearQueryImage()" title="Remove photo" style="padding:0 6px;">&times;</button>
      </span>
      <div class="slider-wrap">
        <span>FTS (0.0)</span>
        <input type="range" id="alpha" min="0" max="1" step="0.1" value="0.5" oninput="alphaTouched=true;document.getElementById('alpha-val').textContent=this.value" />
        <span>Vector (1.0) · α=<strong id="alpha-val">0.5</strong></span>
      </div>
      <input type="text" id="session_id" placeholder="Session (sess-...)" style="flex:0 0 140px;min-width:120px;" title="ID da Sessão" onkeydown="if(event.key==='Enter') runSearch()" />
      <input type="text" id="tenant_id" value="default" placeholder="Tenant" style="flex:0 0 110px;min-width:90px;" title="Tenant ID" />
      <select id="locale" title="Locale" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="">All Locales</option>
        <option value="pt-BR">pt-BR</option>
        <option value="en-US">en-US</option>
        <option value="es-MX">es-MX</option>
      </select>
      <select id="channel" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="">All Channels</option>
        <option value="web">web</option>
        <option value="mobile">mobile</option>
        <option value="store">store</option>
      </select>
      <select id="price_table_id" title="Tabela de preço" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="">Varejo Padrão</option>
        <option value="b2b-corporate">B2B Corporate</option>
        <option value="b2b-gold">B2B Gold</option>
        <option value="vip-club">VIP Club</option>
      </select>
      <select id="sort_by" title="Sort by" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="relevance">Relevance</option>
        <option value="price_asc">Price: Low to High</option>
        <option value="price_desc">Price: High to Low</option>
        <option value="popularity">Best Selling</option>
        <option value="rating_desc">Top Rated</option>
      </select>
      <select id="page_size" title="Page size" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="12">12 per page</option>
        <option value="24" selected>24 per page</option>
        <option value="48">48 per page</option>
      </select>
      <select id="color" title="Cor" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="">Todas as cores</option>
        <option value="preto">preto</option>
        <option value="branco">branco</option>
        <option value="azul">azul</option>
        <option value="prata">prata</option>
        <option value="grafite">grafite</option>
      </select>
      <select id="size" title="Tamanho" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="">Todos os tamanhos</option>
        <option value="38">38</option>
        <option value="39">39</option>
        <option value="40">40</option>
        <option value="41">41</option>
        <option value="42">42</option>
        <option value="43">43</option>
      </select>
      <input type="text" id="cep" placeholder="CEP (01310-100)" style="flex:0 0 130px;min-width:110px;" title="Calcular Entrega" onkeydown="if(event.key==='Enter') runSearch()" />
      <label class="checkbox-wrap">
        <input type="checkbox" id="max_sla_24h" onchange="currentPage=1;runSearch({ keepFacets: true })" /> 24h
      </label>
      <label class="checkbox-wrap">
        <input type="checkbox" id="only_in_stock" onchange="currentPage=1;runSearch({ keepFacets: true })" /> Only in stock
      </label>
      <label class="checkbox-wrap">
        <input type="checkbox" id="ltr" onchange="currentPage=1;runSearch({ keepFacets: true })" /> Re-rank (LTR)
      </label>
      <label class="checkbox-wrap">
        <input type="checkbox" id="parse_intent" checked onchange="currentPage=1;runSearch({ keepFacets: true })" /> Smart query
      </label>
      <select id="experiment_id" data-testid="experiment-select" title="A/B Experiment Cohort" onchange="currentPage=1;runSearch({ keepFacets: true })">
        <option value="">Experiment: Default</option>
        <option value="control">control</option>
        <option value="ltr_v2">ltr_v2</option>
        <option value="ltr_boost">ltr_boost</option>
      </select>
      <button onclick="runSearch()">Search</button>
      <button id="show-all-btn" onclick="showAllCatalog()" title="Show every product across locales">Show All Catalog</button>
      <button type="button" id="open-analytics-btn" data-testid="open-analytics-drawer-btn" onclick="toggleAnalyticsDrawer()" style="background:#1e293b;color:#38bdf8;border:1px solid #38bdf8;">Search Analytics</button>
    </div>
    <div id="analytics-drawer" data-testid="search-analytics-drawer" style="display:none;margin-top:20px;padding:18px;background:var(--card);border:1px solid #38bdf8;border-radius:12px;">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:14px;gap:12px;flex-wrap:wrap;">
        <div>
          <span style="font-size:11px;color:var(--accent);text-transform:uppercase;font-weight:700;">Search Telemetry &amp; Attribution (ADR-015)</span>
          <h3 style="margin:4px 0 0 0;font-size:16px;">Search Analytics Summary</h3>
        </div>
        <div style="display:flex;gap:8px;align-items:center;">
          <button type="button" class="page-btn" onclick="loadAnalyticsSummary()">Refresh</button>
          <button type="button" class="page-btn" onclick="toggleAnalyticsDrawer(false)">Close &times;</button>
        </div>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(170px,1fr));gap:12px;margin-bottom:16px;">
        <div style="background:#0f172a;border:1px solid #334155;border-radius:8px;padding:12px;">
          <div style="font-size:11px;color:var(--muted);">CTR (Click-Through Rate)</div>
          <div id="analytics-ctr" data-testid="analytics-ctr" style="font-size:20px;font-weight:700;color:var(--accent);margin-top:4px;">0.0%</div>
          <div id="analytics-searches-sub" style="font-size:11px;color:var(--muted);margin-top:2px;">0 clicks / 0 searches</div>
        </div>
        <div style="background:#0f172a;border:1px solid #334155;border-radius:8px;padding:12px;">
          <div style="font-size:11px;color:var(--muted);">MRR@10</div>
          <div id="analytics-mrr10" data-testid="analytics-mrr10" style="font-size:20px;font-weight:700;color:var(--accent);margin-top:4px;">0.000</div>
          <div style="font-size:11px;color:var(--muted);margin-top:2px;">Mean Reciprocal Rank</div>
        </div>
        <div style="background:#0f172a;border:1px solid #334155;border-radius:8px;padding:12px;">
          <div style="font-size:11px;color:var(--muted);">Zero-Result Rate</div>
          <div id="analytics-zero-rate" data-testid="analytics-zero-result-rate" style="font-size:20px;font-weight:700;color:#f87171;margin-top:4px;">0.0%</div>
          <div id="analytics-zero-sub" style="font-size:11px;color:var(--muted);margin-top:2px;">0 zero-result queries</div>
        </div>
        <div style="background:#0f172a;border:1px solid #334155;border-radius:8px;padding:12px;">
          <div style="font-size:11px;color:var(--muted);">Conversion Rate</div>
          <div id="analytics-conv-rate" data-testid="analytics-conversion-rate" style="font-size:20px;font-weight:700;color:#22c55e;margin-top:4px;">0.0%</div>
          <div id="analytics-conv-sub" style="font-size:11px;color:var(--muted);margin-top:2px;">0 conversions</div>
        </div>
      </div>
      <div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px;">
        <div data-testid="top-zero-result-queries-section" style="background:#0f172a;border:1px solid #334155;border-radius:8px;padding:12px;">
          <div style="font-size:13px;font-weight:700;color:var(--accent);margin-bottom:8px;">Top Zero-Result Queries</div>
          <div id="analytics-zero-queries" style="font-size:12px;color:var(--muted);display:flex;flex-direction:column;gap:6px;">No zero-result queries recorded.</div>
        </div>
        <div data-testid="ab-experiment-comparison-section" style="background:#0f172a;border:1px solid #334155;border-radius:8px;padding:12px;">
          <div style="font-size:13px;font-weight:700;color:var(--accent);margin-bottom:8px;">A/B Experiment Comparison (control vs ltr_v2)</div>
          <div id="analytics-experiments" style="font-size:12px;color:var(--muted);display:flex;flex-direction:column;gap:6px;">No experiment cohorts recorded yet.</div>
        </div>
      </div>
    </div>
    <div style="display:flex;gap:24px;margin-top:20px;align-items:flex-start;">
      <aside id="facet-sidebar" style="width:230px;flex-shrink:0;background:var(--card);padding:16px;border-radius:12px;border:1px solid #334155;">
        <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;gap:6px;">
          <h3 style="margin:0;font-size:15px;color:var(--accent);">Facets</h3>
          <div style="display:flex;align-items:center;gap:8px;">
            <span id="facets-latency" style="font-size:11px;color:var(--muted);"></span>
            <button type="button" id="sidebar-clear-facets" onclick="clearAllFacets()" style="display:none;background:transparent;border:none;color:var(--accent);font-size:11px;font-weight:600;padding:0;cursor:pointer;text-decoration:underline;">Clear all</button>
          </div>
        </div>
        <div class="facet-group">
          <div class="facet-title">Department</div>
          <div class="facet-list" id="facets-cat-list">-</div>
        </div>
        <div class="facet-group">
          <div class="facet-title">Brands</div>
          <div class="facet-list" id="facets-brand-list">-</div>
        </div>
        <div class="facet-group">
          <div class="facet-title">Price Ranges</div>
          <div class="facet-list" id="facets-price-list">-</div>
        </div>
        <div class="facet-group">
          <div class="facet-title">Colors</div>
          <div class="facet-list" id="facets-color-list">-</div>
        </div>
        <div class="facet-group">
          <div class="facet-title">Sizes</div>
          <div class="facet-list" id="facets-size-list">-</div>
        </div>
        <div class="facet-group">
          <div class="facet-title">Availability</div>
          <div class="facet-list" id="facets-avail-list">-</div>
        </div>
        <div id="facets-dynamic-groups"></div>
      </aside>
      <div style="flex:1;min-width:0;">
        <div id="active-filters" style="display:none;flex-wrap:wrap;gap:6px;margin-bottom:10px;"></div>
        <div class="meta" id="meta">Loading catalog...</div>
        <div id="expanded-terms" style="color:var(--muted);font-size:13px;margin:-8px 0 16px 0;display:none;"></div>
        <div id="recovery-banner" style="display:none;margin:-8px 0 16px 0;padding:10px 14px;background:var(--card);border:1px solid #38bdf8;border-radius:8px;font-size:14px;color:var(--text);"></div>
        <div id="parsed-query-chips" style="display:none;margin:-8px 0 16px 0;gap:8px;flex-wrap:wrap;align-items:center;"></div>
        <div class="grid" id="results"></div>
        <div id="pagination-bar" style="display:none;justify-content:space-between;align-items:center;margin-top:20px;flex-wrap:wrap;gap:12px;"></div>
        <div id="product-details-modal" data-testid="product-recommendations-section" style="display:none;margin-top:24px;padding:18px;background:var(--card);border:1px solid #38bdf8;border-radius:12px;">
          <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:12px;gap:12px;flex-wrap:wrap;">
            <div>
              <span style="font-size:11px;color:var(--accent);text-transform:uppercase;font-weight:700;">Item-to-Item Recommendations (ADR-016)</span>
              <h3 id="modal-product-title" style="margin:4px 0 0 0;font-size:16px;">Product Recommendations</h3>
            </div>
            <div style="display:flex;gap:8px;align-items:center;">
              <button type="button" id="tab-similar-btn" class="page-btn page-btn-active" onclick="switchRecTab('all')">All</button>
              <button type="button" id="tab-only-similar-btn" class="page-btn" onclick="switchRecTab('similar')">Similar Products</button>
              <button type="button" id="tab-only-coviewed-btn" class="page-btn" onclick="switchRecTab('co_viewed')">Frequently Viewed Together</button>
              <button type="button" id="close-details-modal-btn" class="page-btn" onclick="closeProductDetailsModal()">Close &times;</button>
            </div>
          </div>
          <div id="section-similar" data-testid="similar-products-carousel" style="margin-bottom:16px;">
            <div style="font-size:13px;font-weight:700;color:var(--accent);margin-bottom:8px;">Similar Products</div>
            <div id="carousel-similar" style="display:flex;gap:12px;overflow-x:auto;padding-bottom:6px;"></div>
          </div>
          <div id="section-co-viewed" data-testid="co-viewed-carousel">
            <div style="font-size:13px;font-weight:700;color:var(--accent);margin-bottom:8px;">Frequently Viewed Together</div>
            <div id="carousel-co-viewed" style="display:flex;gap:12px;overflow-x:auto;padding-bottom:6px;"></div>
          </div>
        </div>
      </div>
    </div>
  </main>
  <script>
    // Visual search (#33, ADR-010): the photo is resized client-side (max 512 px,
    // JPEG) and sent inline as a data URL; the text box acts as the refinement.
    let queryImage = null;
    function onImagePicked(input) {
      const file = input.files && input.files[0];
      input.value = '';
      if (!file) return;
      if (['image/jpeg', 'image/png', 'image/webp'].indexOf(file.type) < 0) {
        document.getElementById('meta').textContent = 'Error: photo must be JPEG, PNG or WebP';
        return;
      }
      const reader = new FileReader();
      reader.onload = function() {
        const img = new Image();
        img.onload = function() {
          const max = 512;
          const scale = Math.min(1, max / Math.max(img.width, img.height));
          const canvas = document.createElement('canvas');
          canvas.width = Math.max(1, Math.round(img.width * scale));
          canvas.height = Math.max(1, Math.round(img.height * scale));
          canvas.getContext('2d').drawImage(img, 0, 0, canvas.width, canvas.height);
          queryImage = canvas.toDataURL('image/jpeg', 0.85);
          document.getElementById('image-thumb').src = queryImage;
          document.getElementById('image-chip').style.display = 'inline-flex';
          runSearch();
        };
        img.onerror = function() {
          document.getElementById('meta').textContent = 'Error: could not read the photo';
        };
        img.src = reader.result;
      };
      reader.readAsDataURL(file);
    }
    function clearQueryImage() {
      queryImage = null;
      document.getElementById('image-chip').style.display = 'none';
      document.getElementById('image-thumb').removeAttribute('src');
      runSearch();
    }
    function imageSearchBody(q, url) {
      // Reuse the GET parameters built by runSearch as the JSON body of POST /api/search.
      const p = new URLSearchParams(url.split('?')[1] || '');
      const body = { query: q, image_base64: queryImage, image_mime_type: 'image/jpeg', limit: parseInt(p.get('limit') || '20', 10) };
      if (p.get('page')) body.page = parseInt(p.get('page'), 10);
      if (p.get('offset')) body.offset = parseInt(p.get('offset'), 10);
      ['channel', 'tenant_id', 'locale', 'cep', 'price_table_id', 'sort_by', 'experiment_id', 'color', 'size', 'session_id', 'category', 'brand'].forEach(function(k) {
        if (p.get(k)) body[k] = p.get(k);
      });
      ['categories', 'brands', 'colors', 'sizes', 'price_ranges'].forEach(function(k) {
        if (p.get(k)) body[k] = p.get(k).split(',').filter(Boolean);
      });
      const attrsMap = {};
      p.forEach(function(val, key) {
        if (key.indexOf('attr.') === 0) {
          const attrName = key.substring(5);
          const vals = val.split(',').filter(Boolean);
          if (attrName && vals.length > 0) attrsMap[attrName] = vals;
        }
      });
      if (Object.keys(attrsMap).length > 0) body.attrs = attrsMap;
      if (p.get('min_price')) body.min_price = parseFloat(p.get('min_price'));
      if (p.get('max_price')) body.max_price = parseFloat(p.get('max_price'));
      if (p.get('only_in_stock') === 'true') body.only_in_stock = true;
      if (p.get('ltr') === 'true') body.ltr = true;
      if (p.get('max_sla_hours')) body.max_sla_hours = parseInt(p.get('max_sla_hours'), 10);
      body.parse_intent = p.get('parse_intent') !== 'false';
      // alpha is omitted unless the slider was moved: the server then weights the
      // vector (photo) leg at 0.7 instead of the text-tuned 0.5.
      if (alphaTouched) body.alpha = parseFloat(p.get('alpha'));
      return body;
    }
    let alphaTouched = false;
    let currentPage = 1;
    function goToPage(p) {
      currentPage = Math.max(1, parseInt(p, 10) || 1);
      runSearch({ keepFacets: true, keepPage: true });
    }
    function renderPagination(data, pageSize) {
      const bar = document.getElementById('pagination-bar');
      if (!bar) return { start: 0, end: 0, total: 0 };
      bar.textContent = '';
      const totalFound = typeof data.total_found === 'number' ? data.total_found : (data.results ? data.results.length : 0);
      const page = typeof data.page === 'number' && data.page > 0 ? data.page : currentPage;
      const effPageSize = typeof data.page_size === 'number' && data.page_size > 0 ? data.page_size : pageSize;
      const offset = typeof data.offset === 'number' && data.offset >= 0 ? data.offset : (page - 1) * effPageSize;
      const totalPages = typeof data.total_pages === 'number' && data.total_pages > 0 ? data.total_pages : Math.max(1, Math.ceil(totalFound / effPageSize));
      const organicResults = (data.results || []).filter(function(r) { return !r.sponsored; });
      const pageCount = organicResults.length || (data.results ? data.results.length : 0);
      const startItem = totalFound === 0 || pageCount === 0 ? 0 : offset + 1;
      const endItem = totalFound === 0 ? 0 : Math.min(totalFound, offset + pageCount);
      const hasNext = typeof data.has_next === 'boolean' ? data.has_next : (page < totalPages);

      const summary = document.createElement('span');
      summary.id = 'pagination-summary';
      summary.style.cssText = 'color:var(--muted);font-size:13px;';
      summary.textContent = 'Showing ' + startItem + '-' + endItem + ' of ' + totalFound + ' results';
      bar.appendChild(summary);

      const controls = document.createElement('div');
      controls.style.cssText = 'display:flex;gap:6px;align-items:center;flex-wrap:wrap;';

      const prevBtn = document.createElement('button');
      prevBtn.type = 'button';
      prevBtn.id = 'page-prev';
      prevBtn.className = 'page-btn';
      prevBtn.textContent = 'Prev';
      prevBtn.disabled = page <= 1;
      prevBtn.onclick = function() { goToPage(page - 1); };
      controls.appendChild(prevBtn);

      for (let i = 1; i <= totalPages; i++) {
        if (totalPages > 7 && i !== 1 && i !== totalPages && Math.abs(i - page) > 1) {
          if (i === 2 || i === totalPages - 1) {
            const dots = document.createElement('span');
            dots.style.cssText = 'color:var(--muted);padding:0 4px;';
            dots.textContent = '...';
            controls.appendChild(dots);
          }
          continue;
        }
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'page-btn' + (i === page ? ' page-btn-active' : '');
        btn.setAttribute('data-page', String(i));
        if (i === page) btn.setAttribute('aria-current', 'page');
        btn.textContent = String(i);
        btn.onclick = (function(targetPage) {
          return function() { goToPage(targetPage); };
        })(i);
        controls.appendChild(btn);
      }

      const nextBtn = document.createElement('button');
      nextBtn.type = 'button';
      nextBtn.id = 'page-next';
      nextBtn.className = 'page-btn';
      nextBtn.textContent = 'Next';
      nextBtn.disabled = !hasNext && page >= totalPages;
      nextBtn.onclick = function() { goToPage(page + 1); };
      controls.appendChild(nextBtn);

      bar.appendChild(controls);
      bar.style.display = 'flex';
      return { start: startItem, end: endItem, total: totalFound };
    }
    // Disjunctive multi-select facets (#34, #41 ADR-013, #42 ADR-014): clicking a facet
    // checkbox toggles that option within its dimension array (categories, brands, colors,
    // sizes, price_ranges, or dynamic attr.<name>) and re-runs the search with page=1.
    let facetFilters = {};
    let collapsedAttrGroups = {};
    let lastSearchedQuery = null;
    const FACET_ARRAY_KEY = {
      category: 'categories',
      brand: 'brands',
      price: 'price_ranges',
      color: 'colors',
      size: 'sizes'
    };
    function humanizeAttrName(name) {
      return String(name || '')
        .replace(/[_-]+/g, ' ')
        .replace(/([A-Z])/g, ' $1')
        .replace(/\s+/g, ' ')
        .trim()
        .replace(/\b\w/g, function(ch) { return ch.toUpperCase(); });
    }
    function getFacetArray(kind) {
      if (kind && kind.indexOf('attr.') === 0) {
        const attrName = kind.substring(5);
        const attrs = facetFilters.attrs || {};
        return Array.isArray(attrs[attrName]) ? attrs[attrName].slice() : [];
      }
      const k = FACET_ARRAY_KEY[kind];
      if (!k) return [];
      const arr = Array.isArray(facetFilters[k]) ? facetFilters[k].slice() : [];
      if (kind === 'color' || kind === 'size') {
        const el = document.getElementById(kind);
        if (el && el.value && !arr.some(function(x) { return String(x).toLowerCase() === el.value.toLowerCase(); })) {
          arr.push(el.value);
        }
      }
      return arr;
    }
    function syncFacetDimension(kind, arr) {
      if (kind && kind.indexOf('attr.') === 0) {
        const attrName = kind.substring(5);
        if (!facetFilters.attrs) facetFilters.attrs = {};
        if (arr && arr.length > 0) {
          facetFilters.attrs[attrName] = arr;
        } else {
          delete facetFilters.attrs[attrName];
        }
        return;
      }
      const k = FACET_ARRAY_KEY[kind];
      if (!k) return;
      if (arr && arr.length > 0) {
        facetFilters[k] = arr;
      } else {
        delete facetFilters[k];
      }
      if (kind === 'category' || kind === 'brand' || kind === 'price') {
        if (arr && arr.length === 1) {
          facetFilters[kind] = arr[0];
        } else {
          delete facetFilters[kind];
        }
      } else if (kind === 'color' || kind === 'size') {
        setSelectValue(kind, (arr && arr.length === 1) ? arr[0] : '');
      }
    }
    function facetIsActive(kind, val) {
      const v = String(val).toLowerCase();
      const arr = getFacetArray(kind);
      if (arr.some(function(x) { return String(x).toLowerCase() === v; })) {
        return true;
      }
      if (kind === 'category' || kind === 'brand' || kind === 'price') {
        return facetFilters[kind] !== undefined && String(facetFilters[kind]).toLowerCase() === v;
      }
      if (kind === 'color' || kind === 'size') {
        const el = document.getElementById(kind);
        return !!el && el.value.toLowerCase() === v;
      }
      if (kind === 'availability') {
        const el = document.getElementById('only_in_stock');
        return !!el && el.checked && val === 'IN_STOCK';
      }
      return false;
    }
    function facetClickable(kind, val) {
      return val !== '' && !(kind === 'availability' && val !== 'IN_STOCK');
    }
    function setSelectValue(id, val) {
      const el = document.getElementById(id);
      if (!el) return;
      if (val && !Array.prototype.some.call(el.options, function(o) { return o.value === val; })) {
        const opt = document.createElement('option');
        opt.value = val;
        opt.textContent = val;
        el.appendChild(opt);
      }
      el.value = val;
    }
    function toggleFacet(kind, val) {
      const active = facetIsActive(kind, val);
      const vLower = String(val).toLowerCase();
      if (FACET_ARRAY_KEY[kind] || (kind && kind.indexOf('attr.') === 0)) {
        let arr = getFacetArray(kind);
        if (active) {
          arr = arr.filter(function(x) { return String(x).toLowerCase() !== vLower; });
        } else {
          arr.push(val);
        }
        syncFacetDimension(kind, arr);
      } else if (kind === 'availability') {
        document.getElementById('only_in_stock').checked = !active;
      }
      currentPage = 1;
      runSearch({ keepFacets: true });
    }
    function clearAllFacets() {
      facetFilters = {};
      const colEl = document.getElementById('color');
      if (colEl) colEl.value = '';
      const szEl = document.getElementById('size');
      if (szEl) szEl.value = '';
      const stockEl = document.getElementById('only_in_stock');
      if (stockEl) stockEl.checked = false;
      currentPage = 1;
      runSearch({ keepFacets: true });
    }
    function priceBounds(range) {
      // Facet buckets: "0-100", "100-500", "500-1500", "1500+".
      const m = /^(\d+(?:\.\d+)?)(?:-(\d+(?:\.\d+)?)|\+)$/.exec(String(range || ''));
      if (!m) return null;
      return { min: parseFloat(m[1]), max: m[2] !== undefined ? parseFloat(m[2]) : 0 };
    }
    function renderFacetGroup(elId, list, kind) {
      const el = document.getElementById(elId);
      if (!el) return;
      el.textContent = '';
      const merged = Array.isArray(list) ? list.slice() : [];
      const activeVals = getFacetArray(kind);
      activeVals.forEach(function(selVal) {
        const exists = merged.some(function(it) {
          const iv = typeof it === 'object' ? String(it.value) : String(it);
          return iv.toLowerCase() === String(selVal).toLowerCase();
        });
        if (!exists) {
          merged.push({ value: selVal, count: 0, selected: true });
        }
      });
      if (!merged.length) {
        const none = document.createElement('span');
        none.style.cssText = 'color:#64748b;font-size:11px;';
        none.textContent = '(none)';
        el.appendChild(none);
        return;
      }
      merged.forEach(function(item) {
        const val = typeof item === 'object' ? String(item.value) : String(item);
        const cnt = typeof item === 'object' ? item.count : 0;
        const row = document.createElement('div');
        row.className = 'facet-item';
        row.setAttribute('data-facet', kind);
        const leftWrap = document.createElement('span');
        leftWrap.style.cssText = 'display:inline-flex;align-items:center;gap:6px;';
        const clickable = facetClickable(kind, val);
        const active = clickable && (Boolean(item && item.selected) || facetIsActive(kind, val));
        if (clickable) {
          const cb = document.createElement('input');
          cb.type = 'checkbox';
          cb.className = 'facet-checkbox';
          cb.checked = active;
          cb.tabIndex = -1;
          cb.style.cssText = 'width:13px;height:13px;margin:0;pointer-events:none;';
          leftWrap.appendChild(cb);
        }
        const label = document.createElement('span');
        label.textContent = val === '' ? '(sem valor)' : val;
        leftWrap.appendChild(label);
        const count = document.createElement('span');
        count.className = 'facet-count';
        count.textContent = String(cnt);
        row.appendChild(leftWrap);
        row.appendChild(count);
        if (clickable) {
          row.classList.add('facet-click');
          if (active) row.classList.add('facet-active');
          row.setAttribute('role', 'checkbox');
          row.setAttribute('tabindex', '0');
          row.setAttribute('aria-checked', active ? 'true' : 'false');
          row.setAttribute('aria-pressed', active ? 'true' : 'false');
          row.title = active ? 'Remover filtro' : 'Filtrar por ' + val;
          row.onclick = function() { toggleFacet(kind, val); };
          row.onkeydown = function(e) { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleFacet(kind, val); } };
        }
        el.appendChild(row);
      });
    }
    function renderDynamicAttrFacets(attrGroups) {
      const container = document.getElementById('facets-dynamic-groups');
      if (!container) return;
      container.textContent = '';
      const normalized = [];
      const seen = {};
      if (Array.isArray(attrGroups)) {
        attrGroups.forEach(function(g) {
          if (!g) return;
          const name = g.name || g.key || '';
          const buckets = Array.isArray(g.buckets) ? g.buckets : (Array.isArray(g.values) ? g.values : []);
          if (name) {
            seen[name] = true;
            normalized.push({ name: name, label: g.label || humanizeAttrName(name), buckets: buckets });
          }
        });
      } else if (attrGroups && typeof attrGroups === 'object') {
        Object.keys(attrGroups).forEach(function(name) {
          const val = attrGroups[name];
          const buckets = Array.isArray(val) ? val : (val && Array.isArray(val.buckets) ? val.buckets : []);
          seen[name] = true;
          normalized.push({ name: name, label: (val && val.label) || humanizeAttrName(name), buckets: buckets });
        });
      }
      const activeAttrs = facetFilters.attrs || {};
      Object.keys(activeAttrs).forEach(function(name) {
        if (!seen[name] && Array.isArray(activeAttrs[name]) && activeAttrs[name].length > 0) {
          normalized.push({ name: name, label: humanizeAttrName(name), buckets: [] });
        }
      });
      normalized.forEach(function(group) {
        const kind = 'attr.' + group.name;
        const activeVals = getFacetArray(kind);
        if ((!group.buckets || group.buckets.length === 0) && activeVals.length === 0) return;
        const wrap = document.createElement('div');
        wrap.className = 'facet-group';
        wrap.setAttribute('data-attr-group', group.name);
        const header = document.createElement('div');
        header.className = 'facet-title';
        header.style.cssText = 'display:flex;justify-content:space-between;align-items:center;cursor:pointer;user-select:none;';
        const titleSpan = document.createElement('span');
        titleSpan.textContent = group.label || humanizeAttrName(group.name);
        const toggleIcon = document.createElement('span');
        const isCollapsed = Boolean(collapsedAttrGroups[group.name]);
        toggleIcon.textContent = isCollapsed ? '\u25b8' : '\u25be';
        header.appendChild(titleSpan);
        header.appendChild(toggleIcon);
        const listEl = document.createElement('div');
        listEl.className = 'facet-list';
        listEl.id = 'facets-attr-' + group.name;
        listEl.style.display = isCollapsed ? 'none' : 'block';
        header.onclick = function() {
          collapsedAttrGroups[group.name] = !collapsedAttrGroups[group.name];
          const nowCollapsed = Boolean(collapsedAttrGroups[group.name]);
          listEl.style.display = nowCollapsed ? 'none' : 'block';
          toggleIcon.textContent = nowCollapsed ? '\u25b8' : '\u25be';
        };
        wrap.appendChild(header);
        wrap.appendChild(listEl);
        container.appendChild(wrap);
        renderFacetGroup(listEl.id, group.buckets, kind);
      });
    }
    function extractCardSpecs(p) {
      if (!p || typeof p !== 'object') return [];
      const specs = [];
      const seen = {};
      function addSpec(k, v) {
        if (!k || v === undefined || v === null || v === '') return;
        const normKey = String(k).trim();
        if (!normKey) return;
        const lower = normKey.toLowerCase();
        if (lower === 'brand' || lower === 'category' || lower === 'color' || lower === 'size' || seen[lower]) return;
        const valStr = Array.isArray(v) ? v.filter(Boolean).join(', ') : String(v).trim();
        if (!valStr) return;
        seen[lower] = true;
        specs.push({ key: normKey, label: humanizeAttrName(normKey), value: valStr });
      }
      [p.dynamic_attributes, p.specs, p.attrs].forEach(function(src) {
        if (src && typeof src === 'object' && !Array.isArray(src)) {
          Object.keys(src).forEach(function(k) { addSpec(k, src[k]); });
        }
      });
      if (Array.isArray(p.attributes)) {
        p.attributes.forEach(function(attr) {
          if (!attr || !attr.key) return;
          const val = (attr.value && Array.isArray(attr.value.text)) ? attr.value.text : (attr.value !== undefined ? attr.value : attr.text);
          addSpec(attr.key, val);
        });
      } else if (p.attributes && typeof p.attributes === 'object') {
        Object.keys(p.attributes).forEach(function(k) {
          const v = p.attributes[k];
          if (v && typeof v === 'object' && !Array.isArray(v) && Array.isArray(v.text)) {
            addSpec(k, v.text);
          } else {
            addSpec(k, v);
          }
        });
      }
      return specs;
    }
    function renderActiveFilters() {
      const box = document.getElementById('active-filters');
      if (!box) return;
      box.textContent = '';
      const chips = [];
      [['category', 'Brand/Dept', 'Category'], ['brand', 'Marca', 'Brand'], ['price', 'Preço', 'Price'], ['color', 'Cor', 'Color'], ['size', 'Tamanho', 'Size']].forEach(function(meta) {
        const kind = meta[0];
        const prefix = meta[2];
        const vals = getFacetArray(kind);
        vals.forEach(function(val) {
          chips.push([prefix + ': ' + val, function() {
            const next = getFacetArray(kind).filter(function(x) { return String(x).toLowerCase() !== String(val).toLowerCase(); });
            syncFacetDimension(kind, next);
          }]);
        });
      });
      Object.keys(facetFilters.attrs || {}).forEach(function(attrName) {
        const kind = 'attr.' + attrName;
        const prefix = humanizeAttrName(attrName);
        const vals = getFacetArray(kind);
        vals.forEach(function(val) {
          chips.push([prefix + ': ' + val, function() {
            const next = getFacetArray(kind).filter(function(x) { return String(x).toLowerCase() !== String(val).toLowerCase(); });
            syncFacetDimension(kind, next);
          }]);
        });
      });
      const stock = document.getElementById('only_in_stock');
      if (stock && stock.checked) chips.push(['In Stock', function() { stock.checked = false; }]);
      chips.forEach(function(c) {
        const chip = document.createElement('button');
        chip.type = 'button';
        chip.className = 'filter-chip';
        chip.textContent = c[0] + ' \u00d7';
        chip.title = 'Remover filtro';
        chip.onclick = function() { c[1](); currentPage = 1; runSearch({ keepFacets: true }); };
        box.appendChild(chip);
      });
      if (chips.length > 0) {
        const clearBtn = document.createElement('button');
        clearBtn.type = 'button';
        clearBtn.id = 'clear-all-filters-btn';
        clearBtn.className = 'filter-chip';
        clearBtn.style.cssText = 'background:#1e293b;border-color:#f87171;color:#fecaca;font-weight:700;';
        clearBtn.textContent = 'Clear all';
        clearBtn.title = 'Clear all active filters';
        clearBtn.onclick = function() { clearAllFacets(); };
        box.appendChild(clearBtn);
      }
      const sideClear = document.getElementById('sidebar-clear-facets');
      if (sideClear) sideClear.style.display = chips.length ? 'inline-block' : 'none';
      box.style.display = chips.length ? 'flex' : 'none';
    }
    let lastQueryId = '';
    let lastExperimentId = '';
    async function runSearch(opts) {
      opts = opts || {};
      const q = document.getElementById('q').value;
      if (!opts.keepFacets && lastSearchedQuery !== null && q.trim() !== lastSearchedQuery) {
        facetFilters = {}; // a new query text starts from a clean facet state
      }
      if (!opts.keepPage) {
        currentPage = 1; // reset page=1 on query or filter change (#40)
      }
      lastSearchedQuery = q.trim();
      const alpha = document.getElementById('alpha').value;
      const channel = document.getElementById('channel').value;
      const onlyInStock = document.getElementById('only_in_stock').checked;
      const tenantId = document.getElementById('tenant_id') ? document.getElementById('tenant_id').value : 'default';
      const locale = document.getElementById('locale') ? document.getElementById('locale').value : '';
      const cep = document.getElementById('cep') ? document.getElementById('cep').value.trim() : '';
      const maxSla24h = document.getElementById('max_sla_24h') ? document.getElementById('max_sla_24h').checked : false;
      const ltr = document.getElementById('ltr') ? document.getElementById('ltr').checked : false;
      const parseIntent = document.getElementById('parse_intent') ? document.getElementById('parse_intent').checked : true;
      const priceTableId = document.getElementById('price_table_id') ? document.getElementById('price_table_id').value : '';
      const sortBy = document.getElementById('sort_by') ? document.getElementById('sort_by').value : '';
      const experimentId = document.getElementById('experiment_id') ? document.getElementById('experiment_id').value : '';
      const pageSizeEl = document.getElementById('page_size');
      const pageSize = pageSizeEl ? (parseInt(pageSizeEl.value, 10) || 24) : 24;
      const color = document.getElementById('color') ? document.getElementById('color').value : '';
      const size = document.getElementById('size') ? document.getElementById('size').value : '';
      const sessionId = document.getElementById('session_id') ? document.getElementById('session_id').value.trim() : '';
      let url = '/api/search?q=' + encodeURIComponent(q) + '&alpha=' + encodeURIComponent(alpha);
      if (!queryImage && opts.browseAll) {
        url += '&limit=100'; // catalog browse: show every product (#30)
      } else {
        url += '&limit=' + encodeURIComponent(pageSize) + '&page=' + encodeURIComponent(currentPage) + '&offset=' + encodeURIComponent((currentPage - 1) * pageSize);
      }
      if (sessionId) {
        url += '&session_id=' + encodeURIComponent(sessionId);
      }
      if (channel) {
        url += '&channel=' + encodeURIComponent(channel);
      }
      if (priceTableId) {
        url += '&price_table_id=' + encodeURIComponent(priceTableId);
      }
      if (sortBy) {
        url += '&sort_by=' + encodeURIComponent(sortBy);
      }
      if (experimentId) {
        url += '&experiment_id=' + encodeURIComponent(experimentId);
      }
      const activeColors = getFacetArray('color');
      if (activeColors.length > 0) {
        url += '&colors=' + encodeURIComponent(activeColors.join(','));
      }
      if (color) {
        url += '&color=' + encodeURIComponent(color);
      }
      const activeCategories = getFacetArray('category');
      if (activeCategories.length > 0) {
        url += '&categories=' + encodeURIComponent(activeCategories.join(','));
      }
      if (facetFilters.category) {
        url += '&category=' + encodeURIComponent(facetFilters.category);
      }
      const activeBrands = getFacetArray('brand');
      if (activeBrands.length > 0) {
        url += '&brands=' + encodeURIComponent(activeBrands.join(','));
      }
      if (facetFilters.brand) {
        url += '&brand=' + encodeURIComponent(facetFilters.brand);
      }
      const activePriceRanges = getFacetArray('price');
      if (activePriceRanges.length > 0) {
        url += '&price_ranges=' + encodeURIComponent(activePriceRanges.join(','));
      }
      const pb = priceBounds(facetFilters.price);
      if (pb) {
        if (pb.min > 0) url += '&min_price=' + pb.min;
        if (pb.max > 0) url += '&max_price=' + pb.max;
      }
      const activeSizes = getFacetArray('size');
      if (activeSizes.length > 0) {
        url += '&sizes=' + encodeURIComponent(activeSizes.join(','));
      }
      if (size) {
        url += '&size=' + encodeURIComponent(size);
      }
      Object.keys(facetFilters.attrs || {}).forEach(function(attrName) {
        const vals = facetFilters.attrs[attrName];
        if (Array.isArray(vals) && vals.length > 0) {
          url += '&attr.' + encodeURIComponent(attrName) + '=' + encodeURIComponent(vals.join(','));
        }
      });
      if (onlyInStock) {
        url += '&only_in_stock=true';
      }
      if (tenantId) {
        url += '&tenant_id=' + encodeURIComponent(tenantId);
      }
      if (locale) {
        url += '&locale=' + encodeURIComponent(locale);
      }
      if (cep) {
        url += '&cep=' + encodeURIComponent(cep);
      }
      if (maxSla24h) {
        url += '&max_sla_hours=24';
      }
      if (ltr) {
        url += '&ltr=true';
      }
      if (parseIntent) {
        url += '&parse_intent=true';
      } else {
        url += '&parse_intent=false';
      }
      const res = queryImage
        ? await fetch('/api/search', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(imageSearchBody(q, url)) })
        : await fetch(url);
      const data = await res.json();
      if (!res.ok) {
        document.getElementById('meta').textContent = 'Error: ' + (data.error || ('Request failed with status ' + res.status));
        document.getElementById('results').innerHTML = '';
        const banner = document.getElementById('recovery-banner');
        if (banner) banner.style.display = 'none';
        return;
      }
      lastQueryId = data.query_id || data.impression_id || '';
      lastExperimentId = data.experiment_id || experimentId || '';
      document.getElementById('backend-badge').textContent = 'Backend: ' + (data.backend_mode || 'spanner');
      if (document.getElementById('retrieval-badge')) {
        document.getElementById('retrieval-badge').textContent = 'Mode: ' + (data.retrieval_mode || 'hybrid') +
          (data.image_query ? ' · photo w=' + data.image_weight + (data.image_cache_hit ? ' (cached)' : '') : '');
      }
      if (document.getElementById('ann-badge')) {
        if (data.vector_index_used !== undefined) {
          document.getElementById('ann-badge').textContent = data.vector_index_used ? 'ANN: ScaNN' : 'ANN: exact';
          document.getElementById('ann-badge').style.display = 'inline-block';
        } else {
          document.getElementById('ann-badge').style.display = 'none';
        }
      }
      if (document.getElementById('tenant-badge')) {
        if (data.tenant_id) {
          document.getElementById('tenant-badge').textContent = 'Tenant: ' + data.tenant_id;
          document.getElementById('tenant-badge').style.display = 'inline-block';
        } else {
          document.getElementById('tenant-badge').style.display = 'none';
        }
      }
      if (document.getElementById('locale-badge')) {
        if (data.locale) {
          document.getElementById('locale-badge').textContent = 'Locale: ' + data.locale;
          document.getElementById('locale-badge').style.display = 'inline-block';
        } else {
          document.getElementById('locale-badge').style.display = 'none';
        }
      }
      if (document.getElementById('ltr-badge')) {
        if (data.ltr) {
          document.getElementById('ltr-badge').textContent = 'LTR: ' + (data.ltr_model || 'active') + (data.ltr_latency_us !== undefined ? ' (' + data.ltr_latency_us + 'µs)' : '');
          document.getElementById('ltr-badge').style.display = 'inline-block';
        } else {
          document.getElementById('ltr-badge').style.display = 'none';
        }
      }
      if (document.getElementById('intent-badge')) {
        if (data.parsed_query && data.parsed_query.intent_source && data.parsed_query.intent_source !== 'none') {
          document.getElementById('intent-badge').textContent = 'Intent: ' + data.parsed_query.intent_source;
          document.getElementById('intent-badge').style.display = 'inline-block';
        } else {
          document.getElementById('intent-badge').style.display = 'none';
        }
      }
      if (document.getElementById('price-table-badge')) {
        if (data.price_table_id) {
          document.getElementById('price-table-badge').textContent = 'Table: ' + data.price_table_id;
          document.getElementById('price-table-badge').style.display = 'inline-block';
        } else {
          document.getElementById('price-table-badge').style.display = 'none';
        }
      }
      if (document.getElementById('sort-badge')) {
        if (data.sort_by) {
          document.getElementById('sort-badge').textContent = 'Sort: ' + data.sort_by;
          document.getElementById('sort-badge').style.display = 'inline-block';
        } else {
          document.getElementById('sort-badge').style.display = 'none';
        }
      }
      if (document.getElementById('personalized-badge')) {
        if (data.personalized) {
          document.getElementById('personalized-badge').textContent = 'Personalizado';
          document.getElementById('personalized-badge').style.display = 'inline-block';
        } else {
          document.getElementById('personalized-badge').style.display = 'none';
        }
      }
      if (document.getElementById('sponsored-badge')) {
        if (data.sponsored_count && data.sponsored_count > 0) {
          document.getElementById('sponsored-badge').textContent = 'Sponsored: ' + data.sponsored_count;
          document.getElementById('sponsored-badge').style.display = 'inline-block';
        } else {
          document.getElementById('sponsored-badge').style.display = 'none';
        }
      }
      if (document.getElementById('experiment-badge')) {
        if (lastExperimentId) {
          document.getElementById('experiment-badge').textContent = 'Exp: ' + lastExperimentId;
          document.getElementById('experiment-badge').style.display = 'inline-block';
        } else {
          document.getElementById('experiment-badge').style.display = 'none';
        }
      }
      if (document.getElementById('query-id-badge')) {
        if (lastQueryId) {
          document.getElementById('query-id-badge').textContent = 'qid: ' + lastQueryId.substring(0, 8);
          document.getElementById('query-id-badge').style.display = 'inline-block';
        } else {
          document.getElementById('query-id-badge').style.display = 'none';
        }
      }
      const facetLat = data.facets_latency_ms !== undefined ? data.facets_latency_ms : (data.facets && data.facets.latency_ms);
      if (facetLat !== undefined && facetLat !== null) {
        document.getElementById('facets-latency').textContent = Number(facetLat).toFixed(1) + 'ms';
      } else {
        document.getElementById('facets-latency').textContent = '';
      }
      const pgInfo = renderPagination(data, pageSize);
      document.getElementById('meta').textContent =
        'Showing ' + pgInfo.start + '-' + pgInfo.end + ' of ' + pgInfo.total + ' results · Found ' + data.total_found + ' products in ' + (data.latency_ms || 0).toFixed(2) + ' ms (effective_alpha=' + (data.effective_alpha !== undefined ? data.effective_alpha : alpha) + ', retrieval_mode=' + (data.retrieval_mode || 'n/a') + (data.vector_index_used !== undefined ? ', vector_index_used=' + data.vector_index_used : '') + (data.tenant_id ? ', tenant=' + data.tenant_id : '') + (data.locale ? ', locale=' + data.locale : '') + (facetLat !== undefined && facetLat !== null ? ', facets=' + Number(facetLat).toFixed(1) + 'ms' : '') + (data.ltr ? ', ltr=' + (data.ltr_model || 'active') + ' (' + (data.ltr_latency_us || 0) + 'µs)' : '') + (data.parsed_query && data.parsed_query.intent_source && data.parsed_query.intent_source !== 'none' ? ', intent=' + data.parsed_query.intent_source : '') + (data.price_table_id ? ', table=' + data.price_table_id : '') + (data.sort_by ? ', sort=' + data.sort_by : '') + (lastExperimentId ? ', experiment=' + lastExperimentId : '') + (lastQueryId ? ', query_id=' + lastQueryId : '') + (data.personalized ? ', personalized=true' : '') + (data.sponsored_count ? ', sponsored_count=' + data.sponsored_count : '') + ')';
      
      const expEl = document.getElementById('expanded-terms');
      if (expEl) {
        if (data.expanded_terms && data.expanded_terms.length > 0) {
          expEl.textContent = 'Also matching: ' + data.expanded_terms.join(', ');
          expEl.style.display = 'block';
        } else {
          expEl.textContent = '';
          expEl.style.display = 'none';
        }
      }

      const banner = document.getElementById('recovery-banner');
      if (banner) {
        while (banner.firstChild) banner.removeChild(banner.firstChild);
        if (data.auto_corrected && data.did_you_mean) {
          banner.style.display = 'block';
          const textNode = document.createTextNode('Mostrando resultados para "' + data.did_you_mean + '". ');
          banner.appendChild(textNode);
          const link = document.createElement('a');
          link.href = '#';
          link.textContent = 'Buscar por "' + (data.query || q) + '"';
          link.style.color = 'var(--accent)';
          link.style.textDecoration = 'underline';
          link.style.cursor = 'pointer';
          link.onclick = function(e) {
            e.preventDefault();
            document.getElementById('q').value = data.query || q;
            const parseCheckbox = document.getElementById('parse_intent');
            if (parseCheckbox) parseCheckbox.checked = false;
            runSearch();
          };
          banner.appendChild(link);
        } else if (!data.auto_corrected && data.did_you_mean) {
          banner.style.display = 'block';
          const textNode = document.createTextNode('Você quis dizer: ');
          banner.appendChild(textNode);
          const link = document.createElement('a');
          link.href = '#';
          link.textContent = data.did_you_mean + '?';
          link.style.color = 'var(--accent)';
          link.style.textDecoration = 'underline';
          link.style.cursor = 'pointer';
          link.onclick = function(e) {
            e.preventDefault();
            document.getElementById('q').value = data.did_you_mean;
            runSearch();
          };
          banner.appendChild(link);
        } else if (data.query_relaxed) {
          banner.style.display = 'block';
          banner.textContent = 'Nenhum resultado com todos os termos — mostrando resultados com qualquer um deles';
        } else {
          banner.style.display = 'none';
        }
      }

      const chipsEl = document.getElementById('parsed-query-chips');
      if (chipsEl) {
        while (chipsEl.firstChild) chipsEl.removeChild(chipsEl.firstChild);
        const pq = data.parsed_query;
        const f = (pq && pq.extracted_filters) || {};
        const hasFilters = Object.keys(f).length > 0;
        if (pq && (hasFilters || pq.clean_query)) {
          chipsEl.style.display = 'flex';
          const items = [];
          if (pq.clean_query) {
            items.push('Query: "' + pq.clean_query + '"');
          }
          if (f.min_price !== undefined && f.max_price !== undefined) {
            items.push('Preço: ' + f.min_price + ' - ' + f.max_price);
          } else if (f.min_price !== undefined) {
            items.push('Preço ≥ ' + f.min_price);
          } else if (f.max_price !== undefined) {
            items.push('Preço ≤ ' + f.max_price);
          }
          if (f.brand) {
            items.push('Marca: ' + f.brand);
          }
          if (f.category) {
            items.push('Categoria: ' + f.category);
          }
          if (f.color) {
            items.push('Cor: ' + f.color);
          }
          if (pq.intent_source && pq.intent_source !== 'none') {
            items.push('(' + pq.intent_source + ')');
          }
          items.forEach(function(text) {
            const chip = document.createElement('span');
            chip.className = 'badge';
            chip.style.background = '#1e293b';
            chip.style.border = '1px solid #38bdf8';
            chip.style.color = '#38bdf8';
            chip.style.fontSize = '12px';
            chip.style.padding = '4px 10px';
            chip.style.borderRadius = '999px';
            chip.textContent = text;
            chipsEl.appendChild(chip);
          });
        } else {
          chipsEl.style.display = 'none';
        }
      }
      
      const facets = data.facets || {};
      renderFacetGroup('facets-cat-list', facets.categories, 'category');
      renderFacetGroup('facets-brand-list', facets.brands, 'brand');
      renderFacetGroup('facets-price-list', facets.price_ranges, 'price');
      renderFacetGroup('facets-color-list', facets.colors, 'color');
      renderFacetGroup('facets-size-list', facets.sizes, 'size');
      renderFacetGroup('facets-avail-list', facets.availability, 'availability');
      renderDynamicAttrFacets(facets.attributes);
      renderActiveFilters();
      const container = document.getElementById('results');
      container.innerHTML = (data.results || []).map(p => {
        const s = p.score || {};
        const isStockIn = p.availability === 'IN_STOCK';
        const stockClass = isStockIn ? 'stock-in' : 'stock-out';
        const stockBadge = isStockIn
          ? (p.availableQuantity !== undefined && p.availableQuantity !== null
              ? 'IN_STOCK (' + p.availableQuantity + ' unid.)'
              : 'IN_STOCK')
          : 'OUT_OF_STOCK';
        const locBadge = p.locale
          ? '<span class="badge" style="background:#1e293b;border:1px solid #38bdf8;color:#38bdf8;font-size:11px;padding:2px 6px;">' + p.locale + '</span>'
          : '';
        const sponsoredBadge = p.sponsored
          ? '<span class="sponsored-badge" data-testid="sponsored-badge">Sponsored · Patrocinado</span>'
          : '';
        const finalScore = s.final_score !== undefined ? s.final_score : (s.hybrid !== undefined ? s.hybrid : 0);
        const vecScore = s.vector !== undefined ? s.vector : 0;
        const txtScore = s.text !== undefined ? s.text : 0;
        const ltrScore = s.ltr_score !== undefined ? ' | ltr=' + s.ltr_score.toFixed(4) : '';
        return '<div class="card' + (p.sponsored ? ' card-sponsored' : '') + '" data-sponsored="' + (p.sponsored ? 'true' : 'false') + '">' +
          '<div style="display:flex;justify-content:space-between;align-items:center;"><div style="display:flex;gap:6px;align-items:center;flex-wrap:wrap;">' + sponsoredBadge + '<span class="' + stockClass + '">' + stockBadge + '</span>' + locBadge + '<span class="rank-badge" style="display:none;"></span><span class="delivery-badge" style="display:none;"></span><span class="boost-badge" style="display:none;"></span></div><span style="color:#94a3b8;font-size:12px;">' + (p.categories || []).join(', ') + '</span></div>' +
          '<h3>' + p.title + '</h3>' +
          '<div style="color:#94a3b8;font-size:13px;">' + (p.description || '') + '</div>' +
          '<div class="price"><span class="price-val">$' + ((p.priceInfo && p.priceInfo.price) || '0.00') + '</span><span class="base-price" style="display:none;text-decoration:line-through;color:var(--muted);font-size:14px;font-weight:normal;margin-left:8px;"></span><span class="contract-badge" style="display:none;margin-left:8px;"></span></div>' +
          '<div class="variant-chips" style="display:none;gap:4px;flex-wrap:wrap;align-items:center;"></div>' +
          '<div class="spec-badges" data-testid="product-spec-badges" style="display:none;gap:4px;flex-wrap:wrap;align-items:center;margin-top:6px;"></div>' +
          '<div class="scores">hybrid=' + finalScore.toFixed(4) + ' | vec=' + vecScore.toFixed(4) + ' | txt=' + txtScore.toFixed(4) + ltrScore + '</div>' +
          '<div style="margin-top:10px;display:flex;justify-content:space-between;align-items:center;gap:6px;flex-wrap:wrap;">' +
            '<div style="display:flex;gap:6px;flex-wrap:wrap;">' +
              '<button type="button" class="similar-btn" data-testid="open-similar-btn" style="background:#0f172a;border:1px solid #38bdf8;color:#38bdf8;padding:3px 8px;border-radius:4px;cursor:pointer;font-size:11px;">Similar / Co-Viewed</button>' +
              (!isStockIn ? '<button type="button" class="substitutes-btn" data-testid="see-substitutes-btn" style="background:#451a03;border:1px solid #f59e0b;color:#fde68a;padding:3px 8px;border-radius:4px;cursor:pointer;font-size:11px;font-weight:600;">See In-Stock Alternatives</button>' : '') +
            '</div>' +
            '<div style="display:flex;gap:6px;align-items:center;">' +
              '<button type="button" class="add-to-cart-btn" data-testid="add-to-cart-btn" style="background:#064e3b;border:1px solid #10b981;color:#d1fae5;padding:3px 8px;border-radius:4px;cursor:pointer;font-size:11px;font-weight:600;">+ Cart</button>' +
              '<button type="button" class="record-click-btn" style="background:#1e293b;border:1px solid #64748b;color:#f8fafc;padding:3px 8px;border-radius:4px;cursor:pointer;font-size:11px;">+ Clique</button>' +
            '</div>' +
          '</div>' +
          '<div class="substitutes-panel" data-testid="substitutes-panel" style="display:none;margin-top:8px;padding:8px;background:#0f172a;border:1px solid #f59e0b;border-radius:6px;"></div>' +
        '</div>';
      }).join('');
      const baseOffset = typeof data.offset === 'number' && data.offset >= 0 ? data.offset : (currentPage - 1) * pageSize;
      (data.results || []).forEach((p, i) => {
        const card = container.children[i];
        if (!card) return;
        const rankPos = baseOffset + i + 1;
        const priceVal = parseFloat((p.priceInfo && p.priceInfo.price) || '0') || 0;
        card.setAttribute('data-position', String(rankPos));
        if (p.sponsored && p.ad_campaign_id && p.ad_click_id) {
          card.style.cursor = 'pointer';
          card.onclick = function() {
            recordAdClick(p.ad_campaign_id, p.ad_click_id);
          };
        }
        const s = p.score || {};
        if (s.stage1_rank !== undefined && s.stage2_rank !== undefined) {
          const rankEl = card.querySelector('.rank-badge');
          if (rankEl) {
            const diff = s.stage1_rank - s.stage2_rank;
            let rankText = s.stage1_rank + '→' + s.stage2_rank;
            if (diff > 0) {
              rankText += ' (↑' + diff + ')';
            } else if (diff < 0) {
              rankText += ' (↓' + Math.abs(diff) + ')';
            }
            rankEl.textContent = rankText;
            rankEl.style.display = 'inline-block';
            rankEl.style.background = '#1e293b';
            rankEl.style.border = '1px solid #a855f7';
            rankEl.style.color = '#d8b4fe';
            rankEl.style.fontSize = '11px';
            rankEl.style.padding = '2px 6px';
            rankEl.style.borderRadius = '4px';
          }
        }
        if (p.deliveryPromise && p.deliveryPromise.label) {
          const badge = card.querySelector('.delivery-badge');
          if (badge) {
            let labelText = p.deliveryPromise.label;
            if (p.deliveryPromise.free_shipping) {
              labelText += ' · Frete grátis';
            }
            badge.textContent = labelText;
            badge.style.display = 'inline-block';
            badge.style.background = '#064e3b';
            badge.style.border = '1px solid #10b981';
            badge.style.color = '#6ee7b7';
            badge.style.fontSize = '11px';
            badge.style.padding = '2px 6px';
            badge.style.borderRadius = '4px';
          }
        }
        if (p.priceInfo && p.priceInfo.priceTableId) {
          const baseEl = card.querySelector('.base-price');
          if (baseEl) {
            const baseVal = p.priceInfo.basePrice !== undefined && p.priceInfo.basePrice !== null
              ? Number(p.priceInfo.basePrice).toFixed(2)
              : p.priceInfo.originalPrice;
            if (baseVal) {
              baseEl.textContent = '$' + baseVal;
              baseEl.style.display = 'inline';
            }
          }
          const contractEl = card.querySelector('.contract-badge');
          if (contractEl) {
            contractEl.textContent = 'Contrato: ' + p.priceInfo.priceTableId;
            contractEl.style.display = 'inline-block';
            contractEl.style.background = '#1e293b';
            contractEl.style.border = '1px solid #38bdf8';
            contractEl.style.color = '#38bdf8';
            contractEl.style.fontSize = '11px';
            contractEl.style.padding = '2px 6px';
            contractEl.style.borderRadius = '4px';
          }
        }
        if (data.personalized && s.session_boost !== undefined) {
          const boostEl = card.querySelector('.boost-badge');
          if (boostEl) {
            boostEl.textContent = 'boost: +' + Number(s.session_boost).toFixed(4);
            boostEl.style.display = 'inline-block';
            boostEl.style.background = '#3b0764';
            boostEl.style.border = '1px solid #a855f7';
            boostEl.style.color = '#e9d5ff';
            boostEl.style.fontSize = '11px';
            boostEl.style.padding = '2px 6px';
            boostEl.style.borderRadius = '4px';
          }
        }
        const varContainer = card.querySelector('.variant-chips');
        if (varContainer) {
          const availColors = Array.isArray(p.available_colors) ? p.available_colors : [];
          const availSizes = Array.isArray(p.available_sizes) ? p.available_sizes : [];
          const varCount = typeof p.variant_count === 'number' ? p.variant_count : (Array.isArray(p.variants) ? p.variants.length : 0);
          const shownCount = availColors.length + availSizes.length;
          const extraVars = varCount > shownCount ? varCount - shownCount : 0;
          if (availColors.length > 0 || availSizes.length > 0 || extraVars > 0) {
            varContainer.style.display = 'flex';
            availColors.forEach(function(c) {
              const chip = document.createElement('span');
              chip.className = 'badge';
              chip.style.background = '#0f172a';
              chip.style.border = '1px solid #38bdf8';
              chip.style.color = '#e0f2fe';
              chip.style.fontSize = '11px';
              chip.style.padding = '2px 6px';
              chip.style.borderRadius = '4px';
              chip.textContent = c;
              varContainer.appendChild(chip);
            });
            availSizes.forEach(function(sz) {
              const chip = document.createElement('span');
              chip.className = 'badge';
              chip.style.background = '#0f172a';
              chip.style.border = '1px solid #64748b';
              chip.style.color = '#cbd5e1';
              chip.style.fontSize = '11px';
              chip.style.padding = '2px 6px';
              chip.style.borderRadius = '4px';
              chip.textContent = sz;
              varContainer.appendChild(chip);
            });
            if (extraVars > 0) {
              const moreChip = document.createElement('span');
              moreChip.className = 'badge';
              moreChip.style.background = '#1e293b';
              moreChip.style.border = '1px solid #38bdf8';
              moreChip.style.color = '#38bdf8';
              moreChip.style.fontSize = '11px';
              moreChip.style.padding = '2px 6px';
              moreChip.style.borderRadius = '4px';
              moreChip.textContent = '+' + extraVars + ' variantes';
              varContainer.appendChild(moreChip);
            }
          }
        }
        const specContainer = card.querySelector('.spec-badges');
        if (specContainer) {
          const topSpecs = extractCardSpecs(p).slice(0, 3);
          if (topSpecs.length > 0) {
            specContainer.style.display = 'flex';
            topSpecs.forEach(function(spec) {
              const badge = document.createElement('span');
              badge.className = 'badge spec-badge';
              badge.setAttribute('data-testid', 'spec-badge');
              badge.style.background = '#0f172a';
              badge.style.border = '1px solid #475569';
              badge.style.color = '#e2e8f0';
              badge.style.fontSize = '11px';
              badge.style.padding = '2px 6px';
              badge.style.borderRadius = '4px';
              badge.textContent = spec.label + ': ' + spec.value;
              specContainer.appendChild(badge);
            });
          }
        }
        const similarBtn = card.querySelector('.similar-btn');
        if (similarBtn) {
          similarBtn.onclick = function(e) {
            e.stopPropagation();
            openProductDetailsModal(p);
          };
        }
        const subBtn = card.querySelector('.substitutes-btn');
        const subPanel = card.querySelector('.substitutes-panel');
        if (subBtn && subPanel) {
          subBtn.onclick = function(e) {
            e.stopPropagation();
            loadInStockSubstitutes(p.id, subPanel, subBtn);
          };
        }
        const cartBtn = card.querySelector('.add-to-cart-btn');
        if (cartBtn) {
          cartBtn.onclick = async function(e) {
            e.stopPropagation();
            cartBtn.disabled = true;
            cartBtn.textContent = 'Added!';
            recordAnalyticsEvent('add_to_cart', p.id, rankPos, priceVal);
            try {
              const curSid = (document.getElementById('session_id') && document.getElementById('session_id').value.trim()) || 'sess-default';
              const curTenant = (document.getElementById('tenant_id') && document.getElementById('tenant_id').value.trim()) || 'default';
              await fetch('/api/session/events', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ session_id: curSid, product_id: p.id, event_type: 'add_to_cart', tenant_id: curTenant })
              });
            } catch (err) {
              // ignore
            }
            setTimeout(function() {
              cartBtn.disabled = false;
              cartBtn.textContent = '+ Cart';
            }, 900);
          };
        }
        const recordBtn = card.querySelector('.record-click-btn');
        if (recordBtn) {
          recordBtn.textContent = '+ Clique';
          recordBtn.onclick = async function(e) {
            e.stopPropagation();
            if (p.sponsored && p.ad_campaign_id && p.ad_click_id) {
              recordAdClick(p.ad_campaign_id, p.ad_click_id);
            }
            recordAnalyticsEvent('click', p.id, rankPos, priceVal);
            recordBtn.disabled = true;
            recordBtn.textContent = 'Gravando...';
            try {
              const curSid = (document.getElementById('session_id') && document.getElementById('session_id').value.trim()) || 'sess-default';
              const curTenant = (document.getElementById('tenant_id') && document.getElementById('tenant_id').value.trim()) || 'default';
              await fetch('/api/session/events', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ session_id: curSid, product_id: p.id, event_type: 'click', tenant_id: curTenant })
              });
              runSearch();
            } catch (err) {
              console.error(err);
              recordBtn.textContent = 'Erro';
              recordBtn.disabled = false;
            }
          };
        }
      });
    }
    function recordAnalyticsEvent(eventType, productId, position, priceVal) {
      if (!productId) return;
      const curSid = (document.getElementById('session_id') && document.getElementById('session_id').value.trim()) || 'sess-default';
      const curTenant = (document.getElementById('tenant_id') && document.getElementById('tenant_id').value.trim()) || 'default';
      const expSelect = document.getElementById('experiment_id');
      const expId = lastExperimentId || (expSelect ? expSelect.value : '');
      const revMicros = (eventType === 'add_to_cart' || eventType === 'purchase') && priceVal > 0 ? Math.round(priceVal * 1000000) : 0;
      const payload = {
        event_id: 'evt-' + Date.now() + '-' + Math.random().toString(16).substring(2, 10),
        query_id: lastQueryId || '',
        session_id: curSid,
        event_type: eventType || 'click',
        product_id: productId,
        position: typeof position === 'number' && position > 0 ? position : 1,
        experiment_id: expId,
        revenue_micros: revMicros,
        tenant_id: curTenant
      };
      fetch('/api/analytics/events', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
        keepalive: true
      }).catch(function() {});
    }
    function toggleAnalyticsDrawer(forceState) {
      const drawer = document.getElementById('analytics-drawer');
      if (!drawer) return;
      const willOpen = typeof forceState === 'boolean' ? forceState : (drawer.style.display === 'none' || !drawer.style.display);
      drawer.style.display = willOpen ? 'block' : 'none';
      if (willOpen) {
        loadAnalyticsSummary();
      }
    }
    async function loadAnalyticsSummary() {
      const curTenant = (document.getElementById('tenant_id') && document.getElementById('tenant_id').value.trim()) || 'default';
      try {
        const res = await fetch('/api/analytics/summary?tenant_id=' + encodeURIComponent(curTenant) + '&window_hours=168');
        if (!res.ok) return;
        const data = await res.json();
        const ctrEl = document.getElementById('analytics-ctr');
        if (ctrEl) ctrEl.textContent = (((Number(data.click_through_rate) || 0) * 100).toFixed(1)) + '%';
        const subEl = document.getElementById('analytics-searches-sub');
        if (subEl) subEl.textContent = (data.total_clicks || 0) + ' clicks / ' + (data.total_searches || 0) + ' searches';
        const mrrEl = document.getElementById('analytics-mrr10');
        if (mrrEl) mrrEl.textContent = Number(data.mrr_at_10 || 0).toFixed(3);
        const zeroRateEl = document.getElementById('analytics-zero-rate');
        if (zeroRateEl) zeroRateEl.textContent = (((Number(data.zero_result_rate) || 0) * 100).toFixed(1)) + '%';
        const zeroSubEl = document.getElementById('analytics-zero-sub');
        if (zeroSubEl) zeroSubEl.textContent = (data.zero_result_searches || 0) + ' zero-result queries';
        const convRateEl = document.getElementById('analytics-conv-rate');
        if (convRateEl) convRateEl.textContent = (((Number(data.conversion_rate) || 0) * 100).toFixed(1)) + '%';
        const convSubEl = document.getElementById('analytics-conv-sub');
        if (convSubEl) convSubEl.textContent = (data.total_conversions || 0) + ' conversions';

        const zqEl = document.getElementById('analytics-zero-queries');
        if (zqEl) {
          zqEl.textContent = '';
          const zList = Array.isArray(data.top_zero_result_queries) ? data.top_zero_result_queries : [];
          if (zList.length === 0) {
            zqEl.textContent = 'No zero-result queries recorded.';
          } else {
            zList.forEach(function(item) {
              const row = document.createElement('div');
              row.setAttribute('data-testid', 'zero-result-query-item');
              row.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:4px 6px;background:#1e293b;border-radius:4px;';
              const qSpan = document.createElement('span');
              qSpan.style.cssText = 'color:#f8fafc;font-weight:600;';
              qSpan.textContent = '"' + (item.query || '') + '"';
              const cSpan = document.createElement('span');
              cSpan.className = 'facet-count';
              cSpan.textContent = String(item.count || 0);
              row.appendChild(qSpan);
              row.appendChild(cSpan);
              zqEl.appendChild(row);
            });
          }
        }

        const expEl = document.getElementById('analytics-experiments');
        if (expEl) {
          expEl.textContent = '';
          const expList = Array.isArray(data.experiments) ? data.experiments : [];
          if (expList.length === 0) {
            expEl.textContent = 'No experiment cohorts recorded yet.';
          } else {
            expList.forEach(function(exp) {
              const row = document.createElement('div');
              row.setAttribute('data-testid', 'experiment-breakdown-row');
              row.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:6px 8px;background:#1e293b;border:1px solid #334155;border-radius:6px;flex-wrap:wrap;gap:6px;';
              const nameSpan = document.createElement('span');
              nameSpan.className = 'badge';
              nameSpan.textContent = exp.experiment_id || 'default';
              const metricsSpan = document.createElement('span');
              metricsSpan.style.cssText = 'color:#e2e8f0;font-size:11px;';
              metricsSpan.textContent = 'searches=' + (exp.searches || 0) + ' · clicks=' + (exp.clicks || 0) + ' · CTR=' + (((Number(exp.ctr) || 0) * 100).toFixed(1)) + '% · MRR@10=' + Number(exp.mrr_at_10 || 0).toFixed(3);
              row.appendChild(nameSpan);
              row.appendChild(metricsSpan);
              expEl.appendChild(row);
            });
          }
        }
      } catch (err) {
        // ignore summary fetch error
      }
    }
    function buildSimilarUrl(productId, mode, limit) {
      const tenantId = (document.getElementById('tenant_id') && document.getElementById('tenant_id').value) || 'default';
      const locale = (document.getElementById('locale') && document.getElementById('locale').value) || '';
      const channel = (document.getElementById('channel') && document.getElementById('channel').value) || '';
      const priceTableId = (document.getElementById('price_table_id') && document.getElementById('price_table_id').value) || '';
      const cep = (document.getElementById('cep') && document.getElementById('cep').value.trim()) || '';
      let u = '/api/products/' + encodeURIComponent(productId) + '/similar?mode=' + encodeURIComponent(mode) + '&limit=' + encodeURIComponent(limit) + '&tenant_id=' + encodeURIComponent(tenantId);
      if (mode === 'substitutes') u += '&only_in_stock=true';
      if (locale) u += '&locale=' + encodeURIComponent(locale);
      if (channel) u += '&channel=' + encodeURIComponent(channel);
      if (priceTableId) u += '&price_table_id=' + encodeURIComponent(priceTableId);
      if (cep) u += '&cep=' + encodeURIComponent(cep);
      return u;
    }
    function renderRecCarousel(containerId, items) {
      const el = document.getElementById(containerId);
      if (!el) return;
      el.textContent = '';
      if (!Array.isArray(items) || items.length === 0) {
        const empty = document.createElement('span');
        empty.style.cssText = 'color:var(--muted);font-size:12px;';
        empty.textContent = 'No recommendations available.';
        el.appendChild(empty);
        return;
      }
      items.forEach(function(item) {
        const card = document.createElement('div');
        card.style.cssText = 'min-width:190px;max-width:210px;flex-shrink:0;background:#0f172a;border:1px solid #334155;border-radius:8px;padding:10px;cursor:pointer;';
        const title = document.createElement('div');
        title.style.cssText = 'font-weight:700;font-size:13px;color:var(--text);margin-bottom:4px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;';
        title.textContent = item.title || item.name || item.id;
        const cat = document.createElement('div');
        cat.style.cssText = 'font-size:11px;color:var(--muted);margin-bottom:6px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;';
        cat.textContent = Array.isArray(item.categories) ? item.categories.join(', ') : (item.category || '');
        const bottom = document.createElement('div');
        bottom.style.cssText = 'display:flex;justify-content:space-between;align-items:center;';
        const price = document.createElement('span');
        price.style.cssText = 'font-weight:700;font-size:13px;color:#22c55e;';
        price.textContent = '$' + ((item.priceInfo && item.priceInfo.price) || '0.00');
        const stock = document.createElement('span');
        const inStock = item.availability === 'IN_STOCK';
        stock.className = inStock ? 'stock-in' : 'stock-out';
        stock.textContent = inStock ? 'IN_STOCK' : 'OUT_OF_STOCK';
        bottom.appendChild(price);
        bottom.appendChild(stock);
        card.appendChild(title);
        card.appendChild(cat);
        card.appendChild(bottom);
        if (item.similarity_score !== undefined) {
          const meta = document.createElement('div');
          meta.style.cssText = 'font-size:10px;color:#94a3b8;margin-top:4px;';
          meta.textContent = 'sim=' + Number(item.similarity_score).toFixed(2) + (item.co_view_count ? ' · co-views=' + item.co_view_count : '');
          card.appendChild(meta);
        }
        card.onclick = function() {
          openProductDetailsModal(item);
        };
        el.appendChild(card);
      });
    }
    function switchRecTab(tab) {
      const secSim = document.getElementById('section-similar');
      const secCo = document.getElementById('section-co-viewed');
      if (secSim) secSim.style.display = (tab === 'all' || tab === 'similar') ? 'block' : 'none';
      if (secCo) secCo.style.display = (tab === 'all' || tab === 'co_viewed') ? 'block' : 'none';
      const btnAll = document.getElementById('tab-similar-btn');
      const btnSim = document.getElementById('tab-only-similar-btn');
      const btnCo = document.getElementById('tab-only-coviewed-btn');
      if (btnAll) btnAll.className = 'page-btn' + (tab === 'all' ? ' page-btn-active' : '');
      if (btnSim) btnSim.className = 'page-btn' + (tab === 'similar' ? ' page-btn-active' : '');
      if (btnCo) btnCo.className = 'page-btn' + (tab === 'co_viewed' ? ' page-btn-active' : '');
    }
    async function openProductDetailsModal(p) {
      if (!p || !p.id) return;
      const modal = document.getElementById('product-details-modal');
      if (!modal) return;
      modal.style.display = 'block';
      const titleEl = document.getElementById('modal-product-title');
      if (titleEl) titleEl.textContent = (p.title || p.name || p.id) + ' (' + p.id + ')';
      switchRecTab('all');
      const simEl = document.getElementById('carousel-similar');
      const coEl = document.getElementById('carousel-co-viewed');
      if (simEl) simEl.textContent = 'Loading similar products...';
      if (coEl) coEl.textContent = 'Loading frequently viewed together...';
      try {
        const [simRes, coRes] = await Promise.all([
          fetch(buildSimilarUrl(p.id, 'similar', 6)),
          fetch(buildSimilarUrl(p.id, 'co_viewed', 6))
        ]);
        const simData = simRes.ok ? await simRes.json() : { results: [] };
        const coData = coRes.ok ? await coRes.json() : { results: [] };
        renderRecCarousel('carousel-similar', simData.results || []);
        renderRecCarousel('carousel-co-viewed', coData.results || []);
      } catch (err) {
        if (simEl) simEl.textContent = 'Failed to load similar products.';
        if (coEl) coEl.textContent = 'Failed to load co-viewed products.';
      }
    }
    function closeProductDetailsModal() {
      const modal = document.getElementById('product-details-modal');
      if (modal) modal.style.display = 'none';
    }
    async function loadInStockSubstitutes(productId, panelEl, btnEl) {
      if (!productId || !panelEl) return;
      if (panelEl.style.display === 'block') {
        panelEl.style.display = 'none';
        if (btnEl) btnEl.textContent = 'See In-Stock Alternatives';
        return;
      }
      panelEl.style.display = 'block';
      panelEl.textContent = 'Loading in-stock substitutes...';
      if (btnEl) btnEl.textContent = 'Hide In-Stock Alternatives';
      try {
        const res = await fetch(buildSimilarUrl(productId, 'substitutes', 4));
        const data = await res.json();
        panelEl.textContent = '';
        const items = (data && Array.isArray(data.results)) ? data.results : [];
        if (items.length === 0) {
          const none = document.createElement('span');
          none.style.cssText = 'color:var(--muted);font-size:11px;';
          none.textContent = 'No in-stock substitutes found.';
          panelEl.appendChild(none);
          return;
        }
        items.forEach(function(sub) {
          const row = document.createElement('div');
          row.setAttribute('data-testid', 'substitute-item');
          row.style.cssText = 'display:flex;justify-content:space-between;align-items:center;padding:4px 0;border-bottom:1px solid #334155;font-size:12px;cursor:pointer;';
          const left = document.createElement('span');
          left.style.cssText = 'color:#f8fafc;font-weight:600;';
          left.textContent = (sub.title || sub.name || sub.id) + (sub.substitute_reason ? ' (' + sub.substitute_reason + ')' : '');
          const right = document.createElement('span');
          right.style.cssText = 'color:#22c55e;font-weight:700;';
          right.textContent = '$' + ((sub.priceInfo && sub.priceInfo.price) || '0.00') + ' · IN_STOCK';
          row.appendChild(left);
          row.appendChild(right);
          row.onclick = function(e) {
            e.stopPropagation();
            openProductDetailsModal(sub);
          };
          panelEl.appendChild(row);
        });
      } catch (err) {
        panelEl.textContent = 'Error loading substitutes.';
      }
    }
    function recordAdClick(campaignId, clickId) {
      if (!campaignId || !clickId) return;
      fetch('/api/ads/click', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ campaign_id: campaignId, click_id: clickId }),
        keepalive: true
      }).catch(function() {});
    }
    const qInput = document.getElementById('q');
    const qDatalist = document.getElementById('q-suggestions');
    let autoTimer = null;
    if (qInput && qDatalist) {
      qInput.addEventListener('input', function() {
        if (autoTimer) clearTimeout(autoTimer);
        const val = qInput.value.trim();
        if (val.length < 2) {
          while (qDatalist.firstChild) qDatalist.removeChild(qDatalist.firstChild);
          return;
        }
        autoTimer = setTimeout(async function() {
          const tenantId = document.getElementById('tenant_id') ? document.getElementById('tenant_id').value : 'default';
          const locale = document.getElementById('locale') ? document.getElementById('locale').value : '';
          let autoUrl = '/api/autocomplete?q=' + encodeURIComponent(val) + '&limit=8';
          if (tenantId) autoUrl += '&tenant_id=' + encodeURIComponent(tenantId);
          if (locale) autoUrl += '&locale=' + encodeURIComponent(locale);
          try {
            const res = await fetch(autoUrl);
            if (!res.ok) return;
            const data = await res.json();
            while (qDatalist.firstChild) qDatalist.removeChild(qDatalist.firstChild);
            (data.suggestions || []).forEach(function(s) {
              const opt = document.createElement('option');
              opt.value = typeof s === 'object' ? s.text : s;
              qDatalist.appendChild(opt);
            });
          } catch (e) {
            // ignore
          }
        }, 150);
      });
    }
    function showAllCatalog() {
      // Browse the whole catalog across locales (empty query = catalog browse, #30).
      document.getElementById('q').value = '';
      facetFilters = {};
      const stockEl = document.getElementById('only_in_stock');
      if (stockEl) stockEl.checked = false;
      queryImage = null;
      document.getElementById('image-chip').style.display = 'none';
      const loc = document.getElementById('locale');
      if (loc) loc.value = '';
      const colEl = document.getElementById('color');
      if (colEl) colEl.value = '';
      const szEl = document.getElementById('size');
      if (szEl) szEl.value = '';
      runSearch({ browseAll: true });
    }
    const sidEl = document.getElementById('session_id');
    if (sidEl && !sidEl.value) {
      let storedSid = sessionStorage.getItem('psearch_storefront_session_id');
      if (!storedSid) {
        storedSid = 'sess-' + Math.random().toString(16).substring(2, 10) + Math.random().toString(16).substring(2, 10);
        sessionStorage.setItem('psearch_storefront_session_id', storedSid);
      }
      sidEl.value = storedSid;
    }
    runSearch();
  </script>
</body>
</html>`
