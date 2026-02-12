package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJinaBackend_Name(t *testing.T) {
	b := NewJinaBackend("", 10*time.Second)
	if b.Name() != "jina" {
		t.Errorf("expected 'jina', got %q", b.Name())
	}
}

func TestJinaBackend_IsAvailable(t *testing.T) {
	// Jina is always available (works without API key)
	b := NewJinaBackend("", 10*time.Second)
	if !b.IsAvailable() {
		t.Error("Jina should always be available")
	}

	b = NewJinaBackend("some-key", 10*time.Second)
	if !b.IsAvailable() {
		t.Error("Jina should be available with API key")
	}
}

func TestJinaBackend_Defaults(t *testing.T) {
	b := NewJinaBackend("", 0)
	if b.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", b.Timeout)
	}
	if b.BaseURL != "https://r.jina.ai/" {
		t.Errorf("expected default BaseURL, got %q", b.BaseURL)
	}
}

func newTestJinaBackend(serverURL, apiKey string) *JinaBackend {
	return &JinaBackend{
		APIKey:  apiKey,
		Timeout: 10 * time.Second,
		BaseURL: serverURL + "/",
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func TestJinaBackend_Extract_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}

		// The URL path should contain the target URL
		if !strings.Contains(r.URL.Path, "example.com") {
			t.Errorf("expected path containing 'example.com', got %q", r.URL.Path)
		}

		response := `Title: Example Domain

URL Source: https://example.com

Markdown Content:
# Example Domain

This domain is for use in illustrative examples in documents.`

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	}))
	defer server.Close()

	b := newTestJinaBackend(server.URL, "")
	result, err := b.Extract(context.Background(), "https://example.com", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if result.URL != "https://example.com" {
		t.Errorf("expected URL 'https://example.com', got %q", result.URL)
	}
	if result.Title != "Example Domain" {
		t.Errorf("expected title 'Example Domain', got %q", result.Title)
	}
	if !strings.Contains(result.Content, "Example Domain") {
		t.Errorf("expected content to contain 'Example Domain', got %q", result.Content)
	}
}

func TestJinaBackend_Extract_WithAPIKey(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Write([]byte("Title: Test\n\nContent here"))
	}))
	defer server.Close()

	b := newTestJinaBackend(server.URL, "test-api-key")
	b.Extract(context.Background(), "https://example.com", "markdown")

	if capturedAuth != "Bearer test-api-key" {
		t.Errorf("expected 'Bearer test-api-key', got %q", capturedAuth)
	}
}

func TestJinaBackend_Extract_WithoutAPIKey(t *testing.T) {
	var capturedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Write([]byte("Title: Test\n\nContent here"))
	}))
	defer server.Close()

	b := newTestJinaBackend(server.URL, "")
	b.Extract(context.Background(), "https://example.com", "markdown")

	if capturedAuth != "" {
		t.Errorf("expected no Authorization header, got %q", capturedAuth)
	}
}

func TestJinaBackend_Extract_TextFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `Title: Test Page

Markdown Content:
# Heading

**Bold text** and *italic text*.`
		w.Write([]byte(response))
	}))
	defer server.Close()

	b := newTestJinaBackend(server.URL, "")
	result, err := b.Extract(context.Background(), "https://example.com", "text")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Text format should strip markdown
	if strings.Contains(result.Content, "**") {
		t.Errorf("text format should strip bold markers, got %q", result.Content)
	}
	if strings.Contains(result.Content, "# ") {
		t.Errorf("text format should strip heading markers, got %q", result.Content)
	}
}

func TestJinaBackend_Extract_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limited"))
	}))
	defer server.Close()

	b := newTestJinaBackend(server.URL, "")
	_, err := b.Extract(context.Background(), "https://example.com", "markdown")
	if err == nil {
		t.Fatal("expected error for rate limit")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected rate limit error, got: %v", err)
	}
}

func TestJinaBackend_Extract_AuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	b := newTestJinaBackend(server.URL, "bad-key")
	_, err := b.Extract(context.Background(), "https://example.com", "markdown")
	if err == nil {
		t.Fatal("expected error for auth failure")
	}
	if !strings.Contains(err.Error(), "authentication error") {
		t.Errorf("expected auth error, got: %v", err)
	}
}

func TestJinaBackend_Extract_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	b := newTestJinaBackend(server.URL, "")
	_, err := b.Extract(context.Background(), "https://example.com", "markdown")
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestExtractJinaField(t *testing.T) {
	content := `Title: Example Title
URL Source: https://example.com
Published Time: 2024-01-15

Markdown Content:
Some content here`

	tests := []struct {
		field string
		want  string
	}{
		{"Title:", "Example Title"},
		{"URL Source:", "https://example.com"},
		{"Published Time:", "2024-01-15"},
		{"Nonexistent:", ""},
	}

	for _, tt := range tests {
		if got := extractJinaField(content, tt.field); got != tt.want {
			t.Errorf("extractJinaField(%q) = %q, want %q", tt.field, got, tt.want)
		}
	}
}

func TestExtractJinaMarkdown(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "with marker",
			content: `Title: Test

Markdown Content:
# Heading

Body text here`,
			want: "# Heading\n\nBody text here",
		},
		{
			name:    "without marker - starts with heading",
			content: "Title: Test\nURL: https://test.com\n\n# Content\n\nBody",
			want:    "# Content\n\nBody",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJinaMarkdown(tt.content)
			if strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripBasicMarkdown(t *testing.T) {
	input := "# Heading\n\n**Bold** and __underline__\n\n## Sub heading"
	result := stripBasicMarkdown(input)

	if strings.Contains(result, "#") {
		t.Errorf("should strip heading markers: %q", result)
	}
	if strings.Contains(result, "**") {
		t.Errorf("should strip bold markers: %q", result)
	}
	if strings.Contains(result, "__") {
		t.Errorf("should strip underline markers: %q", result)
	}
	if !strings.Contains(result, "Heading") {
		t.Errorf("should preserve heading text: %q", result)
	}
	if !strings.Contains(result, "Bold") {
		t.Errorf("should preserve bold text: %q", result)
	}
}
