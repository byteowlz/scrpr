package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestUAMemoryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "scrpr", "ua-memory.json")

	m1 := newUAMemory(path)
	if m1.Get("example.com") != "" {
		t.Fatal("expected empty memory")
	}
	m1.Set("example.com", BotFallbackUAs[0])

	m2 := newUAMemory(path)
	if got := m2.Get("example.com"); got != BotFallbackUAs[0] {
		t.Errorf("persisted value = %q, want %q", got, BotFallbackUAs[0])
	}
}

func TestFetchStatic_UsesRememberedUAFirst(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		seen = append(seen, ua)
		if ua == BotFallbackUAs[1] { // "OpenAI File Downloader"
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><body><p>ok</p></body></html>`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "ua-memory.json")

	sf := NewSimpleFetcher()
	sf.SetUAMemory(path)
	result, err := sf.FetchStatic(context.Background(), server.URL, FetchOptions{
		Mode:  FetchModeStatic,
		Retry: RetryConfig{RetryOnNetwork: false},
	})
	if err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if !strings.Contains(result.HTML, "ok") {
		t.Errorf("unexpected body: %q", result.HTML)
	}
	firstRun := len(seen)

	// Second run must start directly with the remembered UA.
	sf2 := NewSimpleFetcher()
	sf2.SetUAMemory(path)
	if _, err := sf2.FetchStatic(context.Background(), server.URL, FetchOptions{
		Mode:  FetchModeStatic,
		Retry: RetryConfig{RetryOnNetwork: false},
	}); err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if seen[firstRun] != BotFallbackUAs[1] {
		t.Errorf("second run first UA = %q, want remembered %q", seen[firstRun], BotFallbackUAs[1])
	}
}

func TestHostFromURL(t *testing.T) {
	if got := hostFromURL("https://sub.example.com/x?y=1"); got != "sub.example.com" {
		t.Errorf("hostFromURL = %q", got)
	}
	if got := hostFromURL("::::"); got != "" {
		t.Errorf("invalid url host = %q, want empty", got)
	}
}
