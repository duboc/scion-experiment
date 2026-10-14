package services

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"strings"
	"testing"

	"google.golang.org/genai"
	"psearch/serving-go/internal/models"
)

var (
	jpegBytes = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, []byte("fake-jpeg-body")...)
	pngBytes  = append([]byte("\x89PNG\r\n\x1a\n"), []byte("fake-png-body")...)
)

func TestDecodeQueryImage(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString(jpegBytes)
	if data, mime, err := DecodeQueryImage(b64, "image/jpg"); err != nil || mime != "image/jpeg" || len(data) != len(jpegBytes) {
		t.Fatalf("jpeg: data=%d mime=%q err=%v", len(data), mime, err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	if _, mime, err := DecodeQueryImage(dataURL, ""); err != nil || mime != "image/png" {
		t.Fatalf("data URL: mime=%q err=%v", mime, err)
	}
	if _, _, err := DecodeQueryImage(base64.RawStdEncoding.EncodeToString(jpegBytes), "image/jpeg"); err != nil {
		t.Fatalf("unpadded base64 must be accepted: %v", err)
	}
	big := make([]byte, MaxQueryImageBytes+10)
	copy(big, jpegBytes)
	for name, tc := range map[string][2]string{
		"gif mime":      {b64, "image/gif"},
		"no mime":       {b64, ""},
		"bad base64":    {"%%%not-base64%%%", "image/jpeg"},
		"sniff":         {base64.StdEncoding.EncodeToString(pngBytes), "image/jpeg"},
		"empty":         {"", "image/jpeg"},
		"oversize":      {base64.StdEncoding.EncodeToString(big), "image/jpeg"},
		"bad data URL":  {"data:image/png;base64", ""},
		"url not bytes": {"https://example.com/shoe.jpg", "image/jpeg"},
	} {
		if _, _, err := DecodeQueryImage(tc[0], tc[1]); !errors.Is(err, ErrInvalidImage) {
			t.Errorf("%s: want ErrInvalidImage, got %v", name, err)
		}
	}
}

func TestEmbedImage_SendsInlineBytesAtConfiguredDimension(t *testing.T) {
	var gotCfg genai.EmbedContentConfig
	var gotContents []*genai.Content
	fn := func(_ context.Context, m string, c []*genai.Content, cfg *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error) {
		gotCfg, gotContents = *cfg, c
		return &genai.EmbedContentResponse{Embeddings: []*genai.ContentEmbedding{{Values: make([]float32, 1024)}}}, nil
	}
	svc := newEmbeddingServiceWith(testConfig("v2"), fn)
	vec, err := svc.EmbedImage(context.Background(), jpegBytes, "image/jpeg")
	if err != nil || len(vec) != 1024 {
		t.Fatalf("vec=%d err=%v", len(vec), err)
	}
	if gotCfg.TaskType != "" {
		t.Errorf("images take no task type, got %q", gotCfg.TaskType)
	}
	if gotCfg.OutputDimensionality == nil || *gotCfg.OutputDimensionality != 1024 {
		t.Errorf("want 1024-d output, got %v", gotCfg.OutputDimensionality)
	}
	if len(gotContents) != 1 || len(gotContents[0].Parts) != 1 || gotContents[0].Parts[0].InlineData == nil ||
		gotContents[0].Parts[0].InlineData.MIMEType != "image/jpeg" {
		t.Fatalf("want exactly one content with one inline image part, got %+v", gotContents)
	}
	if _, err := newEmbeddingServiceWith(testConfig("v2"), fakeEmbed(3072, nil, nil, nil)).EmbedImage(context.Background(), jpegBytes, "image/jpeg"); err == nil {
		t.Error("expected dimension mismatch error")
	}
}

type countingImageEmbedder struct {
	calls int
	err   error
}

func (c *countingImageEmbedder) EmbedImage(context.Context, []byte, string) ([]float32, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return []float32{1, 0, 0}, nil
}

func TestImageEmbeddingCache_LRU(t *testing.T) {
	inner := &countingImageEmbedder{}
	c := NewImageEmbeddingCache(inner, 2)
	ctx := context.Background()
	if _, hit, _ := c.EmbedImageWithMeta(ctx, []byte("a"), "image/jpeg"); hit {
		t.Fatal("first call must miss")
	}
	if _, hit, _ := c.EmbedImageWithMeta(ctx, []byte("a"), "image/jpeg"); !hit {
		t.Fatal("second call must hit")
	}
	if _, hit, _ := c.EmbedImageWithMeta(ctx, []byte("a"), "image/png"); hit {
		t.Fatal("mime is part of the key")
	}
	c.EmbedImageWithMeta(ctx, []byte("b"), "image/jpeg") // evicts the oldest ("a"/jpeg)
	if _, hit, _ := c.EmbedImageWithMeta(ctx, []byte("a"), "image/jpeg"); hit {
		t.Fatal("LRU must evict beyond maxEntries")
	}
	if inner.calls != 4 {
		t.Fatalf("want 4 upstream calls, got %d", inner.calls)
	}
	failing := NewImageEmbeddingCache(&countingImageEmbedder{err: errors.New("403")}, 2)
	if _, _, err := failing.EmbedImageWithMeta(ctx, []byte("x"), "image/jpeg"); err == nil {
		t.Fatal("errors must propagate and not be cached")
	}
}

func l2norm(v []float32) float64 {
	var s float64
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func TestFuseVectors(t *testing.T) {
	img, txt := []float32{3, 0}, []float32{0, 2}
	if f := FuseVectors(img, nil, 0.6); f[0] != 1 || f[1] != 0 {
		t.Fatalf("image-only must be the unit image vector, got %v", f)
	}
	f := FuseVectors(img, txt, 0.6)
	if math.Abs(l2norm(f)-1) > 1e-6 {
		t.Fatalf("fused vector must be unit length, got %v", l2norm(f))
	}
	if !(f[0] > f[1] && f[1] > 0) {
		t.Fatalf("w=0.6 must lean to the image but include text, got %v", f)
	}
	if f := FuseVectors(img, txt, 1); f[1] != 0 {
		t.Fatalf("w=1 ignores text, got %v", f)
	}
	if f := FuseVectors(img, []float32{1, 2, 3}, 0.5); f[1] != 0 {
		t.Fatalf("dimension mismatch must ignore text, got %v", f)
	}
}

func TestBuildHybridSQLMode_VectorOnly(t *testing.T) {
	sql := buildHybridSQLMode(vectorColumnV2, false, true, true)
	for _, bad := range []string{"fts AS", "FROM fts", "SEARCH(", "@query_text"} {
		if strings.Contains(sql, bad) {
			t.Errorf("vector-only SQL must not contain %q", bad)
		}
	}
	for _, want := range []string{"WITH\n\t\tann AS", "COSINE_DISTANCE(embedding_v2, @query_embedding)", "'vector' AS source, 1.0 AS weight", "COALESCE(is_visible, TRUE)", "variants"} {
		if !strings.Contains(sql, want) {
			t.Errorf("vector-only SQL missing %q", want)
		}
	}
	if strings.Contains(sql, "),\n\t\tSELECT") || strings.Contains(sql, "),\n\t\tSELECT rrf_score") {
		t.Error("vector-only SQL has a dangling comma after the ann CTE")
	}
	if buildHybridSQLMode(vectorColumnV2, false, false, false) != buildHybridSQL(vectorColumnV2, false) {
		t.Error("vectorOnly=false must render the existing hybrid SQL unchanged")
	}
	if lex := buildHybridSQLMode(vectorColumnV2, true, true, false); strings.Contains(lex, "ann AS") || !strings.Contains(lex, "fts AS") {
		t.Error("lexicalOnly must win over vectorOnly")
	}
}

func TestPlanImageQuery(t *testing.T) {
	img := make([]float32, 1024)
	img[0] = 1
	txt := make([]float32, 1024)
	txt[1] = 1
	f := models.SearchFilters{ImageVector: img}

	svc := &SpannerService{config: testConfig("v2"), embeddings: stubEmbedder{vec: txt}}
	if p := svc.planImageQuery(context.Background(), "", f); !p.vectorOnly || !p.multimodal || p.lexicalOnly {
		t.Fatalf("image-only plan wrong: %+v", p)
	}
	p := svc.planImageQuery(context.Background(), "mas em verde", f)
	v, _ := p.embedding.([]float32)
	if p.vectorOnly || p.lexicalOnly || !p.multimodal || p.column != vectorColumnV2 || len(v) != 1024 || v[0] <= v[1] || v[1] <= 0 {
		t.Fatalf("image+text plan must fuse (image-leaning) and keep the text leg: %+v", p)
	}
	svc.embeddings = stubEmbedder{err: errors.New("403")}
	p = svc.planImageQuery(context.Background(), "mas em verde", f)
	v, _ = p.embedding.([]float32)
	if p.lexicalOnly || p.vectorOnly || v[0] != 1 || v[1] != 0 {
		t.Fatalf("text embed failure must fall back to the image vector with the text leg kept: %+v", p)
	}
}

func TestHybridSearchDetailed_ImageInMemoryMode(t *testing.T) {
	svc := &SpannerService{config: testConfig("v2"), seedProducts: DefaultSeedProducts()}
	img := make([]float32, 1024)
	img[0] = 1
	out, err := svc.HybridSearchDetailed(context.Background(), "", 10, 0, 0.7, models.SearchFilters{ImageVector: img})
	if err != nil || out.RetrievalMode != RetrievalMemory || len(out.Results) != 0 {
		t.Fatalf("image-only without Spanner must return empty memory_fallback (not a catalog browse): mode=%s n=%d err=%v", out.RetrievalMode, len(out.Results), err)
	}
	out, _ = svc.HybridSearchDetailed(context.Background(), "tenis", 10, 0, 0.7, models.SearchFilters{ImageVector: img})
	if out.RetrievalMode != RetrievalMemory || len(out.Results) == 0 {
		t.Fatalf("image+text without Spanner must serve the text refinement from memory: mode=%s n=%d", out.RetrievalMode, len(out.Results))
	}
}
