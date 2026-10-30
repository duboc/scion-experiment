package config

import (
	"os"
	"testing"
)

func TestNormalizeModelID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"gemini-4-pro", "gemini-4-argon"},
		{"gemini-4.0-pro", "gemini-4-argon"},
		{"GEMINI-4-PRO", "gemini-4-argon"},
		{"gemini-4-argon", "gemini-4-argon"},
		{"gemini-3.8-flash", "gemini-3.8-flash"},
		{"custom-model", "custom-model"},
		{"", ""},
	}

	for _, tt := range tests {
		got := NormalizeModelID(tt.input)
		if got != tt.want {
			t.Errorf("NormalizeModelID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestConfigModelDefaults(t *testing.T) {
	// Clear relevant env vars
	os.Unsetenv("GEMINI_TEXT_MODEL")
	os.Unsetenv("GEMINI_LITE_MODEL")
	os.Unsetenv("GEMINI_PRO_MODEL")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.IntentLLMModel != "gemini-3.8-flash" {
		t.Errorf("default IntentLLMModel = %q, want gemini-3.8-flash", cfg.IntentLLMModel)
	}
	if cfg.ProLLMModel != "gemini-4-argon" {
		t.Errorf("default ProLLMModel = %q, want gemini-4-argon", cfg.ProLLMModel)
	}
	if cfg.GenAILocation != "global" {
		t.Errorf("default GenAILocation = %q, want global", cfg.GenAILocation)
	}
}

func TestConfigVectorIndexDefaultsAndEnv(t *testing.T) {
	os.Unsetenv("USE_VECTOR_INDEX")
	os.Unsetenv("ANN_NUM_LEAVES_TO_SEARCH")
	os.Setenv("ACTIVE_EMBEDDING_VERSION", "v2")
	defer os.Unsetenv("ACTIVE_EMBEDDING_VERSION")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.UseVectorIndex || !cfg.EffectiveUseVectorIndex() {
		t.Errorf("expected UseVectorIndex=true by default when ACTIVE_EMBEDDING_VERSION=v2")
	}
	if cfg.EffectiveANNNumLeaves() != 20 {
		t.Errorf("default EffectiveANNNumLeaves() = %d, want 20", cfg.EffectiveANNNumLeaves())
	}

	os.Setenv("USE_VECTOR_INDEX", "false")
	os.Setenv("ANN_NUM_LEAVES_TO_SEARCH", "35")
	defer os.Unsetenv("USE_VECTOR_INDEX")
	defer os.Unsetenv("ANN_NUM_LEAVES_TO_SEARCH")

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg2.UseVectorIndex || cfg2.EffectiveUseVectorIndex() {
		t.Errorf("expected UseVectorIndex=false when USE_VECTOR_INDEX=false")
	}
	if cfg2.EffectiveANNNumLeaves() != 35 {
		t.Errorf("EffectiveANNNumLeaves() = %d, want 35", cfg2.EffectiveANNNumLeaves())
	}
}
