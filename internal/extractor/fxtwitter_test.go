package extractor

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestFxtwitterURLTranslation verifies x.com/twitter.com -> fxtwitter API translation.
func TestFxtwitterURLTranslation(t *testing.T) {
	cases := []struct{ in, handle, id string }{
		{"https://x.com/_can1357/status/2084104053651317140", "_can1357", "2084104053651317140"},
		{"https://twitter.com/foo/status/123", "foo", "123"},
		{"https://x.com/some_user/status/999999", "some_user", "999999"},
		{"https://twitter.com/u/status/1", "u", "1"},
	}
	b := NewFxtwitterBackend("fxtwitter", 0, false, true)
	for _, c := range cases {
		m := tweetURLRe.FindStringSubmatch(c.in)
		if m == nil {
			t.Errorf("%s: failed to match", c.in)
			continue
		}
		if m[1] != c.handle || m[2] != c.id {
			t.Errorf("%s: got handle=%q id=%q want %q/%q", c.in, m[1], m[2], c.handle, c.id)
		}
		if b.Name() != "fxtwitter" {
			t.Errorf("Name() = %q", b.Name())
		}
	}
}

// TestFxtwitterRejectsNonTweetURLs verifies non-X URLs are rejected before any HTTP.
func TestFxtwitterRejectsNonTweetURLs(t *testing.T) {
	b := NewFxtwitterBackend("vxtwitter", 0, false, true)
	if b.Name() != "vxtwitter" {
		t.Errorf("Name() = %q", b.Name())
	}
	if _, err := b.Extract(t.Context(), "https://example.com/article", "markdown"); err == nil {
		t.Errorf("expected error for non-tweet URL, got nil")
	}
}

// threadFixture is a realistic v2 /2/thread/:id response body.
const threadFixture = `{
	"code": 200,
	"status": {"type": "status", "id": "100", "text": "first post", "created_at": "Wed Jun 01 12:00:00 +0000 2022",
		"author": {"screen_name": "alice", "name": "Alice", "followers": 10, "verified": false},
		"replies": 3, "reposts": 4, "likes": 5, "bookmarks": 1, "views": 100, "lang": "en"},
	"thread": [
		{"type": "status", "id": "100", "text": "first post", "created_at": "Wed Jun 01 12:00:00 +0000 2022",
			"author": {"screen_name": "alice", "name": "Alice", "followers": 10},
			"replies": 3, "reposts": 4, "likes": 5, "views": 100, "lang": "en"},
		{"type": "tombstone"},
		{"type": "status", "id": "101", "text": "second post", "created_at": "Wed Jun 01 12:05:00 +0000 2022",
			"author": {"screen_name": "alice", "name": "Alice", "followers": 10},
			"replies": 0, "reposts": 1, "likes": 2, "views": 50, "lang": "en"}
	]
}`

// TestFxtwitterThreadExtraction verifies --thread mode hits /2/thread/:id and
// renders every accessible post, skipping tombstones.
func TestFxtwitterThreadExtraction(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, threadFixture)
	}))
	defer srv.Close()

	b := NewFxtwitterBackend("fxtwitter", 0, true, true)
	b.BaseURL = srv.URL

	res, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if gotPath != "/2/thread/100" {
		t.Errorf("API path = %q, want /2/thread/100", gotPath)
	}

	// Both posts present in order, tombstone skipped, markdown headings numbered.
	for _, want := range []string{"1/ @alice", "first post", "2/ @alice", "second post"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("content missing %q\n---\n%s", want, res.Content)
		}
	}
	if strings.Contains(res.Content, "tombstone") {
		t.Errorf("tombstone rendered in content")
	}
	if !strings.Contains(res.Content, "\n---\n\n") {
		t.Errorf("posts not separated by horizontal rule")
	}

	if res.Metadata["thread_posts"] != "2" {
		t.Errorf("thread_posts = %q, want 2", res.Metadata["thread_posts"])
	}
	if res.Metadata["screen_name"] != "@alice" {
		t.Errorf("screen_name = %q, want @alice", res.Metadata["screen_name"])
	}
	if res.Metadata["likes"] != "5" {
		t.Errorf("likes = %q, want 5 (focal post metrics)", res.Metadata["likes"])
	}
	if res.Title != "Thread by @alice" {
		t.Errorf("Title = %q", res.Title)
	}

	// Text format: plain bylines, no markdown heading/rule syntax.
	resText, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "text")
	if err != nil {
		t.Fatalf("Extract (text) failed: %v", err)
	}
	if strings.Contains(resText.Content, "###") || strings.Contains(resText.Content, "\n---\n") {
		t.Errorf("text format contains markdown syntax:\n%s", resText.Content)
	}
	if !strings.Contains(resText.Content, "@alice — Wed Jun 01 12:00:00 +0000 2022") {
		t.Errorf("text format missing plain byline:\n%s", resText.Content)
	}
}

// TestFxtwitterSinglePostThread verifies a one-post thread renders the post
// text exactly like the legacy single-tweet path (no byline, no numbering).
func TestFxtwitterSinglePostThread(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"code":200,"status":{"type":"status","id":"100","text":"lone post","created_at":"Wed Jun 01 12:00:00 +0000 2022","author":{"screen_name":"alice","name":"Alice","followers":10},"likes":5,"views":100,"lang":"en"},"thread":[{"type":"status","id":"100","text":"lone post","created_at":"Wed Jun 01 12:00:00 +0000 2022","author":{"screen_name":"alice","name":"Alice","followers":10},"likes":5,"views":100,"lang":"en"}]}`)
	}))
	defer srv.Close()

	b := NewFxtwitterBackend("fxtwitter", 0, true, true)
	b.BaseURL = srv.URL

	res, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if res.Content != "lone post" {
		t.Errorf("unexpected single-post content:\n%q", res.Content)
	}
	if res.Metadata["thread_posts"] != "1" || res.Title != "" {
		t.Errorf("thread_posts=%q title=%q", res.Metadata["thread_posts"], res.Title)
	}
}

// TestFxtwitterThreadFallsBackToSingle verifies that when the v2 thread
// endpoint fails, Extract falls back to the legacy v1 single-tweet fetch.
func TestFxtwitterThreadFallsBackToSingle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, "/2/thread/") {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"code":500,"message":"INTERNAL_ERROR"}`)
			return
		}
		// v1 single-tweet path: /alice/status/100
		io.WriteString(w, `{"code":200,"tweet":{"text":"fallback text","author":{"screen_name":"alice","name":"Alice","followers":10},"likes":5,"views":100,"lang":"en"}}`)
	}))
	defer srv.Close()

	b := NewFxtwitterBackend("fxtwitter", 0, true, true)
	b.BaseURL = srv.URL

	res, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if res.Content != "fallback text" {
		t.Errorf("content = %q, want fallback text", res.Content)
	}
	if _, ok := res.Metadata["thread_posts"]; ok {
		t.Errorf("fallback result should not carry thread_posts")
	}
}

// quoteFixture: main thread (alice, 1 post) quoting bob's post 900; bob's
// thread has 2 posts, and bob's second post quotes carol's post 901 (which
// must NOT be fetched — quote expansion is depth 1).
const quoteMainFixture = `{
	"code": 200,
	"status": {"type": "status", "id": "100", "text": "look at this",
		"author": {"screen_name": "alice", "name": "Alice"},
		"quote": {"id": "900", "url": "https://x.com/bob/status/900", "text": "bob original",
			"author": {"screen_name": "bob", "name": "Bob"}},
		"created_at": "Wed Jun 01 12:00:00 +0000 2022", "lang": "en"},
	"thread": [{"type": "status", "id": "100", "text": "look at this",
		"author": {"screen_name": "alice", "name": "Alice"},
		"quote": {"id": "900", "url": "https://x.com/bob/status/900", "text": "bob original",
			"author": {"screen_name": "bob", "name": "Bob"}},
		"created_at": "Wed Jun 01 12:00:00 +0000 2022", "lang": "en"}]
}`

const quoteBobFixture = `{
	"code": 200,
	"status": {"type": "status", "id": "900", "text": "bob original",
		"author": {"screen_name": "bob", "name": "Bob"}, "created_at": "Wed Jun 01 11:00:00 +0000 2022", "lang": "en"},
	"thread": [
		{"type": "status", "id": "900", "text": "bob original",
			"author": {"screen_name": "bob", "name": "Bob"}, "created_at": "Wed Jun 01 11:00:00 +0000 2022", "lang": "en"},
		{"type": "status", "id": "901", "text": "bob followup",
			"author": {"screen_name": "bob", "name": "Bob"},
			"quote": {"id": "9010", "url": "https://x.com/carol/status/9010", "text": "carol said",
				"author": {"screen_name": "carol", "name": "Carol"}},
			"created_at": "Wed Jun 01 11:05:00 +0000 2022", "lang": "en"}
	]
}`

// TestFxtwitterQuoteExpansion verifies quoted threads are fetched once, depth
// is 1 (quotes inside quotes are not followed), and sections are appended.
func TestFxtwitterQuoteExpansion(t *testing.T) {
	var mu sync.Mutex
	threadHits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		threadHits[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/2/thread/100":
			io.WriteString(w, quoteMainFixture)
		case "/2/thread/900":
			io.WriteString(w, quoteBobFixture)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":404}`)
		}
	}))
	defer srv.Close()

	b := NewFxtwitterBackend("fxtwitter", 0, true, true)
	b.BaseURL = srv.URL

	res, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	if !strings.Contains(res.Content, "## Quoted thread by @bob") {
		t.Errorf("missing quoted thread section:\n%s", res.Content)
	}
	for _, want := range []string{"bob original", "bob followup"} {
		if !strings.Contains(res.Content, want) {
			t.Errorf("content missing quoted text %q:\n%s", want, res.Content)
		}
	}
	if res.Metadata["quoted_threads"] != "1" {
		t.Errorf("quoted_threads = %q, want 1", res.Metadata["quoted_threads"])
	}

	// Quoted thread fetched exactly once (dedupe), depth 1: no fetch of the
	// quote inside bob's followup.
	mu.Lock()
	defer mu.Unlock()
	if threadHits["/2/thread/900"] != 1 {
		t.Errorf("quoted thread fetched %d times, want 1", threadHits["/2/thread/900"])
	}
	if threadHits["/2/thread/9010"] != 0 {
		t.Errorf("quote-of-quote was fetched; depth must be 1")
	}
}

// TestFxtwitterQuotesDisabled verifies --no-quotes does not fetch quotes.
func TestFxtwitterQuotesDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/2/thread/900" {
			t.Errorf("quote endpoint should not be called with Quotes=false")
		}
		io.WriteString(w, quoteMainFixture)
	}))
	defer srv.Close()

	b := NewFxtwitterBackend("fxtwitter", 0, true, false)
	b.BaseURL = srv.URL

	res, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	if strings.Contains(res.Content, "Quoted") {
		t.Errorf("quote section rendered with quotes disabled:\n%s", res.Content)
	}
	if _, ok := res.Metadata["quoted_threads"]; ok {
		t.Errorf("quoted_threads set with quotes disabled")
	}
}

// TestFxtwitterQuoteFailureSkipped verifies a deleted/protected quote does not
// fail the extraction and leaves no empty section.
func TestFxtwitterQuoteFailureSkipped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/2/thread/900" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":404,"message":"NOT_FOUND"}`)
			return
		}
		io.WriteString(w, quoteMainFixture)
	}))
	defer srv.Close()

	b := NewFxtwitterBackend("fxtwitter", 0, true, true)
	b.BaseURL = srv.URL

	res, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "markdown")
	if err != nil {
		t.Fatalf("unavailable quote must not fail extraction: %v", err)
	}
	if strings.Contains(res.Content, "Quoted") {
		t.Errorf("empty quote section rendered:\n%s", res.Content)
	}
	if res.Content != "look at this" {
		t.Errorf("content = %q, want focal text only", res.Content)
	}
}

// TestFxtwitterQuoteDedupe verifies two posts quoting the same tweet fetch it
// once.
func TestFxtwitterQuoteDedupe(t *testing.T) {
	dedupeFixture := `{
		"code": 200,
		"status": {"type": "status", "id": "100", "text": "post one",
			"author": {"screen_name": "alice", "name": "Alice"},
			"quote": {"id": "900", "url": "https://x.com/bob/status/900", "text": "bob original",
				"author": {"screen_name": "bob", "name": "Bob"}},
			"created_at": "Wed Jun 01 12:00:00 +0000 2022", "lang": "en"},
		"thread": [
			{"type": "status", "id": "100", "text": "post one",
				"author": {"screen_name": "alice", "name": "Alice"},
				"quote": {"id": "900", "url": "https://x.com/bob/status/900", "text": "bob original",
					"author": {"screen_name": "bob", "name": "Bob"}},
				"created_at": "Wed Jun 01 12:00:00 +0000 2022", "lang": "en"},
			{"type": "status", "id": "101", "text": "post two quotes the same",
				"author": {"screen_name": "alice", "name": "Alice"},
				"quote": {"id": "900", "url": "https://x.com/bob/status/900", "text": "bob original",
					"author": {"screen_name": "bob", "name": "Bob"}},
				"created_at": "Wed Jun 01 12:05:00 +0000 2022", "lang": "en"}
		]
	}`
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/2/thread/900" {
			mu.Lock()
			hits++
			mu.Unlock()
			io.WriteString(w, quoteBobFixture)
			return
		}
		io.WriteString(w, dedupeFixture)
	}))
	defer srv.Close()

	b := NewFxtwitterBackend("fxtwitter", 0, true, true)
	b.BaseURL = srv.URL

	res, err := b.Extract(t.Context(), "https://x.com/alice/status/100", "markdown")
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits != 1 {
		t.Errorf("quoted thread fetched %d times for 2 quote references, want 1", hits)
	}
	if strings.Count(res.Content, "Quoted thread by @bob") != 1 {
		t.Errorf("quoted section rendered more than once:\n%s", res.Content)
	}
}
