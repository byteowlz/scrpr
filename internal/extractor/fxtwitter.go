package extractor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// FxtwitterBackend extracts tweet text from X/Twitter URLs using the public
// fxtwitter API (api.fxtwitter.com). vxtwitter.com is the same service, so a
// single implementation serves both backend names.
type FxtwitterBackend struct {
	NameID  string // "fxtwitter" or "vxtwitter"
	Timeout time.Duration
	BaseURL string // overridable for testing (default: https://api.fxtwitter.com)
	client  *http.Client
}

// NewFxtwitterBackend creates a fxtwitter/vxtwitter extraction backend.
// name selects the reported backend id ("fxtwitter" or "vxtwitter").
func NewFxtwitterBackend(name string, timeout time.Duration) *FxtwitterBackend {
	if name == "" {
		name = "fxtwitter"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &FxtwitterBackend{
		NameID:  name,
		Timeout: timeout,
		BaseURL: "https://api.fxtwitter.com",
		client:  &http.Client{Timeout: timeout},
	}
}

// Name returns the backend identifier (fxtwitter or vxtwitter)
func (f *FxtwitterBackend) Name() string { return f.NameID }

// IsAvailable always returns true - the fxtwitter API needs no key
func (f *FxtwitterBackend) IsAvailable() bool { return true }

// tweetURLRe matches x.com/twitter.com status URLs.
var tweetURLRe = regexp.MustCompile(`https?://(?:x|twitter)\.com/([^/]+)/status/(\d+)`)

// IsTweetURL reports whether url is an x.com/twitter.com status URL.
func IsTweetURL(url string) bool {
	return tweetURLRe.MatchString(url)
}

// fxtweetResponse is the fxtwitter API JSON shape (subset).
type fxtweetResponse struct {
	Code  int `json:"code"`
	Tweet struct {
		Text   string `json:"text"`
		Author struct {
			ScreenName  string `json:"screen_name"`
			Name        string `json:"name"`
			Followers   int64  `json:"followers"`
			Verified    bool   `json:"verified"`
			Description string `json:"description"`
		} `json:"author"`
		Replies    int64  `json:"replies"`
		Retweets   int64  `json:"retweets"`
		Likes      int64  `json:"likes"`
		Bookmarks  int64  `json:"bookmarks"`
		Views      int64  `json:"views"`
		CreatedAt  string `json:"created_at"`
		Lang       string `json:"lang"`
		ReplyingTo string `json:"replying_to"`
		Quote      *struct {
			URL    string `json:"url"`
			Text   string `json:"text"`
			Author struct {
				ScreenName string `json:"screen_name"`
				Name       string `json:"name"`
			} `json:"author"`
		} `json:"quote"`
		Article *struct {
			Title       string `json:"title"`
			PreviewText string `json:"preview_text"`
			Content     struct {
				Blocks []struct {
					Text string `json:"text"`
				} `json:"blocks"`
			} `json:"content"`
		} `json:"article"`
	} `json:"tweet"`
}

// Extract fetches tweet text from an X/Twitter URL via the fxtwitter API.
func (f *FxtwitterBackend) Extract(ctx context.Context, url string, format string) (*ExtractResult, error) {
	m := tweetURLRe.FindStringSubmatch(url)
	if m == nil {
		return nil, fmt.Errorf("%s: not an x.com/twitter.com status URL: %s", f.NameID, url)
	}
	handle, id := m[1], m[2]
	apiURL := fmt.Sprintf("%s/%s/status/%s", f.BaseURL, handle, id)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to create request: %w", f.NameID, err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: request failed: %w", f.NameID, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to read response: %w", f.NameID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d: %s", f.NameID, resp.StatusCode, string(body))
	}

	var fr fxtweetResponse
	if err := json.Unmarshal(body, &fr); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON: %w", f.NameID, err)
	}
	if fr.Code != 200 {
		return nil, fmt.Errorf("%s: API returned code %d", f.NameID, fr.Code)
	}

	content := fr.Tweet.Text
	title := ""
	if fr.Tweet.Article != nil {
		// note-tweet / article: reconstruct body from draftjs blocks
		var blocks []string
		for _, b := range fr.Tweet.Article.Content.Blocks {
			if s := strings.TrimSpace(b.Text); s != "" {
				blocks = append(blocks, s)
			}
		}
		title = fr.Tweet.Article.Title
		content = strings.Join(blocks, "\n")
		if content == "" {
			content = fr.Tweet.Article.PreviewText
		}
	}

	if format == "text" {
		content = stripBasicMarkdown(content)
	}

	md := map[string]string{
		"url":         url,
		"screen_name": "@" + fr.Tweet.Author.ScreenName,
		"name":        fr.Tweet.Author.Name,
		"followers":   fmt.Sprintf("%d", fr.Tweet.Author.Followers),
		"verified":    fmt.Sprintf("%t", fr.Tweet.Author.Verified),
		"description": fr.Tweet.Author.Description,
		"likes":       fmt.Sprintf("%d", fr.Tweet.Likes),
		"retweets":    fmt.Sprintf("%d", fr.Tweet.Retweets),
		"replies":     fmt.Sprintf("%d", fr.Tweet.Replies),
		"bookmarks":   fmt.Sprintf("%d", fr.Tweet.Bookmarks),
		"views":       fmt.Sprintf("%d", fr.Tweet.Views),
		"created_at":  fr.Tweet.CreatedAt,
		"lang":        fr.Tweet.Lang,
	}
	if fr.Tweet.ReplyingTo != "" {
		md["replying_to"] = fr.Tweet.ReplyingTo
	}
	if fr.Tweet.Quote != nil {
		md["quote_author"] = "@" + fr.Tweet.Quote.Author.ScreenName
		md["quote_text"] = fr.Tweet.Quote.Text
	}
	if fr.Tweet.Article != nil {
		md["note_tweet"] = "true"
	}

	return &ExtractResult{URL: url, Title: title, Content: content, Metadata: md}, nil
}
