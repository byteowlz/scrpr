package processor

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
)

// Readability's "unlikely candidate" and class-weight heuristics match
// substrings of class/id attributes ("header", "footer", "menu", "comment",
// ...). Component-scoped class names from CSS modules and similar tooling
// (e.g. "JumplinkHeader-module__x__jumplinkHeader") trip these, so readability
// silently drops every section heading wrapped in such an element while keeping
// the surrounding paragraphs. The text looks plausible but its structure is gone.
//
// recoverHeadings detects that case by comparing heading counts in the page's
// main container against the extraction, and re-runs readability on that
// container with class/id/style attributes stripped, so only structural
// heuristics (tags, text and link density) apply.

const (
	// minSourceHeadings is the number of headings the source container must
	// have before heading loss is considered meaningful.
	minSourceHeadings = 3
	// minRecoveredTextRatio guards against a retry that keeps the headings but
	// loses most of the text.
	minRecoveredTextRatio = 0.5
)

const headingSelector = "h2, h3, h4, h5, h6"

// recoverHeadings returns a replacement article when the original extraction
// lost most of the source's headings and a class-agnostic retry keeps more of
// them. It returns nil when the original extraction should be kept.
func recoverHeadings(html string, original readability.Article) *readability.Article {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil
	}
	container := mainContainer(doc)
	sourceHeadings := container.Find(headingSelector).Length()
	if sourceHeadings < minSourceHeadings {
		return nil
	}
	kept := countHeadings(original.Content)
	if kept*2 >= sourceHeadings {
		return nil
	}

	container.Find("nav, aside, script, style, noscript, template, svg").Remove()
	container.Find("*").Each(func(_ int, s *goquery.Selection) {
		s.RemoveAttr("class")
		s.RemoveAttr("id")
		s.RemoveAttr("style")
	})
	inner, err := container.Html()
	if err != nil {
		return nil
	}

	var page strings.Builder
	page.WriteString("<html><head><title>")
	page.WriteString(escapeText(doc.Find("title").First().Text()))
	page.WriteString("</title></head><body><div>")
	page.WriteString(inner)
	page.WriteString("</div></body></html>")

	retry, err := readability.FromReader(strings.NewReader(page.String()), nil)
	if err != nil {
		return nil
	}
	if countHeadings(retry.Content) <= kept {
		return nil
	}
	if float64(len(retry.TextContent)) < float64(len(original.TextContent))*minRecoveredTextRatio {
		return nil
	}
	return &retry
}

// mainContainer returns the element most likely to hold the page's primary
// content, falling back to <body>.
func mainContainer(doc *goquery.Document) *goquery.Selection {
	for _, sel := range []string{"main", "[role='main']", "article"} {
		if s := doc.Find(sel); s.Length() == 1 {
			return s
		}
	}
	return doc.Find("body").First()
}

// countHeadings counts h2-h6 elements in an HTML fragment. h1 is excluded
// because readability rewrites it to h2 and pages usually reserve it for the
// title, which is reported separately.
func countHeadings(fragment string) int {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(fragment))
	if err != nil {
		return 0
	}
	return doc.Find(headingSelector).Length()
}

func escapeText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
