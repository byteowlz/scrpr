package extractor

import (
	"context"
	"testing"
)

// Verify interfaces are satisfied at compile time
var _ Backend = (*TavilyBackend)(nil)
var _ Backend = (*JinaBackend)(nil)

func TestExtractResult_Fields(t *testing.T) {
	r := ExtractResult{
		URL:     "https://example.com",
		Title:   "Example",
		Content: "Hello, world!",
	}
	if r.URL != "https://example.com" {
		t.Errorf("unexpected URL: %q", r.URL)
	}
	if r.Title != "Example" {
		t.Errorf("unexpected Title: %q", r.Title)
	}
	if r.Content != "Hello, world!" {
		t.Errorf("unexpected Content: %q", r.Content)
	}
}

// mockBackend for testing
type mockExtractorBackend struct {
	name      string
	available bool
	result    *ExtractResult
	err       error
}

func (m *mockExtractorBackend) Name() string      { return m.name }
func (m *mockExtractorBackend) IsAvailable() bool  { return m.available }
func (m *mockExtractorBackend) Extract(ctx context.Context, url string, format string) (*ExtractResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestMockBackend_Interface(t *testing.T) {
	var _ Backend = &mockExtractorBackend{}
}
