package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                    int
	Environment             string
	ProjectID               string
	Region                  string
	SpannerInstanceID       string
	SpannerDatabaseID       string
	GeminiModelName         string
	EmbeddingDimension      int
	ActiveEmbeddingVersion  string
	DefaultAlpha            float64
	DefaultLimit            int
	MinScoreValue           float64
	UseMemoryStore          bool
	StaticDir               string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:                   8080,
		Environment:            getEnv("ENVIRONMENT", "production"),
		ProjectID:              getEnv("PROJECT_ID", "riojucu-sandbox"),
		Region:                 getEnv("REGION", "us-central1"),
		SpannerInstanceID:      getEnv("SPANNER_INSTANCE_ID", "psearch-instance"),
		SpannerDatabaseID:      getEnv("SPANNER_DATABASE_ID", "psearch-db"),
		GeminiModelName:        getEnv("GEMINI_MODEL_NAME", "text-multilingual-embedding-002"),
		EmbeddingDimension:     768,
		ActiveEmbeddingVersion: getEnv("ACTIVE_EMBEDDING_VERSION", "v1"),
		DefaultAlpha:           0.5,
		DefaultLimit:           20,
		MinScoreValue:          0.0,
		UseMemoryStore:         getEnv("USE_MEMORY_STORE", "false") == "true",
		StaticDir:              getEnv("STATIC_DIR", "/app/static"),
	}

	if port, err := strconv.Atoi(getEnv("PORT", "8080")); err == nil {
		cfg.Port = port
	}
	if dim, err := strconv.Atoi(getEnv("EMBEDDING_DIMENSION", "768")); err == nil {
		cfg.EmbeddingDimension = dim
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

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
