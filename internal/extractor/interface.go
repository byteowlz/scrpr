package extractor

import "context"

// ExtractResult holds the output of a content extraction
type ExtractResult struct {
	URL     string
	Title   string
	Content string // Extracted content (plain text or markdown depending on backend)
}

// Backend is the interface for content extraction backends
type Backend interface {
	// Name returns the unique identifier for this backend
	Name() string

	// Extract fetches and extracts content from a URL
	Extract(ctx context.Context, url string, format string) (*ExtractResult, error)

	// IsAvailable checks if the backend is properly configured
	IsAvailable() bool
}
