package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
	"time"
)

// YtScrapeBackend extracts real content from YouTube search results pages
// (/results?search_query=...) by pulling the ytInitialData JSON blob that
// YouTube embeds in the page's raw HTML. The results list is JS-rendered in the
// DOM, so readability/Jina only recover an empty shell — but the video titles
// are serialized server-side in ytInitialData and are available to any client
// that fetches the page with a desktop browser user agent. No key required.
type YtScrapeBackend struct {
	Timeout time.Duration
	client  *http.Client
}

// NewYtScrapeBackend creates a YouTube search extraction backend.
func NewYtScrapeBackend(timeout time.Duration) *YtScrapeBackend {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &YtScrapeBackend{
		Timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

// Name returns the backend identifier.
func (y *YtScrapeBackend) Name() string { return "ytscrape" }

// IsAvailable always returns true - YouTube search needs no key.
func (y *YtScrapeBackend) IsAvailable() bool { return true }

// ytSearchRe matches YouTube search results URLs (www/m/music etc).
var ytSearchRe = regexp.MustCompile(`(?i)^https?://(?:www\.|m\.|music\.)?youtube\.com/results\?`)

// IsYouTubeSearchURL reports whether url is a YouTube search results page.
func IsYouTubeSearchURL(url string) bool {
	return ytSearchRe.MatchString(url)
}

// desktopUA is a generic desktop Chrome agent; YouTube serves the full,
// content-bearing search page (including ytInitialData) to it but a
// reduced/empty shell to script-like agents.
const ytsDesktopUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// Extract fetches a YouTube search page and returns the list of video results.
func (y *YtScrapeBackend) Extract(ctx context.Context, url, format string) (*ExtractResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("ytscrape: failed to create request: %w", err)
	}
	setBrowserHeaders(req)

	resp, err := y.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ytscrape: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ytscrape: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("ytscrape: failed to read body: %w", err)
	}

	data, err := parseYtInitialData(string(body))
	if err != nil {
		return nil, err
	}

	// Pull a human header from the search query when present.
	queryTitle := ""
	if q, perr := neturl.Parse(url); perr == nil {
		queryTitle = strings.TrimSpace(q.Query().Get("search_query"))
	}

	var lines []string
	if queryTitle != "" {
		lines = append(lines, "YouTube search results for: "+queryTitle)
		lines = append(lines, "")
	}

	count := 0
	walkYtJSON(data, func(title string) {
		count++
		lines = append(lines, title)
	})

	if count == 0 {
		return nil, fmt.Errorf("ytscrape: no video results found in page")
	}

	content := strings.Join(lines, "\n")
	if format == "markdown" {
		content = strings.Join(lines, "\n\n")
	}

	return &ExtractResult{
		URL:     url,
		Title:   queryTitle,
		Content: content,
	}, nil
}

// setBrowserHeaders fills a request with headers that make YouTube treat the
// client as a real desktop browser (avoids the consent/empty shell).
func setBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", ytsDesktopUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
}

// parseYtInitialData extracts and decodes the ytInitialData JSON blob from a
// YouTube HTML body. On www it is a raw JSON object literal; on the mobile
// site it is escaped as a single-quoted string. Both forms are handled.
func parseYtInitialData(body string) (interface{}, error) {
	i := strings.Index(body, "ytInitialData")
	if i < 0 {
		return nil, fmt.Errorf("ytscrape: ytInitialData not found in page")
	}
	eq := strings.Index(body[i:], "=")
	if eq < 0 {
		return nil, fmt.Errorf("ytscrape: malformed ytInitialData assignment")
	}
	start := i + eq + 1
	for start < len(body) && (body[start] == ' ' || body[start] == '\n' || body[start] == '\t') {
		start++
	}
	if start >= len(body) {
		return nil, fmt.Errorf("ytscrape: empty ytInitialData")
	}

	raw := ""
	var err error
	if body[start] == '\'' {
		// Escaped string form (mobile): find closing quote, unescape.
		end := strings.Index(body[start+1:], "'")
		if end < 0 {
			return nil, fmt.Errorf("ytscrape: unterminated ytInitialData string")
		}
		s := body[start+1 : start+1+end]
		s = strings.ReplaceAll(s, `\x`, `\u00`)
		var urlErr error
		dec := json.NewDecoder(strings.NewReader(`"` + s + `"`))
		if urlErr = dec.Decode(&raw); urlErr != nil {
			return nil, fmt.Errorf("ytscrape: failed to unescape ytInitialData: %w", urlErr)
		}
	} else if body[start] == '{' {
		end := findBalanced(body, start)
		if end <= start {
			return nil, fmt.Errorf("ytscrape: unbalanced ytInitialData object")
		}
		raw = body[start : end+1]
	} else {
		return nil, fmt.Errorf("ytscrape: unexpected ytInitialData marker")
	}

	var data interface{}
	if err = json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("ytscrape: failed to parse ytInitialData: %w", err)
	}
	return data, nil
}

// findBalanced returns the index of the closing brace for the object that
// opens at start (start must point at '{'). Ignores braces inside strings.
func findBalanced(s string, start int) int {
	depth := 0
	inStr := false
	esc := false
	for j := start; j < len(s); j++ {
		c := s[j]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// walkYtJSON walks decoded ytInitialData and calls fn with each video title in
// document order.
func walkYtJSON(v interface{}, fn func(string)) {
	switch t := v.(type) {
	case map[string]interface{}:
		if vr, ok := t["videoRenderer"].(map[string]interface{}); ok {
			if title := ytVideoTitle(vr); title != "" {
				fn(title)
			}
		}
		for _, child := range t {
			walkYtJSON(child, fn)
		}
	case []interface{}:
		for _, child := range t {
			walkYtJSON(child, fn)
		}
	}
}

// ytVideoTitle reconstructs a video's title from its title.runs[].text.
func ytVideoTitle(vr map[string]interface{}) string {
	ti, ok := vr["title"].(map[string]interface{})
	if !ok {
		return ""
	}
	runs, _ := ti["runs"].([]interface{})
	var b strings.Builder
	for _, r := range runs {
		if rm, ok := r.(map[string]interface{}); ok {
			if txt, ok := rm["text"].(string); ok {
				b.WriteString(txt)
			}
		}
	}
	return strings.TrimSpace(b.String())
}
