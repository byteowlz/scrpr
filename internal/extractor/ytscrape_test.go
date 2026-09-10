package extractor

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseYtInitialDataRawObject(t *testing.T) {
	body := `<html><script>var ytInitialData = {"contents":{"videoRenderer":{"title":{"runs":[{"text":"Rust programming tutorial"},{"text":" part 1"}]}}}};</script></html>`
	data, err := parseYtInitialData(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var titles []string
	walkYtJSON(data, func(title string) { titles = append(titles, title) })
	if len(titles) != 1 || titles[0] != "Rust programming tutorial part 1" {
		t.Fatalf("unexpected titles: %v", titles)
	}
}

func TestParseYtInitialDataEscapedMobile(t *testing.T) {
	// Mobile site escapes the JSON as a single-quoted string with \xNN bytes.
	esc := `{"contents":{"videoRenderer":{"title":{"runs":[{"text":"Rust programming"}]}}}}`
	var sb strings.Builder
	for i := 0; i < len(esc); i++ {
		if esc[i] < 0x80 {
			sb.WriteString("\\x" + fmt.Sprintf("%02x", esc[i]))
		}
	}
	body := `<script>var ytInitialData = '` + sb.String() + `';</script>`
	data, err := parseYtInitialData(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var titles []string
	walkYtJSON(data, func(title string) { titles = append(titles, title) })
	if len(titles) != 1 || titles[0] != "Rust programming" {
		t.Fatalf("unexpected titles: %v", titles)
	}
}

func TestIsYouTubeSearchURL(t *testing.T) {
	cases := map[string]bool{
		"https://www.youtube.com/results?search_query=rust": true,
		"https://m.youtube.com/results?search_query=x":      true,
		"https://youtube.com/results?search_query=go":       true,
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ":       false,
		"https://www.google.com/results?q=rust":             false,
	}
	for u, want := range cases {
		if got := IsYouTubeSearchURL(u); got != want {
			t.Errorf("IsYouTubeSearchURL(%q) = %v, want %v", u, got, want)
		}
	}
}
