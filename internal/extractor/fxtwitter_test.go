package extractor

import (
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
	b := NewFxtwitterBackend("fxtwitter", 0)
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
	b := NewFxtwitterBackend("vxtwitter", 0)
	if b.Name() != "vxtwitter" {
		t.Errorf("Name() = %q", b.Name())
	}
	if _, err := b.Extract(t.Context(), "https://example.com/article", "markdown"); err == nil {
		t.Errorf("expected error for non-tweet URL, got nil")
	}
}
