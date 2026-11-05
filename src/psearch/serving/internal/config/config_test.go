package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

func TestConfigProjectIDDefaultEmptyAndSpannerValidation(t *testing.T) {
	os.Unsetenv("PROJECT_ID")
	os.Unsetenv("GCP_SANDBOX_PROJECT")
	os.Unsetenv("CLOUDSDK_CORE_PROJECT")
	os.Unsetenv("USE_MEMORY_STORE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ProjectID != "" {
		t.Fatalf("default ProjectID = %q, want empty string", cfg.ProjectID)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() must fail fast when UseMemoryStore=false and ProjectID is empty")
	}

	cfg.UseMemoryStore = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() must succeed in memory mode without ProjectID: %v", err)
	}
}

func TestTrackedSourceContainsNoForbiddenIDPattern(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available; skipping tracked-file scan")
	}
	topCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	topOut, err := topCmd.Output()
	if err != nil {
		t.Skipf("not in a git working tree (%v); skipping tracked-file scan", err)
	}
	repoRoot := strings.TrimSpace(string(topOut))
	for _, mustExist := range []string{"README.md", "Dockerfile"} {
		if _, err := os.Stat(filepath.Join(repoRoot, mustExist)); err != nil {
			t.Fatalf("resolved repoRoot %q missing top-level %s: %v", repoRoot, mustExist, err)
		}
	}

	cmd := exec.Command("git", "ls-files")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git ls-files in %s: %v", repoRoot, err)
	}
	files := strings.Split(strings.TrimSpace(string(out)), "\n")
	seenTopLevel := map[string]bool{}
	for _, relPath := range files {
		seenTopLevel[strings.TrimSpace(relPath)] = true
	}
	for _, required := range []string{"README.md", "Dockerfile"} {
		if !seenTopLevel[required] {
			t.Fatalf("git ls-files from %q did not include top-level %s (scanned subtree instead of repo root)", repoRoot, required)
		}
	}

	pattern := strings.TrimSpace(os.Getenv("FORBIDDEN_ID_PATTERN"))
	if pattern == "" {
		t.Skip("FORBIDDEN_ID_PATTERN unset; skipping tracked-file content scan (root and file-list checks passed)")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("invalid FORBIDDEN_ID_PATTERN %q: %v", pattern, err)
	}

	for _, relPath := range files {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, relPath))
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for idx, line := range lines {
			if re.MatchString(line) {
				t.Errorf("forbidden ID pattern matched in %s:%d", relPath, idx+1)
			}
		}
	}
}
