package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAcceptHeader(t *testing.T) {
	sf := NewSimpleFetcher()

	tests := []struct {
		format   string
		expected string
	}{
		{
			format:   "markdown",
			expected: "text/markdown;q=1.0, text/x-markdown;q=0.9, text/plain;q=0.8, text/html;q=0.7, */*;q=0.1",
		},
		{
			format:   "text",
			expected: "text/plain;q=1.0, text/markdown;q=0.9, text/html;q=0.8, */*;q=0.1",
		},
		{
			format:   "html",
			expected: "text/html;q=1.0, application/xhtml+xml;q=0.9, text/plain;q=0.8, text/markdown;q=0.7, */*;q=0.1",
		},
		{
			format:   "",
			expected: "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got := sf.acceptHeader(tt.format)
			if got != tt.expected {
				t.Errorf("acceptHeader(%q) = %q, want %q", tt.format, got, tt.expected)
			}
		})
	}
}

func TestFetchStatic_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			t.Errorf("expected Accept header to contain text/html, got %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><head><title>Test Page</title></head><body><p>Hello World</p></body></html>`)
	}))
	defer server.Close()

	sf := NewSimpleFetcher()
	ctx := context.Background()

	result, err := sf.FetchStatic(ctx, server.URL, FetchOptions{Format: "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Title != "Test Page" {
		t.Errorf("expected title 'Test Page', got %q", result.Title)
	}
	if !strings.Contains(result.HTML, "Hello World") {
		t.Errorf("expected HTML to contain 'Hello World', got %q", result.HTML)
	}
	if result.ContentType != "text/html; charset=utf-8" {
		t.Errorf("expected content type 'text/html; charset=utf-8', got %q", result.ContentType)
	}
}

func TestFetchStatic_MarkdownAcceptHeader(t *testing.T) {
	var acceptHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		acceptHeader = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>test</body></html>`)
	}))
	defer server.Close()

	sf := NewSimpleFetcher()
	ctx := context.Background()

	_, err := sf.FetchStatic(ctx, server.URL, FetchOptions{Format: "markdown"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(acceptHeader, "text/markdown") {
		t.Errorf("expected Accept header to contain text/markdown, got %q", acceptHeader)
	}
	if !strings.Contains(acceptHeader, "q=1.0") {
		t.Errorf("expected Accept header to have markdown q=1.0, got %q", acceptHeader)
	}
}

func TestFetchStatic_RetryOn429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body>success after retry</body></html>`)
	}))
	defer server.Close()

	sf := NewSimpleFetcher()
	ctx := context.Background()

	result, err := sf.FetchStatic(ctx, server.URL, FetchOptions{
		Format: "text",
		Retry: RetryConfig{
			MaxRetries:     3,
			BaseDelay:      50 * time.Millisecond,
			RetryStatuses:  []int{429},
			RetryOnNetwork: true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if !strings.Contains(result.HTML, "success after retry") {
		t.Errorf("expected success content, got %q", result.HTML)
	}
}

func TestFetchStatic_RetryExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	sf := NewSimpleFetcher()
	ctx := context.Background()

	_, err := sf.FetchStatic(ctx, server.URL, FetchOptions{
		Format: "text",
		Retry: RetryConfig{
			MaxRetries:     2,
			BaseDelay:      10 * time.Millisecond,
			RetryStatuses:  []int{429},
			RetryOnNetwork: true,
		},
	})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}

	if attempts != 3 { // initial + 2 retries
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestFetchStatic_SizeLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Content-Length", "10485760") // 10MB
		fmt.Fprint(w, strings.Repeat("x", 100))
	}))
	defer server.Close()

	sf := NewSimpleFetcher()
	ctx := context.Background()

	_, err := sf.FetchStatic(ctx, server.URL, FetchOptions{
		Format:          "text",
		MaxResponseSize: 5 << 20, // 5MB
	})
	if err == nil {
		t.Fatal("expected error for oversized content-length")
	}
	if !strings.Contains(err.Error(), "response too large") {
		t.Errorf("expected 'response too large' error, got %v", err)
	}
}

func TestFetchStatic_SizeLimitByBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		// No Content-Length header, body will be limited by reader
		fmt.Fprint(w, strings.Repeat("x", 100))
	}))
	defer server.Close()

	sf := NewSimpleFetcher()
	ctx := context.Background()

	// Should succeed since body is only 100 bytes
	result, err := sf.FetchStatic(ctx, server.URL, FetchOptions{
		Format:          "text",
		MaxResponseSize: 5 << 20, // 5MB
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.HTML) != 100 {
		t.Errorf("expected 100 bytes, got %d", len(result.HTML))
	}
}

func TestFetchStatic_ImageContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	}))
	defer server.Close()

	sf := NewSimpleFetcher()
	ctx := context.Background()

	result, err := sf.FetchStatic(ctx, server.URL, FetchOptions{Format: "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ContentType != "image/png" {
		t.Errorf("expected content type 'image/png', got %q", result.ContentType)
	}
}

func TestBackoffDelay(t *testing.T) {
	sf := NewSimpleFetcher()

	// Test that delay increases with attempt
	delay1 := sf.backoffDelay(1, 100*time.Millisecond, 10*time.Second)
	delay2 := sf.backoffDelay(2, 100*time.Millisecond, 10*time.Second)

	if delay1 >= delay2 {
		t.Errorf("expected delay2 > delay1, got delay1=%v, delay2=%v", delay1, delay2)
	}

	// Test max delay cap
	delay := sf.backoffDelay(10, 1*time.Second, 5*time.Second)
	if delay > 5*time.Second {
		t.Errorf("expected delay <= 5s, got %v", delay)
	}

	// Test jitter is applied (delay should vary)
	delays := make(map[time.Duration]bool)
	for i := 0; i < 20; i++ {
		d := sf.backoffDelay(1, 1*time.Second, 10*time.Second)
		delays[d] = true
	}
	if len(delays) < 5 {
		t.Errorf("expected jitter to produce varied delays, got %d unique values", len(delays))
	}
}

func TestShouldRetryStatus(t *testing.T) {
	sf := NewSimpleFetcher()
	statuses := []int{429, 502, 503}

	if !sf.shouldRetryStatus(429, statuses) {
		t.Error("expected 429 to be retryable")
	}
	if !sf.shouldRetryStatus(503, statuses) {
		t.Error("expected 503 to be retryable")
	}
	if sf.shouldRetryStatus(404, statuses) {
		t.Error("expected 404 to not be retryable")
	}
}
