package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                     int
	Environment              string
	ProjectID                string
	Region                   string
	SpannerInstanceID        string
	SpannerDatabaseID        string
	GenAILocation            string // Vertex AI location for Gen AI SDK calls (Gemini 3.x / gemini-embedding-2 => "global")
	EmbeddingModelName       string // Gemini embedding model used for the v2 vector column
	GeminiEmbeddingDimension int    // output_dimensionality requested from the embedding model (ADR-001: 1024)
	EmbeddingDimension       int    // dimension of the ACTIVE vector column (v1=768 legacy, v2=GeminiEmbeddingDimension)
	ActiveEmbeddingVersion   string // initial version from env; use ActiveVersion() at runtime
	LTRModelJSON             string // optional learned LTR model (BigQuery ML export); empty = baseline
	IntentLLMFallback        bool   // ADR-007: Gemini Flash Lite fallback for unparsed conversational queries
	IntentLLMModel           string // Issue #23: text/intent model ("gemini-3.8-flash")
	ProLLMModel              string // Issue #23: pro reasoning model ("gemini-4-argon")
	SessionBoostWeight       float64 // ADR-007 (#20): weight of the session centroid similarity boost
	BoostWeight              float64 // scale of the 8-signal boost added to RRF (ADR-004); 0 disables
	TextFolding              bool    // use accent-folded tokenlists (migration 014b) so "tenis" matches "Tênis" (Issue #4)
	AdminToken               string  // guards POST /api/config/embedding-version (empty = disabled)
	activeVersion            atomic.Pointer[string]
	DefaultAlpha             float64
	DefaultLimit             int
	MinScoreValue            float64
	UseMemoryStore           bool
	StaticDir                string
	AdSlots                  []int   // ADR-011 (#35): 1-based slot positions for sponsored products (default [1,4,8])
	AdMax                    int     // ADR-011 (#35): max sponsored products per response (default 3)
	AdMinSimilarity          float64 // ADR-011 (#35): minimum cosine similarity for vector ad match (default 0.55)
	UseVectorIndex           bool          // ADR-012 (#39): use ScaNN VECTOR INDEX (APPROX_COSINE_DISTANCE) when v2 is active
	useVectorIndexExplicit   bool          // true when USE_VECTOR_INDEX was explicitly set in env
	ANNNumLeavesToSearch     int           // ADR-012 (#39): ScaNN num_leaves_to_search option (default 20)
	SearchTimeout            time.Duration // Issue #66: per-search deadline (default 10s, must stay < 15s WriteTimeout)
	SearchMaxInflight        int           // Issue #66: per-instance in-flight search semaphore capacity (default 8)
	SearchQueueWait          time.Duration // Issue #66: max wait for an in-flight search slot before 503 (default 250ms)
	SearchQueueWaitSet       bool          // true when SearchQueueWait was explicitly configured (allows 0ms wait)
}

// NormalizeModelID normalizes model aliases (e.g. gemini-4-pro / gemini-4.0-pro -> gemini-4-argon).
func NormalizeModelID(model string) string {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "gemini-4-pro", "gemini-4.0-pro":
		return "gemini-4-argon"
	default:
		return model
	}
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                     8080,
		Environment:              getEnv("ENVIRONMENT", "production"),
		ProjectID:                getEnv("PROJECT_ID", getEnv("GCP_SANDBOX_PROJECT", getEnv("CLOUDSDK_CORE_PROJECT", ""))),
		Region:                   getEnv("REGION", "us-central1"),
		SpannerInstanceID:        getEnv("SPANNER_INSTANCE_ID", "psearch-instance"),
		SpannerDatabaseID:        getEnv("SPANNER_DATABASE_ID", "psearch-db"),
		GenAILocation:            getEnv("GENAI_LOCATION", "global"),
		EmbeddingModelName:       getEnv("EMBEDDING_MODEL_NAME", getEnv("GEMINI_MODEL_NAME", "gemini-embedding-2")),
		GeminiEmbeddingDimension: 1024,
		ActiveEmbeddingVersion:   getEnv("ACTIVE_EMBEDDING_VERSION", "v1"),
		AdminToken:               os.Getenv("ADMIN_TOKEN"),
		TextFolding:              getEnv("TEXT_FOLDING", "false") == "true",
		BoostWeight:              0.002,
		SessionBoostWeight:       0.003,
		IntentLLMFallback:        getEnv("INTENT_LLM_FALLBACK", "false") == "true",
		IntentLLMModel:           NormalizeModelID(getEnv("GEMINI_TEXT_MODEL", getEnv("GEMINI_LITE_MODEL", "gemini-3.8-flash"))),
		ProLLMModel:              NormalizeModelID(getEnv("GEMINI_PRO_MODEL", "gemini-4-argon")),
		LTRModelJSON:             os.Getenv("LTR_MODEL_JSON"),
		DefaultAlpha:             0.5,
		DefaultLimit:             20,
		MinScoreValue:            0.0,
		UseMemoryStore:           getEnv("USE_MEMORY_STORE", "false") == "true",
		StaticDir:                getEnv("STATIC_DIR", "/app/static"),
		AdSlots:                  ParseAdSlots(getEnv("AD_SLOTS", "1,4,8")),
		AdMax:                    3,
		AdMinSimilarity:          0.55,
		ANNNumLeavesToSearch:     20,
		SearchTimeout:            10 * time.Second,
		SearchMaxInflight:        8,
		SearchQueueWait:          250 * time.Millisecond,
	}

	if d, ok := parseEnvDuration("SEARCH_TIMEOUT"); ok && d > 0 && d <= 14*time.Second {
		cfg.SearchTimeout = d
	}
	if maxInf, err := strconv.Atoi(getEnv("SEARCH_MAX_INFLIGHT", "8")); err == nil && maxInf > 0 && maxInf <= 256 {
		cfg.SearchMaxInflight = maxInf
	}
	if d, ok := parseEnvDuration("SEARCH_QUEUE_WAIT"); ok && d >= 0 && d <= 5*time.Second {
		cfg.SearchQueueWait = d
		cfg.SearchQueueWaitSet = true
	}

	rawVecIdx := strings.ToLower(strings.TrimSpace(os.Getenv("USE_VECTOR_INDEX")))
	if rawVecIdx == "" {
		cfg.UseVectorIndex = cfg.ActiveEmbeddingVersion == "v2"
	} else {
		cfg.useVectorIndexExplicit = true
		cfg.UseVectorIndex = rawVecIdx == "true" || rawVecIdx == "1" || rawVecIdx == "yes" || rawVecIdx == "on"
	}
	if leaves, err := strconv.Atoi(getEnv("ANN_NUM_LEAVES_TO_SEARCH", "20")); err == nil && leaves > 0 && leaves <= 1000 {
		cfg.ANNNumLeavesToSearch = leaves
	}

	if adMax, err := strconv.Atoi(getEnv("AD_MAX", "3")); err == nil && adMax > 0 && adMax <= 10 {
		cfg.AdMax = adMax
	} else if len(cfg.AdSlots) > 0 {
		cfg.AdMax = len(cfg.AdSlots)
	}
	if adSim, err := strconv.ParseFloat(getEnv("AD_MIN_SIMILARITY", "0.55"), 64); err == nil && adSim > 0 && adSim <= 1.0 {
		cfg.AdMinSimilarity = adSim
	}

	if port, err := strconv.Atoi(getEnv("PORT", "8080")); err == nil {
		cfg.Port = port
	}
	if dim, err := strconv.Atoi(getEnv("EMBEDDING_DIMENSION", "1024")); err == nil && dim > 0 {
		cfg.GeminiEmbeddingDimension = dim
	}
	cfg.EmbeddingDimension = cfg.ActiveDimension()
	if w, err := strconv.ParseFloat(getEnv("SESSION_BOOST_WEIGHT", "0.003"), 64); err == nil && w >= 0 && w <= 0.05 {
		cfg.SessionBoostWeight = w
	}
	if w, err := strconv.ParseFloat(getEnv("BOOST_WEIGHT", "0.002"), 64); err == nil && w >= 0 && w <= 0.05 {
		cfg.BoostWeight = w
	}
	if alpha, err := strconv.ParseFloat(getEnv("DEFAULT_HYBRID_ALPHA", "0.5"), 64); err == nil {
		cfg.DefaultAlpha = alpha
	}
	if limit, err := strconv.Atoi(getEnv("DEFAULT_LIMIT", "20")); err == nil {
		cfg.DefaultLimit = limit
	}
	if minScore, err := strconv.ParseFloat(getEnv("MIN_SCORE_VALUE", "0.0"), 64); err == nil {
		cfg.MinScoreValue = minScore
	}

	return cfg, nil
}

// LegacyEmbeddingDimension is the dimension of the v1 `embedding` column
// (deterministic hash embeddings, ARRAY<FLOAT32>(vector_length=>768)).
const LegacyEmbeddingDimension = 768

// Validate checks that required settings are present when the Spanner backend is enabled (#52).
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is nil")
	}
	if !c.UseMemoryStore && strings.TrimSpace(c.ProjectID) == "" {
		return fmt.Errorf("PROJECT_ID is required when USE_MEMORY_STORE is false")
	}
	return nil
}

// UseGeminiEmbeddings reports whether the v2 (Gemini, embedding_v2) vector column is active.
func (c *Config) UseGeminiEmbeddings() bool { return c.ActiveVersion() == "v2" }

// ActiveVersion returns the embedding version currently in effect. It can be
// switched at runtime (per instance) via SetActiveVersion (ADR-001 Blue/Green).
func (c *Config) ActiveVersion() string {
	if p := c.activeVersion.Load(); p != nil {
		return *p
	}
	return c.ActiveEmbeddingVersion
}

// SetActiveVersion switches the active embedding version ("v1" or "v2") for this instance.
func (c *Config) SetActiveVersion(v string) error {
	if v != "v1" && v != "v2" {
		return fmt.Errorf("invalid embedding version %q (want v1 or v2)", v)
	}
	c.activeVersion.Store(&v)
	if !c.useVectorIndexExplicit {
		c.UseVectorIndex = v == "v2"
	}
	return nil
}

// EffectiveUseVectorIndex reports whether the ScaNN ANN vector index should be used
// for the currently active embedding version (ADR-012, Issue #39).
func (c *Config) EffectiveUseVectorIndex() bool {
	if c == nil || !c.UseGeminiEmbeddings() {
		return false
	}
	return c.UseVectorIndex
}

// EffectiveANNNumLeaves returns the ScaNN num_leaves_to_search option (default 20).
func (c *Config) EffectiveANNNumLeaves() int {
	if c != nil && c.ANNNumLeavesToSearch > 0 && c.ANNNumLeavesToSearch <= 1000 {
		return c.ANNNumLeavesToSearch
	}
	return 20
}

// ActiveDimension returns the dimension of the currently active vector column.
func (c *Config) ActiveDimension() int {
	if c.UseGeminiEmbeddings() {
		return c.GeminiEmbeddingDimension
	}
	return LegacyEmbeddingDimension
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ParseAdSlots parses a comma-separated 1-based slot list (e.g. "1,4,8").
func ParseAdSlots(raw string) []int {
	var out []int
	seen := map[int]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil && n > 0 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return []int{1, 4, 8}
	}
	return out
}

// EffectiveAdSlots returns the active 1-based ad slots capped by EffectiveAdMax.
func (c *Config) EffectiveAdSlots() []int {
	slots := []int{1, 4, 8}
	if c != nil && len(c.AdSlots) > 0 {
		slots = append([]int(nil), c.AdSlots...)
	}
	k := c.EffectiveAdMax()
	if len(slots) > k {
		slots = slots[:k]
	}
	return slots
}

// EffectiveAdMax returns K (max sponsored products per response; default 3).
func (c *Config) EffectiveAdMax() int {
	if c != nil && c.AdMax > 0 {
		return c.AdMax
	}
	if c != nil && len(c.AdSlots) > 0 {
		return len(c.AdSlots)
	}
	return 3
}

// EffectiveAdMinSimilarity returns the minimum vector similarity for sponsored products (default 0.55).
func (c *Config) EffectiveAdMinSimilarity() float64 {
	if c != nil && c.AdMinSimilarity > 0 && c.AdMinSimilarity <= 1.0 {
		return c.AdMinSimilarity
	}
	return 0.55
}

func parseEnvDuration(key string) (time.Duration, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(raw); err == nil {
		return d, true
	}
	if ms, err := strconv.Atoi(raw); err == nil {
		return time.Duration(ms) * time.Millisecond, true
	}
	return 0, false
}

// EffectiveSearchTimeout returns the per-search context deadline (default 10s, capped at 14s
// so it always stays below the 15s HTTP WriteTimeout in cmd/server/main.go; Issue #66).
func (c *Config) EffectiveSearchTimeout() time.Duration {
	if c != nil && c.SearchTimeout > 0 {
		if c.SearchTimeout > 14*time.Second {
			return 14 * time.Second
		}
		return c.SearchTimeout
	}
	return 10 * time.Second
}

// EffectiveSearchMaxInflight returns the per-instance in-flight search semaphore limit (default 8; Issue #66).
func (c *Config) EffectiveSearchMaxInflight() int {
	if c != nil && c.SearchMaxInflight > 0 && c.SearchMaxInflight <= 256 {
		return c.SearchMaxInflight
	}
	return 8
}

// EffectiveSearchQueueWait returns the max wait duration for a search admission slot before returning 503 (default 250ms; Issue #66).
func (c *Config) EffectiveSearchQueueWait() time.Duration {
	if c != nil && c.SearchQueueWaitSet {
		if c.SearchQueueWait < 0 {
			return 0
		}
		if c.SearchQueueWait > 5*time.Second {
			return 5 * time.Second
		}
		return c.SearchQueueWait
	}
	if c != nil && c.SearchQueueWait > 0 {
		if c.SearchQueueWait > 5*time.Second {
			return 5 * time.Second
		}
		return c.SearchQueueWait
	}
	return 250 * time.Millisecond
}

