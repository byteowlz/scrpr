package extractor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JinaBackend extracts content using Jina Reader API (r.jina.ai)
type JinaBackend struct {
	APIKey  string // Optional - works without auth but with rate limits
	Timeout time.Duration
	client  *http.Client
}

// NewJinaBackend creates a new Jina Reader extraction backend
func NewJinaBackend(apiKey string, timeout time.Duration) *JinaBackend {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &JinaBackend{
		APIKey:  apiKey,
		Timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the backend identifier
func (j *JinaBackend) Name() string {
	return "jina"
}

// IsAvailable always returns true - Jina Reader works without an API key
func (j *JinaBackend) IsAvailable() bool {
	return true
}

// Extract fetches and extracts content from a URL using Jina Reader
func (j *JinaBackend) Extract(ctx context.Context, url string, format string) (*ExtractResult, error) {
	// Jina Reader: GET https://r.jina.ai/{URL}
	jinaURL := "https://r.jina.ai/" + url

	req, err := http.NewRequestWithContext(ctx, "GET", jinaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("jina: failed to create request: %w", err)
	}

	// Request markdown output
	req.Header.Set("Accept", "text/plain")

	// Add API key if available (higher rate limits)
	if j.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+j.APIKey)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jina: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("jina: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case 401, 403:
			return nil, fmt.Errorf("jina: authentication error: %s", string(body))
		case 429:
			return nil, fmt.Errorf("jina: rate limited - consider adding an API key")
		default:
			return nil, fmt.Errorf("jina: HTTP %d: %s", resp.StatusCode, string(body))
		}
	}

	content := string(body)

	// Parse the Jina response - it returns structured text with Title:, URL Source:, Markdown Content:
	title := extractJinaField(content, "Title:")
	markdownContent := extractJinaMarkdown(content)

	if markdownContent == "" {
		markdownContent = content
	}

	// For text format, strip markdown formatting
	finalContent := markdownContent
	if format == "text" {
		finalContent = stripBasicMarkdown(markdownContent)
	}

	return &ExtractResult{
		URL:     url,
		Title:   title,
		Content: finalContent,
	}, nil
}

// extractJinaField extracts a named field from Jina's response
func extractJinaField(content, field string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, field) {
			return strings.TrimSpace(strings.TrimPrefix(line, field))
		}
	}
	return ""
}

// extractJinaMarkdown extracts the markdown content section from Jina's response
func extractJinaMarkdown(content string) string {
	marker := "Markdown Content:"
	idx := strings.Index(content, marker)
	if idx == -1 {
		// Try without the header section - just find the first heading or paragraph after metadata
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "#") || (i > 3 && strings.TrimSpace(line) != "" && !strings.Contains(line, ":")) {
				return strings.Join(lines[i:], "\n")
			}
		}
		return content
	}
	return strings.TrimSpace(content[idx+len(marker):])
}

// stripBasicMarkdown removes basic markdown formatting for plain text output
func stripBasicMarkdown(md string) string {
	lines := strings.Split(md, "\n")
	var result []string
	for _, line := range lines {
		// Remove heading markers
		trimmed := line
		for strings.HasPrefix(trimmed, "#") {
			trimmed = strings.TrimPrefix(trimmed, "#")
		}
		trimmed = strings.TrimSpace(trimmed)

		// Remove bold/italic markers
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")

		result = append(result, trimmed)
	}
	return strings.Join(result, "\n")
}
