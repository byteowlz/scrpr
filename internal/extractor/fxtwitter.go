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
//
// Thread mode (Thread=true) uses the v2 API (/2/thread/:id), which returns the
// unrolled thread from its first post even when given a mid-thread tweet ID;
// on failure it falls back to the legacy v1 single-tweet fetch. Thread=false
// fetches only the focal tweet via the v1 API (/:handle/status/:id).
type FxtwitterBackend struct {
	NameID  string // "fxtwitter" or "vxtwitter"
	Timeout time.Duration
	Thread  bool   // fetch the entire thread (default), falling back to the single tweet
	Quotes  bool   // also fetch quoted posts/threads and append them (default)
	BaseURL string // overridable for testing (default: https://api.fxtwitter.com)
	client  *http.Client
}

// NewFxtwitterBackend creates a fxtwitter/vxtwitter extraction backend.
// name selects the reported backend id ("fxtwitter" or "vxtwitter").
// thread selects full-thread extraction with single-tweet fallback; quotes
// selects automatic quote expansion (quoted posts and quoted threads).
func NewFxtwitterBackend(name string, timeout time.Duration, thread bool, quotes bool) *FxtwitterBackend {
	if name == "" {
		name = "fxtwitter"
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &FxtwitterBackend{
		NameID:  name,
		Timeout: timeout,
		Thread:  thread,
		Quotes:  quotes,
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

// fxV2Author is the author object shared by v2 statuses and thread responses.
type fxV2Author struct {
	ScreenName  string `json:"screen_name"`
	Name        string `json:"name"`
	Followers   int64  `json:"followers"`
	Verified    bool   `json:"verified"`
	Description string `json:"description"`
}

// fxV2Status is one post in a v2 API response (subset).
type fxV2Status struct {
	Type       string     `json:"type"` // "status", or "tombstone" for unavailable posts
	URL        string     `json:"url"`
	ID         string     `json:"id"`
	Text       string     `json:"text"`
	Author     fxV2Author `json:"author"`
	Replies    int64      `json:"replies"`
	Reposts    int64      `json:"reposts"`
	Likes      int64      `json:"likes"`
	Bookmarks  int64      `json:"bookmarks"`
	Views      int64      `json:"views"`
	CreatedAt  string     `json:"created_at"`
	Lang       string     `json:"lang"`
	ReplyingTo *struct {
		ScreenName string `json:"screen_name"`
		Status     string `json:"status"`
	} `json:"replying_to"`
	Quote *struct {
		ID     string     `json:"id"`
		URL    string     `json:"url"`
		Text   string     `json:"text"`
		Author fxV2Author `json:"author"`
	} `json:"quote"`
}

// fxV2ThreadResponse is the v2 /2/thread/:id JSON shape (subset). The thread
// array holds every post of the unrolled thread, starting with the first.
type fxV2ThreadResponse struct {
	Code   int          `json:"code"`
	Status *fxV2Status  `json:"status"` // focal post
	Thread []fxV2Status `json:"thread"`
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
// With Thread set (the default routing), the entire unrolled thread is fetched;
// if that fails for any reason, it falls back to the single-tweet fetch.
func (f *FxtwitterBackend) Extract(ctx context.Context, url string, format string) (*ExtractResult, error) {
	m := tweetURLRe.FindStringSubmatch(url)
	if m == nil {
		return nil, fmt.Errorf("%s: not an x.com/twitter.com status URL: %s", f.NameID, url)
	}
	handle, id := m[1], m[2]
	if f.Thread {
		res, threadErr := f.extractThread(ctx, url, format, id)
		if threadErr == nil {
			return res, nil
		}
		// Thread fetch failed (deleted focal post, API hiccup, ...): fall back
		// to the single-tweet fetch so tweet URLs keep working.
		if res, err := f.extractSingle(ctx, url, format, handle, id); err == nil {
			return res, nil
		}
		return nil, threadErr
	}
	return f.extractSingle(ctx, url, format, handle, id)
}

// extractSingle fetches one tweet via the legacy v1 API (/:handle/status/:id).
func (f *FxtwitterBackend) extractSingle(ctx context.Context, url string, format string, handle, id string) (*ExtractResult, error) {
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

// extractThread fetches the full unrolled thread for tweet id via the v2 API
// (/2/thread/:id) and renders every post. The v2 endpoint only needs the tweet
// ID and returns the thread from its first post even for mid-thread IDs.
// When Quotes is set, quoted posts/threads referenced by any post of the
// thread are fetched and appended.
func (f *FxtwitterBackend) extractThread(ctx context.Context, url string, format string, id string) (*ExtractResult, error) {
	focal, posts, err := f.fetchThreadPosts(ctx, id)
	if err != nil {
		return nil, err
	}

	// A one-post "thread" is just a tweet: render it exactly like the legacy
	// single-tweet path (no byline heading) so plain tweet URLs are unchanged.
	var content string
	if len(posts) == 1 {
		content = posts[0].Text
	} else {
		content = renderThread(posts, format)
	}
	var quotedThreads int
	if f.Quotes {
		for _, section := range f.quoteSections(ctx, posts, format) {
			content += section
			quotedThreads++
		}
	}

	md := map[string]string{
		"url":          url,
		"screen_name":  "@" + focal.Author.ScreenName,
		"name":         focal.Author.Name,
		"followers":    fmt.Sprintf("%d", focal.Author.Followers),
		"verified":     fmt.Sprintf("%t", focal.Author.Verified),
		"description":  focal.Author.Description,
		"likes":        fmt.Sprintf("%d", focal.Likes),
		"retweets":     fmt.Sprintf("%d", focal.Reposts),
		"replies":      fmt.Sprintf("%d", focal.Replies),
		"bookmarks":    fmt.Sprintf("%d", focal.Bookmarks),
		"views":        fmt.Sprintf("%d", focal.Views),
		"created_at":   focal.CreatedAt,
		"lang":         focal.Lang,
		"thread_posts": fmt.Sprintf("%d", len(posts)),
	}
	if focal.ReplyingTo != nil && focal.ReplyingTo.ScreenName != "" {
		md["replying_to"] = focal.ReplyingTo.ScreenName
	}
	if focal.Quote != nil {
		md["quote_author"] = "@" + focal.Quote.Author.ScreenName
		md["quote_text"] = focal.Quote.Text
	}

	title := ""
	if len(posts) > 1 {
		title = fmt.Sprintf("Thread by @%s", posts[0].Author.ScreenName)
	}
	if quotedThreads > 0 {
		md["quoted_threads"] = fmt.Sprintf("%d", quotedThreads)
	}

	return &ExtractResult{URL: url, Title: title, Content: content, Metadata: md}, nil
}

// maxQuotedThreads bounds quote expansion per extraction so a thread that
// quotes heavily cannot turn into an unbounded crawl.
const maxQuotedThreads = 5

// fetchThreadPosts fetches /2/thread/:id and returns the focal post (when
// accessible) plus the accessible posts of the unrolled thread. Tombstones
// (deleted/suspended posts) are skipped rather than rendered as holes.
func (f *FxtwitterBackend) fetchThreadPosts(ctx context.Context, id string) (*fxV2Status, []fxV2Status, error) {
	apiURL := fmt.Sprintf("%s/2/thread/%s", f.BaseURL, id)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: failed to create request: %w", f.NameID, err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: request failed: %w", f.NameID, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: failed to read response: %w", f.NameID, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("%s: HTTP %d: %s", f.NameID, resp.StatusCode, string(body))
	}

	var tr fxV2ThreadResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, nil, fmt.Errorf("%s: invalid JSON: %w", f.NameID, err)
	}
	if tr.Code != 200 {
		return nil, nil, fmt.Errorf("%s: API returned code %d", f.NameID, tr.Code)
	}

	var posts []fxV2Status
	for _, p := range tr.Thread {
		if p.Type == "status" {
			posts = append(posts, p)
		}
	}
	if len(posts) == 0 {
		return nil, nil, fmt.Errorf("%s: thread %s contains no accessible posts", f.NameID, id)
	}

	focal := tr.Status
	if focal == nil || focal.Type != "status" {
		focal = &posts[0]
	}
	return focal, posts, nil
}

// quoteSections fetches the unique quoted posts/threads referenced by posts
// and renders each as a labelled section appended to the main content.
// Depth is 1: quotes inside quoted content are not followed. Deleted or
// protected quotes are skipped silently — a broken quote must not fail the
// extraction of the requested thread.
func (f *FxtwitterBackend) quoteSections(ctx context.Context, posts []fxV2Status, format string) []string {
	inThread := make(map[string]bool, len(posts))
	seen := make(map[string]bool)
	var ids []string
	for i := range posts {
		q := posts[i].Quote
		if q == nil || q.ID == "" || seen[q.ID] || inThread[q.ID] {
			continue
		}
		seen[q.ID] = true
		for _, p := range posts {
			if p.ID == q.ID {
				inThread[q.ID] = true // quote of a post already in this thread
				break
			}
		}
		if !inThread[q.ID] {
			ids = append(ids, q.ID)
		}
	}

	var sections []string
	for _, qid := range ids {
		if len(sections) >= maxQuotedThreads {
			break
		}
		qfocal, qposts, err := f.fetchThreadPosts(ctx, qid)
		if err != nil || qfocal == nil {
			continue
		}
		label := "Quoted post"
		if len(qposts) > 1 {
			label = "Quoted thread"
		}
		heading := fmt.Sprintf("%s by @%s", label, qfocal.Author.ScreenName)
		var b strings.Builder
		// Leading blank line: a `---` glued to the previous text line would
		// parse as a setext heading underline in markdown.
		if format == "markdown" {
			b.WriteString("\n\n---\n\n## " + heading + "\n\n")
		} else {
			b.WriteString("\n\n[" + heading + "]\n")
		}
		if len(qposts) == 1 {
			b.WriteString(qposts[0].Text)
		} else {
			b.WriteString(renderThread(qposts, format))
		}
		b.WriteString("\n")
		sections = append(sections, b.String())
	}
	return sections
}

// renderThread joins all posts of a thread into one document. Markdown gets a
// numbered heading per post and horizontal rules between posts; text format
// gets a plain author-date byline and blank-line separation.
func renderThread(posts []fxV2Status, format string) string {
	var b strings.Builder
	for i, p := range posts {
		if i > 0 {
			if format == "markdown" {
				b.WriteString("\n---\n\n")
			} else {
				b.WriteString("\n\n")
			}
		}

		heading := fmt.Sprintf("@%s — %s", p.Author.ScreenName, p.CreatedAt)
		if len(posts) > 1 {
			heading = fmt.Sprintf("%d/ %s", i+1, heading)
		}
		if format == "markdown" {
			b.WriteString("### " + heading + "\n\n")
		} else {
			b.WriteString(heading + "\n")
		}
		b.WriteString(p.Text)
		b.WriteString("\n")
	}
	return b.String()
}
