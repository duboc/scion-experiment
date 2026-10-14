package services

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
)

// Visual & multimodal search (Issue #33, ADR-010).
//
// gemini-embedding-2 is natively multimodal: an image embedded at the configured
// output dimensionality (1024) lands in the same space as the product text
// vectors already stored in products.embedding_v2. A photo query therefore needs
// no new column, index or catalog backfill; "this dress, but green" is a fused
// vector normalize(w*v_img + (1-w)*v_txt) plus the text modifier in the FTS leg.

const (
	// MaxQueryImageBytes caps the decoded upload (the storefront resizes to <= 1024 px).
	MaxQueryImageBytes = 4 << 20
	// DefaultImageWeight is the image share of the fused query vector.
	DefaultImageWeight = 0.6
	// DefaultMultimodalAlpha weights the vector leg when an image is present and the
	// caller did not set alpha (cross-modal cosines are low, ~0.25-0.35, so the image
	// leg must not be drowned by the text leg).
	DefaultMultimodalAlpha = 0.7
	// RetrievalMultimodal is reported when a query image drove the vector leg.
	RetrievalMultimodal = "multimodal_v2"
)

var allowedImageMIME = map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}

// ErrInvalidImage is returned for malformed, oversized or unsupported uploads (HTTP 400).
var ErrInvalidImage = errors.New("invalid query image")

// ImageEmbedder embeds a query image into the product vector space.
type ImageEmbedder interface {
	EmbedImage(ctx context.Context, data []byte, mimeType string) ([]float32, error)
}

// DecodeQueryImage validates and decodes a base64 upload. A data URL prefix
// ("data:image/png;base64,") is accepted and, when mimeType is empty, used to
// infer it. Only inline bytes are accepted: image URLs are never fetched
// server-side (SSRF).
func DecodeQueryImage(b64, mimeType string) ([]byte, string, error) {
	b64 = strings.TrimSpace(b64)
	if strings.HasPrefix(b64, "data:") {
		comma := strings.IndexByte(b64, ',')
		if comma < 0 {
			return nil, "", fmt.Errorf("%w: malformed data URL", ErrInvalidImage)
		}
		header := b64[len("data:"):comma]
		if mimeType == "" {
			mimeType = strings.TrimSuffix(header, ";base64")
		}
		b64 = b64[comma+1:]
	}
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "image/jpg" {
		mimeType = "image/jpeg"
	}
	if !allowedImageMIME[mimeType] {
		return nil, "", fmt.Errorf("%w: image_mime_type must be image/jpeg, image/png or image/webp", ErrInvalidImage)
	}
	if base64.StdEncoding.DecodedLen(len(b64)) > MaxQueryImageBytes+3 {
		return nil, "", fmt.Errorf("%w: image larger than %d bytes", ErrInvalidImage, MaxQueryImageBytes)
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		if data, err = base64.RawStdEncoding.DecodeString(b64); err != nil {
			return nil, "", fmt.Errorf("%w: image_base64 is not valid base64", ErrInvalidImage)
		}
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("%w: empty image", ErrInvalidImage)
	}
	if len(data) > MaxQueryImageBytes {
		return nil, "", fmt.Errorf("%w: image larger than %d bytes", ErrInvalidImage, MaxQueryImageBytes)
	}
	if !sniffMatches(data, mimeType) {
		return nil, "", fmt.Errorf("%w: content does not match %s", ErrInvalidImage, mimeType)
	}
	return data, mimeType, nil
}

func sniffMatches(b []byte, mimeType string) bool {
	switch mimeType {
	case "image/jpeg":
		return len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF
	case "image/png":
		return len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n"
	case "image/webp":
		return len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
	}
	return false
}

// EmbedImage returns a 1024-d gemini-embedding-2 vector for an image (no task
// type applies to images). One content per call: the model rejects batches.
func (s *EmbeddingService) EmbedImage(ctx context.Context, data []byte, mimeType string) ([]float32, error) {
	start := time.Now()
	res, err := s.embed(ctx, s.model, []*genai.Content{genai.NewContentFromBytes(data, mimeType, genai.RoleUser)}, &genai.EmbedContentConfig{
		OutputDimensionality: genai.Ptr(s.dimension),
	})
	if err != nil {
		return nil, fmt.Errorf("embed image (%s @ %s): %w", s.model, s.location, err)
	}
	if res == nil || len(res.Embeddings) == 0 || res.Embeddings[0] == nil || len(res.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("embed image (%s): empty embedding in response", s.model)
	}
	vals := res.Embeddings[0].Values
	if int32(len(vals)) != s.dimension {
		return nil, fmt.Errorf("embed image (%s): got %d dims, want %d", s.model, len(vals), s.dimension)
	}
	log.Printf("embedding: %s generated %d-d image vector (%d bytes) in %s", s.model, len(vals), len(data), time.Since(start))
	return vals, nil
}

// ImageEmbeddingCache is a bounded in-process LRU keyed by sha256(mime|bytes):
// warm image calls cost ~350-550 ms, so repeated photos (retries, pagination,
// refinement edits) must not re-embed.
type ImageEmbeddingCache struct {
	inner ImageEmbedder
	max   int
	mu    sync.Mutex
	ll    *list.List
	items map[[32]byte]*list.Element
}

type imageCacheEntry struct {
	key [32]byte
	vec []float32
}

// NewImageEmbeddingCache wraps inner with an LRU of maxEntries vectors.
func NewImageEmbeddingCache(inner ImageEmbedder, maxEntries int) *ImageEmbeddingCache {
	if maxEntries <= 0 {
		maxEntries = 256
	}
	return &ImageEmbeddingCache{inner: inner, max: maxEntries, ll: list.New(), items: map[[32]byte]*list.Element{}}
}

// EmbedImageWithMeta returns the vector and whether it was served from cache.
func (c *ImageEmbeddingCache) EmbedImageWithMeta(ctx context.Context, data []byte, mimeType string) ([]float32, bool, error) {
	h := sha256.New()
	h.Write([]byte(mimeType))
	h.Write([]byte{0})
	h.Write(data)
	var key [32]byte
	copy(key[:], h.Sum(nil))
	c.mu.Lock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		vec := el.Value.(*imageCacheEntry).vec
		c.mu.Unlock()
		return vec, true, nil
	}
	c.mu.Unlock()
	vec, err := c.inner.EmbedImage(ctx, data, mimeType)
	if err != nil {
		return nil, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return vec, false, nil
	}
	c.items[key] = c.ll.PushFront(&imageCacheEntry{key: key, vec: vec})
	for c.ll.Len() > c.max {
		old := c.ll.Back()
		c.ll.Remove(old)
		delete(c.items, old.Value.(*imageCacheEntry).key)
	}
	return vec, false, nil
}

// EmbedImage implements ImageEmbedder.
func (c *ImageEmbeddingCache) EmbedImage(ctx context.Context, data []byte, mimeType string) ([]float32, error) {
	v, _, err := c.EmbedImageWithMeta(ctx, data, mimeType)
	return v, err
}

// FuseVectors returns normalize(w*normalize(img) + (1-w)*normalize(txt)).
// With no text vector (or w >= 1) it returns the normalized image vector.
func FuseVectors(img, txt []float32, w float64) []float32 {
	if w < 0 {
		w = 0
	}
	if w > 1 {
		w = 1
	}
	ni := unit(img)
	if len(txt) != len(img) || w >= 1 {
		return ni
	}
	nt := unit(txt)
	out := make([]float32, len(img))
	for i := range out {
		out[i] = float32(w*float64(ni[i]) + (1-w)*float64(nt[i]))
	}
	return unit(out)
}

func unit(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	out := make([]float32, len(v))
	if sum == 0 {
		copy(out, v)
		return out
	}
	n := math.Sqrt(sum)
	for i, x := range v {
		out[i] = float32(float64(x) / n)
	}
	return out
}
