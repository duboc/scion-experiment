// Command backfill-embeddings populates products.embedding_v2 with
// gemini-embedding-2 (RETRIEVAL_DOCUMENT, 1024 dims) via the Google Gen AI SDK.
// Run by @deployer after migration 011 and before setting ACTIVE_EMBEDDING_VERSION=v2
// (ADR-001 Blue/Green, Issue #11). Rows that already have embedding_v2 are skipped
// unless -force is given.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
	"psearch/serving-go/internal/config"
	"psearch/serving-go/internal/services"
)

func main() {
	force := flag.Bool("force", false, "re-embed rows that already have embedding_v2")
	dryRun := flag.Bool("dry-run", false, "embed but do not write to Spanner")
	workers := flag.Int("workers", 8, "concurrent batch embedding workers (ADR-012, #39)")
	embedBatch := flag.Int("embed-batch", 32, "documents per EmbedDocumentsBatch RPC (ADR-012, #39)")
	writeChunk := flag.Int("write-chunk", 200, "rows per Spanner client.Apply transaction (ADR-012, #39)")
	limit := flag.Int("limit", 0, "max rows to backfill (0 = all matching rows)")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	emb, err := services.NewEmbeddingService(ctx, cfg)
	if err != nil {
		log.Fatalf("embedding service: %v", err)
	}
	db := fmt.Sprintf("projects/%s/instances/%s/databases/%s", cfg.ProjectID, cfg.SpannerInstanceID, cfg.SpannerDatabaseID)
	client, err := spanner.NewClient(ctx, db)
	if err != nil {
		log.Fatalf("spanner: %v", err)
	}
	defer client.Close()

	where := "WHERE embedding_v2 IS NULL"
	if *force {
		where = ""
	}
	limitSQL := ""
	params := map[string]interface{}{}
	if *limit > 0 {
		limitSQL = " LIMIT @row_limit"
		params["row_limit"] = int64(*limit)
	}
	stmt := spanner.Statement{
		SQL: `SELECT product_id, IFNULL(title, ''), IFNULL(description, ''), IFNULL(category, ''), IFNULL(brand, '')
		FROM products ` + where + limitSQL,
		Params: params,
	}
	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var rows []services.BackfillRow
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("query: %v", err)
		}
		var id, title, desc, cat, brand string
		if err := row.Columns(&id, &title, &desc, &cat, &brand); err != nil {
			log.Fatalf("scan: %v", err)
		}
		text := strings.TrimSpace(strings.Join([]string{title, desc, cat, brand}, " "))
		rows = append(rows, services.BackfillRow{ID: id, Text: text})
	}
	iter.Stop()

	writer := func(ctx context.Context, chunk []services.BackfillUpdate) error {
		muts := make([]*spanner.Mutation, 0, len(chunk))
		for _, u := range chunk {
			muts = append(muts, spanner.Update("products", []string{"product_id", "embedding_v2"}, []interface{}{u.ID, u.Vector}))
		}
		_, err := client.Apply(ctx, muts)
		return err
	}

	stats, err := services.RunEmbeddingBackfill(ctx, rows, emb, writer, services.BackfillOptions{
		Workers:    *workers,
		EmbedBatch: *embedBatch,
		WriteChunk: *writeChunk,
		Limit:      *limit,
		DryRun:     *dryRun,
	})
	if err != nil {
		log.Fatalf("backfill: %v", err)
	}
	log.Printf("completed embedding_v2 backfill: embedded=%d written=%d batches=%d chunks=%d model=%s (%d dims)",
		stats.TotalEmbedded, stats.TotalWritten, stats.Batches, stats.ChunksCommitted, emb.Model(), cfg.GeminiEmbeddingDimension)
}
