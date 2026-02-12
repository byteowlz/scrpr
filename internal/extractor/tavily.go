package extractor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TavilyBackend extracts content using Tavily Extract API
type TavilyBackend struct {
	APIKey       string
	ExtractDepth string // "basic" or "advanced"
	Timeout      time.Duration
	BaseURL      string // overridable for testing
	client       *http.Client
}

// NewTavilyBackend creates a new Tavily extraction backend
func NewTavilyBackend(apiKey, extractDepth string, timeout time.Duration) *TavilyBackend {
	if extractDepth == "" {
		extractDepth = "basic"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &TavilyBackend{
		APIKey:       apiKey,
		ExtractDepth: extractDepth,
		Timeout:      timeout,
		BaseURL:      "https://api.tavily.com/extract",
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the backend identifier
func (t *TavilyBackend) Name() string {
	return "tavily"
}

// IsAvailable checks if the Tavily API key is configured
func (t *TavilyBackend) IsAvailable() bool {
	return t.APIKey != ""
}

// tavilyExtractRequest is the POST body for Tavily extract
type tavilyExtractRequest struct {
	URLs         []string `json:"urls"`
	ExtractDepth string   `json:"extract_depth,omitempty"`
}

// tavilyExtractResponse is the Tavily extract API response
type tavilyExtractResponse struct {
	Results      []tavilyExtractResult `json:"results"`
	FailedURLs   []string              `json:"failed_results"`
	ResponseTime float64               `json:"response_time"`
}

type tavilyExtractResult struct {
	URL        string `json:"url"`
	RawContent string `json:"raw_content"`
	Title      string `json:"title"`
}

// Extract fetches and extracts content from a URL using Tavily
func (t *TavilyBackend) Extract(ctx context.Context, url string, format string) (*ExtractResult, error) {
	if !t.IsAvailable() {
		return nil, fmt.Errorf("tavily: API key not configured")
	}

	reqBody := tavilyExtractRequest{
		URLs:         []string{url},
		ExtractDepth: t.ExtractDepth,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tavily: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.BaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("tavily: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.APIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tavily: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case 401, 403:
			return nil, fmt.Errorf("tavily: authentication failed: %s", string(respBody))
		case 429:
			return nil, fmt.Errorf("tavily: rate limited: %s", string(respBody))
		default:
			return nil, fmt.Errorf("tavily: HTTP %d: %s", resp.StatusCode, string(respBody))
		}
	}

	var tavilyResp tavilyExtractResponse
	if err := json.Unmarshal(respBody, &tavilyResp); err != nil {
		return nil, fmt.Errorf("tavily: failed to parse response: %w", err)
	}

	if len(tavilyResp.Results) == 0 {
		if len(tavilyResp.FailedURLs) > 0 {
			return nil, fmt.Errorf("tavily: extraction failed for %s", url)
		}
		return nil, fmt.Errorf("tavily: no results returned for %s", url)
	}

	result := tavilyResp.Results[0]
	content := result.RawContent

	// Add title as markdown heading if format is markdown
	if format == "markdown" && result.Title != "" {
		content = fmt.Sprintf("# %s\n\n%s", result.Title, content)
	}

	return &ExtractResult{
		URL:     result.URL,
		Title:   result.Title,
		Content: content,
	}, nil
}
