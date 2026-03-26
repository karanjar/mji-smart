// internal/services/ai_client.go
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/karanjar/mji-smart/internal/models"
	"go.uber.org/zap"
)

// HTTPAIClient implements AIClient interface for Python AI service
type HTTPAIClient struct {
	baseURL string
	client  *http.Client
	logger  *zap.Logger
}

// NewHTTPAIClient creates a new HTTP AI client
func NewHTTPAIClient(baseURL string, logger *zap.Logger) *HTTPAIClient {
	return &HTTPAIClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		logger: logger,
	}
}

// VerifyReport sends report to AI service for verification
func (c *HTTPAIClient) VerifyReport(ctx context.Context, report *models.Report) (*VerificationResult, error) {
	// Prepare request
	reqBody := map[string]interface{}{
		"report_id":   report.ID,
		"image_url":   report.ImageURL,
		"description": report.Description,
		"category":    report.Category,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create request
	url := fmt.Sprintf("%s/verify", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result VerificationResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}
