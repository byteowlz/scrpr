package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SimpleFetcher struct {
	client *http.Client
}

func NewSimpleFetcher() *SimpleFetcher {
	return &SimpleFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (sf *SimpleFetcher) FetchStatic(ctx context.Context, url string, opts FetchOptions) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if opts.UserAgent != "" {
		req.Header.Set("User-Agent", opts.UserAgent)
	} else {
		req.Header.Set("User-Agent", "scrpr/1.0")
	}

	// Add cookies
	for _, cookie := range opts.Cookies {
		req.AddCookie(cookie)
	}

	resp, err := sf.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	html := string(body)

	return &FetchResult{
		HTML:     html,
		Title:    sf.extractTitle(html),
		URL:      url,
		UsedJS:   false,
		Metadata: sf.extractMetadata(html),
	}, nil
}

func (sf *SimpleFetcher) extractTitle(html string) string {
	lowerHTML := strings.ToLower(html)
	start := strings.Index(lowerHTML, "<title")
	if start == -1 {
		return ""
	}

	start = strings.Index(html[start:], ">")
	if start == -1 {
		return ""
	}
	start += start + 1

	end := strings.Index(strings.ToLower(html[start:]), "</title>")
	if end == -1 {
		return ""
	}

	return strings.TrimSpace(html[start : start+end])
}

func (sf *SimpleFetcher) extractMetadata(html string) map[string]string {
	metadata := make(map[string]string)

	// Extract meta tags
	metaTags := []struct {
		name string
		attr string
	}{
		{"author", "author"},
		{"description", "description"},
		{"keywords", "keywords"},
		{"date", "date"},
		{"published", "article:published_time"},
		{"modified", "article:modified_time"},
	}

	for _, tag := range metaTags {
		if value := sf.findMetaContent(html, tag.attr); value != "" {
			metadata[tag.name] = value
		}
	}

	// Extract Open Graph tags
	ogTags := []string{"og:title", "og:description", "og:image", "og:url", "og:type"}
	for _, tag := range ogTags {
		if value := sf.findMetaContent(html, tag); value != "" {
			metadata[strings.TrimPrefix(tag, "og:")] = value
		}
	}

	return metadata
}

func (sf *SimpleFetcher) findMetaContent(html, property string) string {
	patterns := []string{
		fmt.Sprintf(`name="%s"`, property),
		fmt.Sprintf(`property="%s"`, property),
		fmt.Sprintf(`name='%s'`, property),
		fmt.Sprintf(`property='%s'`, property),
	}

	lowerHTML := strings.ToLower(html)

	for _, pattern := range patterns {
		if idx := strings.Index(lowerHTML, pattern); idx != -1 {
			// Find the content attribute
			metaStart := strings.LastIndex(lowerHTML[:idx], "<meta")
			if metaStart == -1 {
				continue
			}

			metaEnd := strings.Index(lowerHTML[idx:], ">")
			if metaEnd == -1 {
				continue
			}
			metaEnd += idx

			metaTag := html[metaStart:metaEnd]

			// Extract content value
			contentStart := strings.Index(strings.ToLower(metaTag), `content="`)
			if contentStart == -1 {
				contentStart = strings.Index(strings.ToLower(metaTag), `content='`)
				if contentStart == -1 {
					continue
				}
				contentStart += 9 // len(`content='`)
			} else {
				contentStart += 9 // len(`content="`)
			}

			quote := metaTag[contentStart-1]
			contentEnd := strings.IndexByte(metaTag[contentStart:], quote)
			if contentEnd == -1 {
				continue
			}

			return strings.TrimSpace(metaTag[contentStart : contentStart+contentEnd])
		}
	}

	return ""
}
