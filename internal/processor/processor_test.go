package processor

import (
	"strings"
	"testing"
)

// Regression for trx-2d9y: markdown output must include body content, not just
// the title. Readability wraps content in <div id="readability-page-1">, whose
// id contains the substring "ad"; the old ad-removal selectors deleted it.
func TestToMarkdownIncludesBody(t *testing.T) {
	html := `<!DOCTYPE html><html><head><title>Test Article</title></head>
<body><article><h1>Test Article</h1>
<p>This is the first paragraph of body content that should appear.</p>
<p>Here is a second paragraph with more information about the topic.</p>
<p>And a third paragraph to make sure readability picks it up as real content and not boilerplate noise here.</p>
</article></body></html>`

	cp := NewContentProcessor()
	p, err := cp.Process(html, "http://example.com/", ProcessOptions{
		RemoveAds:        true,
		CleanHTML:        true,
		MinContentLength: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	md := cp.ToMarkdown(p, false, true)
	if !strings.Contains(md, "first paragraph of body content") {
		t.Fatalf("markdown missing body content:\n%s", md)
	}
}

func TestRemoveAdsKeepsNonAdTokens(t *testing.T) {
	cp := NewContentProcessor()
	html := `<div id="readability-page-1" class="page header"><p>keep me</p><div class="banner-ad">spam</div></div>`
	got := cp.removeAds(html)
	if !strings.Contains(got, "keep me") {
		t.Errorf("removed legitimate content: %q", got)
	}
	if strings.Contains(got, "spam") {
		t.Errorf("did not remove ad element: %q", got)
	}
}
