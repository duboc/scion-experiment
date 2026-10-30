/*
 * Copyright 2025 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     https://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"google.golang.org/genai"
	"psearch/serving-go/internal/config"
)

// QueryEmbedder produces query embeddings for vector retrieval.
// It is an interface so SpannerService can be unit-tested without network access.
type QueryEmbedder interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// BatchDocumentEmbedder embeds batches of product documents (ADR-012, Issue #39).
type BatchDocumentEmbedder interface {
	EmbedDocumentsBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// embedContentFunc abstracts the Gen AI SDK call (genai.Models.EmbedContent).
type embedContentFunc func(ctx context.Context, model string, contents []*genai.Content, cfg *genai.EmbedContentConfig) (*genai.EmbedContentResponse, error)

// EmbeddingService generates query embeddings with the Google Gen AI SDK
// (google.golang.org/genai) against Vertex AI (Issue #11, ADR-001).
type EmbeddingService struct {
	model          string
	dimension      int32
	location       string
	embed          embedContentFunc
	retryBaseDelay time.Duration
	maxRetries     int
}

// NewEmbeddingService creates a Vertex AI-backed Gen AI client.
// Gemini 3.x / gemini-embedding-2 are served from the "global" location
// (not us-central1), so GENAI_LOCATION is independent of the Cloud Run region.
func NewEmbeddingService(ctx context.Context, cfg *config.Config) (*EmbeddingService, error) {
	if cfg.ProjectID == "" {
		return nil, errors.New("embedding service: PROJECT_ID is required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  cfg.ProjectID,
		Location: cfg.GenAILocation,
	})
	if err != nil {
		return nil, fmt.Errorf("embedding service: create genai client: %w", err)
	}
	return newEmbeddingServiceWith(cfg, client.Models.EmbedContent), nil
}

func newEmbeddingServiceWith(cfg *config.Config, fn embedContentFunc) *EmbeddingService {
	return &EmbeddingService{
		model:          cfg.EmbeddingModelName,
		dimension:      int32(cfg.GeminiEmbeddingDimension),
		location:       cfg.GenAILocation,
		embed:          fn,
		retryBaseDelay: 100 * time.Millisecond,
		maxRetries:     4,
	}
}

// Model returns the configured embedding model ID.
func (s *EmbeddingService) Model() string { return s.model }

// EmbedQuery returns a RETRIEVAL_QUERY embedding with the configured output dimensionality.
func (s *EmbeddingService) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return s.embedText(ctx, text, "RETRIEVAL_QUERY")
}

// EmbedDocument returns a RETRIEVAL_DOCUMENT embedding (used to backfill embedding_v2).
func (s *EmbeddingService) EmbedDocument(ctx context.Context, text string) ([]float32, error) {
	return s.embedText(ctx, text, "RETRIEVAL_DOCUMENT")
}

func (s *EmbeddingService) embedText(ctx context.Context, text, taskType string) ([]float32, error) {
	start := time.Now()
	res, err := s.embed(ctx, s.model, genai.Text(text), &genai.EmbedContentConfig{
		TaskType:             taskType,
		OutputDimensionality: genai.Ptr(s.dimension),
	})
	if err != nil {
		return nil, fmt.Errorf("embed %s (%s @ %s): %w", taskType, s.model, s.location, err)
	}
	if res == nil || len(res.Embeddings) == 0 || res.Embeddings[0] == nil || len(res.Embeddings[0].Values) == 0 {
		return nil, fmt.Errorf("embed %s (%s): empty embedding in response", taskType, s.model)
	}
	vals := res.Embeddings[0].Values
	if int32(len(vals)) != s.dimension {
		return nil, fmt.Errorf("embed %s (%s): got %d dims, want %d", taskType, s.model, len(vals), s.dimension)
	}
	log.Printf("embedding: %s generated %d-d %s vector in %s", s.model, len(vals), taskType, time.Since(start))
	return vals, nil
}

func isRetryableEmbedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, code := range []string{"429", "503", "resource_exhausted", "unavailable", "rate limit", "quota", "too many requests"} {
		if strings.Contains(msg, code) {
			return true
		}
	}
	return false
}

// EmbedDocumentsBatch generates RETRIEVAL_DOCUMENT embeddings for a batch of texts in a
// single Gen AI SDK RPC with exponential-backoff retry on 429/503 (ADR-012, Issue #39).
func (s *EmbeddingService) EmbedDocumentsBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	contents := make([]*genai.Content, 0, len(texts))
	for _, t := range texts {
		contents = append(contents, genai.Text(t)...)
	}
	cfg := &genai.EmbedContentConfig{
		TaskType:             "RETRIEVAL_DOCUMENT",
		OutputDimensionality: genai.Ptr(s.dimension),
	}
	baseDelay := s.retryBaseDelay
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	maxRetries := s.maxRetries
	if maxRetries <= 0 {
		maxRetries = 4
	}
	var res *genai.EmbedContentResponse
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		res, err = s.embed(ctx, s.model, contents, cfg)
		if err == nil {
			break
		}
		if attempt == maxRetries || !isRetryableEmbedError(err) {
			return nil, fmt.Errorf("embed batch (%s @ %s, n=%d): %w", s.model, s.location, len(texts), err)
		}
		delay := baseDelay * (1 << attempt)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	if res == nil || len(res.Embeddings) != len(texts) {
		got := 0
		if res != nil {
			got = len(res.Embeddings)
		}
		return nil, fmt.Errorf("embed batch (%s): got %d embeddings, want %d", s.model, got, len(texts))
	}
	out := make([][]float32, len(texts))
	for i, emb := range res.Embeddings {
		if emb == nil || len(emb.Values) == 0 {
			return nil, fmt.Errorf("embed batch (%s): empty embedding at index %d", s.model, i)
		}
		if int32(len(emb.Values)) != s.dimension {
			return nil, fmt.Errorf("embed batch (%s): index %d got %d dims, want %d", s.model, i, len(emb.Values), s.dimension)
		}
		out[i] = emb.Values
	}
	return out, nil
}

// BackfillRow is one candidate product row to embed during backfill (ADR-012, Issue #39).
type BackfillRow struct {
	ID   string
	Text string
}

// BackfillUpdate is one embedded product row ready to be written to Spanner.
type BackfillUpdate struct {
	ID     string
	Vector []float32
}

// BackfillChunkWriter persists a chunk of BackfillUpdate rows (e.g., via spanner.Client.Apply).
type BackfillChunkWriter func(ctx context.Context, chunk []BackfillUpdate) error

// BackfillOptions configures concurrent batched embedding backfill (ADR-012, Issue #39).
type BackfillOptions struct {
	Workers    int
	EmbedBatch int
	WriteChunk int
	Limit      int
	DryRun     bool
}

// BackfillStats reports backfill execution metrics.
type BackfillStats struct {
	TotalEmbedded   int
	TotalWritten    int
	Batches         int
	ChunksCommitted int
}

// RunEmbeddingBackfill embeds rows concurrently in batches of opts.EmbedBatch using
// opts.Workers goroutines and flushes mutations in chunks of opts.WriteChunk rows
// immediately as workers finish so runs stay well below Spanner's 80k mutation ceiling (ADR-012, #39).
func RunEmbeddingBackfill(ctx context.Context, rows []BackfillRow, embedder BatchDocumentEmbedder, writer BackfillChunkWriter, opts BackfillOptions) (BackfillStats, error) {
	if opts.Workers <= 0 {
		opts.Workers = 8
	}
	if opts.EmbedBatch <= 0 {
		opts.EmbedBatch = 32
	}
	if opts.WriteChunk <= 0 {
		opts.WriteChunk = 200
	}
	if opts.Limit > 0 && len(rows) > opts.Limit {
		rows = rows[:opts.Limit]
	}
	total := len(rows)
	if total == 0 {
		return BackfillStats{}, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var batches [][]BackfillRow
	for i := 0; i < total; i += opts.EmbedBatch {
		end := i + opts.EmbedBatch
		if end > total {
			end = total
		}
		batches = append(batches, rows[i:end])
	}

	workers := opts.Workers
	if workers > len(batches) {
		workers = len(batches)
	}

	type batchResult struct {
		updates []BackfillUpdate
		err     error
	}

	jobCh := make(chan []BackfillRow, len(batches))
	for _, b := range batches {
		jobCh <- b
	}
	close(jobCh)

	resCh := make(chan batchResult, len(batches))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range jobCh {
				if ctx.Err() != nil {
					return
				}
				texts := make([]string, len(batch))
				for i, r := range batch {
					texts[i] = r.Text
				}
				vecs, err := embedder.EmbedDocumentsBatch(ctx, texts)
				if err != nil {
					resCh <- batchResult{err: err}
					cancel()
					return
				}
				updates := make([]BackfillUpdate, len(batch))
				for i, r := range batch {
					updates[i] = BackfillUpdate{ID: r.ID, Vector: vecs[i]}
				}
				resCh <- batchResult{updates: updates}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(resCh)
	}()

	start := time.Now()
	var stats BackfillStats
	var pending []BackfillUpdate

	flushChunk := func(chunk []BackfillUpdate) error {
		if len(chunk) == 0 {
			return nil
		}
		if !opts.DryRun && writer != nil {
			if err := writer(ctx, chunk); err != nil {
				return err
			}
			stats.TotalWritten += len(chunk)
			stats.ChunksCommitted++
		}
		elapsed := time.Since(start).Seconds()
		rate := 0.0
		if elapsed > 0 {
			rate = float64(stats.TotalEmbedded) / elapsed
		}
		log.Printf("backfill: embedded %d/%d rows (written=%d, chunks=%d, rate=%.1f docs/s)",
			stats.TotalEmbedded, total, stats.TotalWritten, stats.ChunksCommitted, rate)
		return nil
	}

	for res := range resCh {
		if res.err != nil {
			cancel()
			return stats, res.err
		}
		stats.Batches++
		stats.TotalEmbedded += len(res.updates)
		pending = append(pending, res.updates...)
		for len(pending) >= opts.WriteChunk {
			chunk := make([]BackfillUpdate, opts.WriteChunk)
			copy(chunk, pending[:opts.WriteChunk])
			pending = pending[opts.WriteChunk:]
			if err := flushChunk(chunk); err != nil {
				cancel()
				return stats, err
			}
		}
	}
	if len(pending) > 0 {
		if err := flushChunk(pending); err != nil {
			return stats, err
		}
	}
	return stats, nil
}
