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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"psearch/serving-go/internal/config"

	"golang.org/x/oauth2/google"
)

// EmbeddingService handles the generation of embeddings via REST API
type EmbeddingService struct {
	config     *config.Config
	httpClient *http.Client // Added httpClient
}

// NewEmbeddingService creates a new embedding service using REST
func NewEmbeddingService(ctx context.Context, cfg *config.Config) (*EmbeddingService, error) {
	// Create an authenticated HTTP client using Application Default Credentials
	// Scopes needed for Vertex AI prediction endpoint
	client, err := google.DefaultClient(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, fmt.Errorf("failed to create default google client for REST API: %v", err)
	}

	return &EmbeddingService{
		config:     cfg,
		httpClient: client,
	}, nil
}

// GenerateEmbedding generates an embedding vector for the provided text using the REST API
func (s *EmbeddingService) GenerateEmbedding(ctx context.Context, text string) ([]float32, error) {
	startTime := time.Now()

	// Construct the API endpoint URL
	url := fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predict",
		s.config.Region,
		s.config.ProjectID,
		s.config.Region,
		s.config.GeminiModelName, // This needs to be the embedding model ID
	)

	// Construct the request body structure matching the REST API
	requestPayload := struct {
		Instances []struct {
			Content  string `json:"content"`
			TaskType string `json:"task_type"` // Note: snake_case in REST API
		} `json:"instances"`
	}{
		Instances: []struct {
			Content  string `json:"content"`
			TaskType string `json:"task_type"`
		}{
			{Content: text, TaskType: "RETRIEVAL_QUERY"}, // Use appropriate task type
		},
	}

	// Marshal the request payload to JSON
	jsonBody, err := json.Marshal(requestPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal REST request body: %v", err)
	}
	log.Printf("DEBUG: Embedding Request Body: %s", string(jsonBody)) // Log request body

	// Create the HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create REST http request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute the request using the authenticated client
	log.Printf("DEBUG: Sending embedding request to %s", url)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute REST http request: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	responseBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read REST response body: %v", err)
	}

	// Check for non-200 status codes
	if resp.StatusCode != http.StatusOK {
		log.Printf("ERROR: Embedding API request failed with status %d: %s", resp.StatusCode, string(responseBodyBytes))
		// Attempt to parse standard Google API error structure
		var apiError struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		if json.Unmarshal(responseBodyBytes, &apiError) == nil && apiError.Error.Message != "" {
			return nil, fmt.Errorf("embedding API error: %s (code %d, status %s)", apiError.Error.Message, apiError.Error.Code, apiError.Error.Status)
		}
		// Fallback error
		return nil, fmt.Errorf("embedding API request failed with status %d", resp.StatusCode)
	}

	// Define the expected response structure
	var responsePayload struct {
		Predictions []struct {
			Embeddings struct {
				Values      []float32 `json:"values"`
				Statistics struct {
					TokenCount         int  `json:"token_count"`
					Truncated          bool `json:"truncated"`
				} `json:"statistics"`
			} `json:"embeddings"`
		} `json:"predictions"`
		// DeployedModelID string `json:"deployedModelId"` // Optional
	}

	// Unmarshal the response JSON
	if err := json.Unmarshal(responseBodyBytes, &responsePayload); err != nil {
		log.Printf("ERROR: Failed to unmarshal embedding response: %s", string(responseBodyBytes))
		return nil, fmt.Errorf("failed to unmarshal REST response body: %v", err)
	}

	// Extract the embedding values
	if len(responsePayload.Predictions) == 0 || len(responsePayload.Predictions[0].Embeddings.Values) == 0 {
		log.Printf("WARN: Embedding response contained no predictions or empty values: %+v", responsePayload)
		return nil, fmt.Errorf("no embeddings returned from REST API")
	}
	embedding := responsePayload.Predictions[0].Embeddings.Values

	// Log the time taken
	elapsed := time.Since(startTime)
	log.Printf("Generated embedding via REST in %s (dimension: %d)", elapsed, len(embedding))

	return embedding, nil
}
