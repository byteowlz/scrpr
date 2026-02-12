package extractor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTavilyBackend_Name(t *testing.T) {
	b := NewTavilyBackend("key", "basic", 10*time.Second)
	if b.Name() != "tavily" {
		t.Errorf("expected 'tavily', got %q", b.Name())
	}
}

func TestTavilyBackend_IsAvailable(t *testing.T) {
	tests := []struct {
		apiKey string
		want   bool
	}{
		{"", false},
		{"tvly-xxx", true},
	}
	for _, tt := range tests {
		b := NewTavilyBackend(tt.apiKey, "basic", 10*time.Second)
		if got := b.IsAvailable(); got != tt.want {
			t.Errorf("IsAvailable(%q) = %v, want %v", tt.apiKey, got, tt.want)
		}
	}
}

func TestTavilyBackend_Defaults(t *testing.T) {
	b := NewTavilyBackend("key", "", 0)
	if b.ExtractDepth != "basic" {
		t.Errorf("expected default extract_depth 'basic', got %q", b.ExtractDepth)
	}
	if b.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", b.Timeout)
	}
	if b.BaseURL != "https://api.tavily.com/extract" {
		t.Errorf("expected default BaseURL, got %q", b.BaseURL)
	}
}

func TestTavilyBackend_Extract_Unavailable(t *testing.T) {
	b := NewTavilyBackend("", "basic", 10*time.Second)
	_, err := b.Extract(context.Background(), "https://example.com", "markdown")
	if err == nil {
		t.Fatal("expected error for unavailable backend")
	}
	if !strings.Contains(err.Error(), "API key not configured") {
		t.Errorf("unexpected error: %v", err)
	}
}

func newTestTavilyExtractor(serverURL, apiKey string) *TavilyBackend {
	return &TavilyBackend{
		APIKey:       apiKey,
		ExtractDepth: "basic",
		Timeout:      10 * time.Second,
		BaseURL:      serverURL,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

func TestTavilyBackend_Extract_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Parse request body
		body, _ := io.ReadAll(r.Body)
		var req tavilyExtractRequest
		json.Unmarshal(body, &req)

		if len(req.URLs) != 1 || req.URLs[0] != "https://example.com" {
			t.Errorf("expected URL 'https://example.com', got %v", req.URLs)
		}

		resp := tavilyExtractResponse{
			Results: []tavilyExtractResult{
				{
					URL:        "https://example.com",
					Title:      "Example Page",
					RawContent: "This is the extracted content.",
				},
			},
			ResponseTime: 1.5,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestTavilyExtractor(server.URL, "test-key")
	result, err := b.Extract(context.Background(), "https://example.com", "text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %q", result.URL)
	}
	if result.Title != "Example Page" {
		t.Errorf("expected title 'Example Page', got %q", result.Title)
	}
	if result.Content != "This is the extracted content." {
		t.Errorf("expected extracted content, got %q", result.Content)
	}
}

func TestTavilyBackend_Extract_MarkdownFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tavilyExtractResponse{
			Results: []tavilyExtractResult{
				{URL: "https://test.com", Title: "Page Title", RawContent: "Body content"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestTavilyExtractor(server.URL, "key")
	result, err := b.Extract(context.Background(), "https://test.com", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if !strings.HasPrefix(result.Content, "# Page Title") {
		t.Errorf("expected markdown heading, got %q", result.Content)
	}
	if !strings.Contains(result.Content, "Body content") {
		t.Errorf("expected body content, got %q", result.Content)
	}
}

func TestTavilyBackend_Extract_FailedURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tavilyExtractResponse{
			Results:    []tavilyExtractResult{},
			FailedURLs: []string{"https://failed.com"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestTavilyExtractor(server.URL, "key")
	_, err := b.Extract(context.Background(), "https://failed.com", "text")
	if err == nil {
		t.Fatal("expected error for failed URL")
	}
	if !strings.Contains(err.Error(), "extraction failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTavilyBackend_Extract_NoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tavilyExtractResponse{Results: []tavilyExtractResult{}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	b := newTestTavilyExtractor(server.URL, "key")
	_, err := b.Extract(context.Background(), "https://empty.com", "text")
	if err == nil {
		t.Fatal("expected error for empty results")
	}
	if !strings.Contains(err.Error(), "no results") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTavilyBackend_Extract_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"detail": "invalid key"}`))
	}))
	defer server.Close()

	b := newTestTavilyExtractor(server.URL, "bad-key")
	_, err := b.Extract(context.Background(), "https://example.com", "text")
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("expected auth error, got: %v", err)
	}
}

func TestTavilyBackend_Extract_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`rate limited`))
	}))
	defer server.Close()

	b := newTestTavilyExtractor(server.URL, "key")
	_, err := b.Extract(context.Background(), "https://example.com", "text")
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected rate limit error, got: %v", err)
	}
}

func TestTavilyBackend_Extract_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not valid json`))
	}))
	defer server.Close()

	b := newTestTavilyExtractor(server.URL, "key")
	_, err := b.Extract(context.Background(), "https://example.com", "text")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
