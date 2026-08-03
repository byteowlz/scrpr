package extractor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefuddleBackend extracts content via a self-hosted defuddle proxy
// (github.com/kepano/defuddle). Public defuddle.com is a web UI, not a clean
// API, so this backend requires a configured base URL pointing at a running
// instance (e.g. http://localhost:8890).
type DefuddleBackend struct {
	BaseURL string // self-hosted proxy root
	Timeout time.Duration
	client  *http.Client
}

// NewDefuddleBackend creates a defuddle proxy extraction backend.
func NewDefuddleBackend(baseURL string, timeout time.Duration) *DefuddleBackend {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &DefuddleBackend{
		BaseURL: baseURL,
		Timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name returns the backend identifier
func (d *DefuddleBackend) Name() string { return "defuddle" }

// IsAvailable requires a configured proxy base URL
func (d *DefuddleBackend) IsAvailable() bool { return d.BaseURL != "" }

// Extract fetches {BaseURL}/{url} through the defuddle proxy.
func (d *DefuddleBackend) Extract(ctx context.Context, url string, format string) (*ExtractResult, error) {
	proxyURL := d.BaseURL + "/" + url
	req, err := http.NewRequestWithContext(ctx, "GET", proxyURL, nil)
	if err != nil {
		return nil, fmt.Errorf("defuddle: failed to create request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("defuddle: request failed (is the proxy running at %s?): %w", d.BaseURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("defuddle: failed to read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("defuddle: HTTP %d: %s", resp.StatusCode, string(body))
	}

	content := string(body)
	if format == "text" {
		content = stripBasicMarkdown(content)
	}
	return &ExtractResult{URL: url, Content: content}, nil
}
