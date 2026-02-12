package fetcher

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

type FetchMode string

const (
	FetchModeAuto   FetchMode = "auto"
	FetchModeStatic FetchMode = "static"
	FetchModeJS     FetchMode = "javascript"
)

type FetchOptions struct {
	Mode            FetchMode
	Timeout         time.Duration
	UserAgent       string
	BrowserAgent    string
	Cookies         []*http.Cookie
	SkipBanners     bool
	BannerTimeout   time.Duration
	WaitForSelector string
}

type FetchResult struct {
	HTML     string
	Title    string
	URL      string
	UsedJS   bool
	Metadata map[string]string
}

type ContentFetcher struct {
	client          *http.Client
	userAgentSelect *UserAgentSelector
}

func NewContentFetcher() *ContentFetcher {
	return &ContentFetcher{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		userAgentSelect: NewUserAgentSelector(),
	}
}

func (cf *ContentFetcher) Fetch(ctx context.Context, url string, opts FetchOptions) (*FetchResult, error) {
	if opts.Mode == FetchModeStatic {
		return cf.fetchStatic(ctx, url, opts)
	}

	if opts.Mode == FetchModeJS {
		return cf.fetchWithJS(ctx, url, opts)
	}

	// Auto mode: try static first, then JS if needed
	result, err := cf.fetchStatic(ctx, url, opts)
	if err != nil {
		return nil, err
	}

	if cf.needsJSRendering(result.HTML) {
		return cf.fetchWithJS(ctx, url, opts)
	}

	return result, nil
}

func (cf *ContentFetcher) fetchStatic(ctx context.Context, url string, opts FetchOptions) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent (custom takes precedence, then browser agent, then random)
	userAgent := opts.UserAgent
	if userAgent == "" {
		// Use browser agent selector if no custom user agent specified
		userAgent = cf.userAgentSelect.GetUserAgent(opts.BrowserAgent)
	}
	req.Header.Set("User-Agent", userAgent)

	// Add headers that make the request look more like a real browser
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
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

	resp, err := cf.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, resp.Status)
	}

	// Read response body
	buf := make([]byte, 1024*1024) // 1MB buffer
	n, err := resp.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	html := string(buf[:n])

	return &FetchResult{
		HTML:     html,
		Title:    cf.extractTitle(html),
		URL:      url,
		UsedJS:   false,
		Metadata: cf.extractMetadata(html),
	}, nil
}

func (cf *ContentFetcher) fetchWithJS(ctx context.Context, url string, opts FetchOptions) (*FetchResult, error) {
	// Create Chrome context
	chromeCtx, cancel := chromedp.NewContext(ctx)
	defer cancel()

	// Set timeout
	if opts.Timeout > 0 {
		chromeCtx, cancel = context.WithTimeout(chromeCtx, opts.Timeout)
		defer cancel()
	}

	var html, title string
	var err error

	tasks := []chromedp.Action{
		chromedp.Navigate(url),
	}

	// Add cookies if provided
	if len(opts.Cookies) > 0 {
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			for _, _ = range opts.Cookies {
				// TODO: Implement cookie setting with proper cdproto API
				// For now, skip cookie setting as the API requires cdproto conversion
			}
			return nil
		}))
		// Navigate again after setting cookies
		tasks = append(tasks, chromedp.Navigate(url))
	}

	// Dismiss cookie banners if enabled
	if opts.SkipBanners {
		tasks = append(tasks, cf.dismissCookieBanners(opts.BannerTimeout)...)
	}

	// Wait for specific selector if provided
	if opts.WaitForSelector != "" {
		tasks = append(tasks, chromedp.WaitVisible(opts.WaitForSelector))
	} else {
		// Default wait for document ready
		tasks = append(tasks, chromedp.WaitReady("body"))
	}

	// Extract content
	tasks = append(tasks,
		chromedp.OuterHTML("html", &html),
		chromedp.Title(&title),
	)

	if err = chromedp.Run(chromeCtx, tasks...); err != nil {
		return nil, fmt.Errorf("failed to run Chrome tasks: %w", err)
	}

	return &FetchResult{
		HTML:     html,
		Title:    title,
		URL:      url,
		UsedJS:   true,
		Metadata: cf.extractMetadata(html),
	}, nil
}

func (cf *ContentFetcher) dismissCookieBanners(timeout time.Duration) []chromedp.Action {
	bannerSelectors := []string{
		`[id*="cookie"]`,
		`[class*="cookie"]`,
		`[id*="consent"]`,
		`[class*="consent"]`,
		`[id*="gdpr"]`,
		`[class*="gdpr"]`,
		`.cookie-banner`,
		`.consent-banner`,
		`#cookieConsent`,
		`#cookie-notice`,
		`[role="dialog"]`,
		`.modal`,
	}

	acceptSelectors := []string{
		`button[id*="accept"]`,
		`button[class*="accept"]`,
		`.cookie-accept`,
		`[data-action="accept"]`,
		`button:contains("Accept")`,
		`button:contains("OK")`,
		`button:contains("Agree")`,
		`button:contains("Allow")`,
	}

	var tasks []chromedp.Action

	// Wait a bit for banners to appear
	tasks = append(tasks, chromedp.Sleep(1*time.Second))

	// Try to find and dismiss banners
	for _, selector := range bannerSelectors {
		_ = selector // used in close selectors below when chromedp API is fixed
		tasks = append(tasks, chromedp.ActionFunc(func(ctx context.Context) error {
			// TODO: Fix chromedp API usage
			// Check if banner exists - temporarily disabled
			return nil
		}))
	}

	// NOTE: The following banner dismissal logic is disabled pending chromedp API fixes.
	// When re-enabled, it should iterate bannerSelectors and try accept/close buttons.
	_ = acceptSelectors

	return tasks
}

func (cf *ContentFetcher) needsJSRendering(html string) bool {
	lowerHTML := strings.ToLower(html)

	// Check for SPA frameworks
	jsFrameworks := []string{
		"react", "vue", "angular", "backbone", "ember",
		"data-reactroot", "ng-app", "v-app",
	}

	for _, framework := range jsFrameworks {
		if strings.Contains(lowerHTML, framework) {
			return true
		}
	}

	// Check for minimal content with loading indicators
	if strings.Contains(lowerHTML, "loading") && len(strings.TrimSpace(html)) < 2000 {
		return true
	}

	// Check for heavy script usage
	scriptCount := strings.Count(lowerHTML, "<script")
	bodyContent := cf.extractBodyContent(html)

	if scriptCount > 5 && len(strings.TrimSpace(bodyContent)) < 1000 {
		return true
	}

	return false
}

func (cf *ContentFetcher) extractTitle(html string) string {
	start := strings.Index(strings.ToLower(html), "<title")
	if start == -1 {
		return ""
	}

	start = strings.Index(html[start:], ">")
	if start == -1 {
		return ""
	}
	start += start

	end := strings.Index(strings.ToLower(html[start:]), "</title>")
	if end == -1 {
		return ""
	}

	return strings.TrimSpace(html[start : start+end])
}

func (cf *ContentFetcher) extractBodyContent(html string) string {
	lowerHTML := strings.ToLower(html)
	start := strings.Index(lowerHTML, "<body")
	if start == -1 {
		return html
	}

	start = strings.Index(html[start:], ">")
	if start == -1 {
		return html
	}
	start += start + 1

	end := strings.Index(lowerHTML[start:], "</body>")
	if end == -1 {
		return html[start:]
	}

	return html[start : start+end]
}

func (cf *ContentFetcher) extractMetadata(html string) map[string]string {
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
		if value := cf.findMetaContent(html, tag.attr); value != "" {
			metadata[tag.name] = value
		}
	}

	// Extract Open Graph tags
	ogTags := []string{"og:title", "og:description", "og:image", "og:url", "og:type"}
	for _, tag := range ogTags {
		if value := cf.findMetaContent(html, tag); value != "" {
			metadata[strings.TrimPrefix(tag, "og:")] = value
		}
	}

	return metadata
}

func (cf *ContentFetcher) findMetaContent(html, property string) string {
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
