package fetcher

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type SimpleFetcher struct {
	client          *http.Client
	userAgentSelect *UserAgentSelector
	uaMemory        *uaMemory
}

func NewSimpleFetcher() *SimpleFetcher {
	return &SimpleFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		userAgentSelect: NewUserAgentSelector(),
	}
}

// SetUAMemory enables per-domain user agent memory persisted at path.
func (sf *SimpleFetcher) SetUAMemory(path string) {
	if path != "" {
		sf.uaMemory = newUAMemory(path)
	}
}

// SetFollowRedirects configures whether the fetcher follows HTTP redirects
func (sf *SimpleFetcher) SetFollowRedirects(follow bool) {
	if !follow {
		sf.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		sf.client.CheckRedirect = nil
	}
}

func (sf *SimpleFetcher) FetchStatic(ctx context.Context, url string, opts FetchOptions) (*FetchResult, error) {
	retryConfig := opts.Retry
	if retryConfig.MaxRetries == 0 {
		retryConfig = DefaultRetryConfig()
	}

	maxSize := opts.MaxResponseSize
	if maxSize == 0 {
		maxSize = defaultMaxResponseSize
	}

	// Bot-UA fallback is disabled when the user pinned an explicit UA.
	botFallback := opts.UserAgent == ""

	var lastErr error
	// fallbackIdx indexes BotFallbackUAs; -1 means use the regular selection.
	fallbackIdx := -1
	host := hostFromURL(url)

	// Skip straight to a UA that worked for this domain before.
	if botFallback && sf.uaMemory != nil && host != "" {
		if idx := indexOfBotUA(sf.uaMemory.Get(host)); idx >= 0 {
			fallbackIdx = idx
		}
	}

	for attempt := 0; attempt <= retryConfig.MaxRetries+len(BotFallbackUAs); attempt++ {
		if attempt > 0 {
			delay := sf.backoffDelay(attempt, retryConfig.BaseDelay, retryConfig.MaxDelay)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("fetch cancelled: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		uaOverride := ""
		if botFallback && fallbackIdx >= 0 && fallbackIdx < len(BotFallbackUAs) {
			uaOverride = BotFallbackUAs[fallbackIdx]
		}

		req, err := sf.buildRequest(ctx, url, opts, uaOverride)
		if err != nil {
			return nil, err
		}

		resp, err := sf.client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to fetch URL: %w", err)
			if retryConfig.RetryOnNetwork && attempt < retryConfig.MaxRetries {
				continue
			}
			return nil, lastErr
		}

		// Handle retryable status codes
		if sf.shouldRetryStatus(resp.StatusCode, retryConfig.RetryStatuses) {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
			if attempt < retryConfig.MaxRetries {
				continue
			}
			return nil, lastErr
		}

		// Cloudflare bot detection: retry with honest UA
		if resp.StatusCode == 403 && resp.Header.Get("Cf-Mitigated") == "challenge" {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP error: %d %s (Cloudflare challenge)", resp.StatusCode, resp.Status)
			if attempt < retryConfig.MaxRetries {
				// Next attempt will use honest UA via buildRequest
				continue
			}
			return nil, lastErr
		}

		// Blocking status: rotate through bot UAs before giving up. Many sites
		// whitelist AI crawler agents while blocking generic browser traffic.
		if botFallback && isBlockingStatus(resp.StatusCode) && fallbackIdx < len(BotFallbackUAs)-1 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
			fallbackIdx++
			continue
		}

		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
		}

		// Check Content-Length before reading body
		if resp.ContentLength > maxSize && maxSize > 0 {
			resp.Body.Close()
			return nil, fmt.Errorf("response too large: %d bytes exceeds limit of %d bytes", resp.ContentLength, maxSize)
		}

		// Read body with size limit
		var body []byte
		var readErr error
		if maxSize > 0 {
			body, readErr = io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
		} else {
			body, readErr = io.ReadAll(resp.Body)
		}
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", readErr)
			if retryConfig.RetryOnNetwork && attempt < retryConfig.MaxRetries {
				continue
			}
			return nil, lastErr
		}

		if maxSize > 0 && int64(len(body)) > maxSize {
			return nil, fmt.Errorf("response too large: exceeds limit of %d bytes", maxSize)
		}

		contentType := resp.Header.Get("Content-Type")
		html := string(body)

		// Remember the bot UA that succeeded for this domain.
		if botFallback && sf.uaMemory != nil && fallbackIdx >= 0 && host != "" {
			sf.uaMemory.Set(host, BotFallbackUAs[fallbackIdx])
		}

		return &FetchResult{
			HTML:        html,
			Title:       sf.extractTitle(html),
			URL:         url,
			UsedJS:      false,
			Metadata:    sf.extractMetadata(html),
			ContentType: contentType,
		}, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("fetch failed after %d attempts", retryConfig.MaxRetries+1)
}

func (sf *SimpleFetcher) buildRequest(ctx context.Context, url string, opts FetchOptions, uaOverride string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Determine user agent
	var userAgent string
	switch {
	case uaOverride != "":
		userAgent = uaOverride
	case opts.UserAgent != "":
		userAgent = opts.UserAgent
	default:
		userAgent = sf.userAgentSelect.GetUserAgent(opts.BrowserAgent)
	}
	req.Header.Set("User-Agent", userAgent)

	// Format-aware Accept header
	req.Header.Set("Accept", sf.acceptHeader(opts.Format))
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	// Don't set Accept-Encoding - let Go's http client handle compression automatically
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Cache-Control", "max-age=0")

	// Add cookies
	for _, cookie := range opts.Cookies {
		req.AddCookie(cookie)
	}

	return req, nil
}

func (sf *SimpleFetcher) acceptHeader(format string) string {
	switch format {
	case "markdown":
		return "text/markdown;q=1.0, text/x-markdown;q=0.9, text/plain;q=0.8, text/html;q=0.7, */*;q=0.1"
	case "text":
		return "text/plain;q=1.0, text/markdown;q=0.9, text/html;q=0.8, */*;q=0.1"
	case "html":
		return "text/html;q=1.0, application/xhtml+xml;q=0.9, text/plain;q=0.8, text/markdown;q=0.7, */*;q=0.1"
	default:
		return "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"
	}
}

// isBlockingStatus reports whether a status code typically indicates
// UA/bot-based blocking rather than a transient failure.
func isBlockingStatus(status int) bool {
	switch status {
	case 401, 402, 403, 429, 999: // 999 is LinkedIn's non-standard deny code
		return true
	}
	return false
}

// indexOfBotUA returns the index of ua in BotFallbackUAs, or -1.
func indexOfBotUA(ua string) int {
	for i, candidate := range BotFallbackUAs {
		if candidate == ua {
			return i
		}
	}
	return -1
}

func (sf *SimpleFetcher) shouldRetryStatus(status int, retryStatuses []int) bool {
	for _, s := range retryStatuses {
		if status == s {
			return true
		}
	}
	return false
}

func (sf *SimpleFetcher) backoffDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if baseDelay == 0 {
		baseDelay = 1 * time.Second
	}
	if maxDelay == 0 {
		maxDelay = 30 * time.Second
	}
	// Exponential backoff: baseDelay * 2^attempt
	delay := baseDelay * time.Duration(1<<attempt)
	// Add jitter: ±25%
	jitter := time.Duration(float64(delay) * (0.75 + 0.5*rand.Float64()))
	if jitter > maxDelay {
		jitter = maxDelay
	}
	return jitter
}

func (sf *SimpleFetcher) extractTitle(html string) string {
	lowerHTML := strings.ToLower(html)
	titleStart := strings.Index(lowerHTML, "<title")
	if titleStart == -1 {
		return ""
	}

	start := strings.Index(html[titleStart:], ">")
	if start == -1 {
		return ""
	}
	start += titleStart + 1

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
