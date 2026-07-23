package processor

import (
	"fmt"
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
)

type ProcessOptions struct {
	RemoveAds        bool
	CleanHTML        bool
	MinContentLength int
	IncludeMetadata  bool
	MetadataFields   []string
}

type ProcessedContent struct {
	Title       string
	Content     string
	TextContent string
	Author      string
	Excerpt     string
	Byline      string
	Length      int
	Metadata    map[string]string
	Images      []string
	Links       []Link
}

type Link struct {
	Text string
	URL  string
}

type ContentProcessor struct {
}

func NewContentProcessor() *ContentProcessor {
	return &ContentProcessor{}
}

func (cp *ContentProcessor) Process(html, url string, opts ProcessOptions) (*ProcessedContent, error) {
	if len(html) < opts.MinContentLength {
		return nil, fmt.Errorf("content too short: %d characters (minimum: %d)", len(html), opts.MinContentLength)
	}

	// Use readability to extract main content
	article, err := readability.FromReader(strings.NewReader(html), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to process with readability: %w", err)
	}

	result := &ProcessedContent{
		Title:       article.Title,
		Content:     article.Content,
		TextContent: cp.CleanNewlines(article.TextContent),
		Author:      article.Byline,
		Excerpt:     article.Excerpt,
		Byline:      article.Byline,
		Length:      article.Length,
		Metadata:    make(map[string]string),
		Images:      []string{},
		Links:       []Link{},
	}

	// Parse HTML for additional processing
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(article.Content))
	if err != nil {
		return result, nil // Return what we have from readability
	}

	// Extract images
	result.Images = cp.extractImages(doc)

	// Extract links
	result.Links = cp.extractLinks(doc)

	// Extract additional metadata if requested
	if opts.IncludeMetadata {
		originalDoc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err == nil {
			result.Metadata = cp.extractMetadata(originalDoc, opts.MetadataFields)
		}
	}

	// Clean HTML if requested
	if opts.CleanHTML {
		result.Content = cp.cleanHTML(result.Content)
	}

	// Remove ads if requested
	if opts.RemoveAds {
		result.Content = cp.removeAds(result.Content)
	}

	return result, nil
}

func (cp *ContentProcessor) extractImages(doc *goquery.Document) []string {
	var images []string

	doc.Find("img").Each(func(i int, s *goquery.Selection) {
		if src, exists := s.Attr("src"); exists && src != "" {
			images = append(images, src)
		}
		// Also check data-src for lazy loaded images
		if dataSrc, exists := s.Attr("data-src"); exists && dataSrc != "" {
			images = append(images, dataSrc)
		}
	})

	return images
}

func (cp *ContentProcessor) extractLinks(doc *goquery.Document) []Link {
	var links []Link

	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if !exists || href == "" {
			return
		}

		text := strings.TrimSpace(s.Text())
		if text == "" {
			text = href
		}

		links = append(links, Link{
			Text: text,
			URL:  href,
		})
	})

	return links
}

func (cp *ContentProcessor) extractMetadata(doc *goquery.Document, fields []string) map[string]string {
	metadata := make(map[string]string)

	for _, field := range fields {
		switch field {
		case "title":
			if title := doc.Find("title").Text(); title != "" {
				metadata["title"] = strings.TrimSpace(title)
			}
		case "author":
			if author := cp.findMetaContent(doc, []string{"author", "article:author"}); author != "" {
				metadata["author"] = author
			}
		case "description":
			if desc := cp.findMetaContent(doc, []string{"description", "og:description"}); desc != "" {
				metadata["description"] = desc
			}
		case "date":
			if date := cp.findMetaContent(doc, []string{"article:published_time", "date", "pubdate"}); date != "" {
				metadata["date"] = date
			}
		case "url":
			if url := cp.findMetaContent(doc, []string{"og:url", "canonical"}); url != "" {
				metadata["url"] = url
			} else if canonical := doc.Find("link[rel='canonical']").AttrOr("href", ""); canonical != "" {
				metadata["url"] = canonical
			}
		case "image":
			if image := cp.findMetaContent(doc, []string{"og:image", "twitter:image"}); image != "" {
				metadata["image"] = image
			}
		case "keywords":
			if keywords := cp.findMetaContent(doc, []string{"keywords"}); keywords != "" {
				metadata["keywords"] = keywords
			}
		}
	}

	return metadata
}

func (cp *ContentProcessor) findMetaContent(doc *goquery.Document, properties []string) string {
	for _, prop := range properties {
		// Check name attribute
		if content := doc.Find(fmt.Sprintf("meta[name='%s']", prop)).AttrOr("content", ""); content != "" {
			return strings.TrimSpace(content)
		}
		// Check property attribute (for Open Graph tags)
		if content := doc.Find(fmt.Sprintf("meta[property='%s']", prop)).AttrOr("content", ""); content != "" {
			return strings.TrimSpace(content)
		}
	}
	return ""
}

func (cp *ContentProcessor) cleanHTML(content string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return content
	}

	// Remove script and style elements
	doc.Find("script, style, noscript").Remove()

	// Remove comments
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		html, _ := s.Html()
		cleanedHTML := cp.removeHTMLComments(html)
		s.SetHtml(cleanedHTML)
	})

	// Remove empty paragraphs and divs
	doc.Find("p, div").Each(func(i int, s *goquery.Selection) {
		if strings.TrimSpace(s.Text()) == "" {
			s.Remove()
		}
	})

	result, _ := doc.Html()
	return result
}

func (cp *ContentProcessor) removeHTMLComments(html string) string {
	// Simple comment removal - could be improved with a proper HTML parser
	for strings.Contains(html, "<!--") {
		start := strings.Index(html, "<!--")
		end := strings.Index(html[start:], "-->")
		if end == -1 {
			break
		}
		end += start + 3
		html = html[:start] + html[end:]
	}
	return html
}

// adTokens are id/class tokens that mark an element as advertising. They are
// matched against delimiter-separated tokens (not substrings) so that words
// like "readability" or "header" are not mistaken for "ad".
var adTokens = map[string]bool{
	"ad":            true,
	"ads":           true,
	"advert":        true,
	"adsense":       true,
	"adsystem":      true,
	"advertisement": true,
	"advertising":   true,
	"sponsor":       true,
	"sponsored":     true,
	"promo":         true,
}

func hasAdToken(attr string) bool {
	tokens := strings.FieldsFunc(strings.ToLower(attr), func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	for _, tok := range tokens {
		if adTokens[tok] {
			return true
		}
	}
	return false
}

func (cp *ContentProcessor) removeAds(content string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return content
	}

	doc.Find("[id], [class]").Each(func(i int, s *goquery.Selection) {
		id, _ := s.Attr("id")
		class, _ := s.Attr("class")
		if hasAdToken(id) || hasAdToken(class) {
			s.Remove()
		}
	})

	result, _ := doc.Html()
	return result
}

func (cp *ContentProcessor) ToText(content *ProcessedContent, lineWidth int) string {
	var text string
	if content.TextContent != "" {
		text = content.TextContent
	} else {
		// Fallback: extract text from HTML
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(content.Content))
		if err != nil {
			return content.Content
		}
		text = doc.Text()
	}

	// Clean newlines before wrapping
	text = cp.CleanNewlines(text)
	return cp.wrapText(text, lineWidth)
}

func (cp *ContentProcessor) ToMarkdown(content *ProcessedContent, includeMetadata bool, preserveLinks bool) string {
	var md strings.Builder

	// Add title
	if content.Title != "" {
		md.WriteString(fmt.Sprintf("# %s\n\n", content.Title))
	}

	// Add metadata if requested
	if includeMetadata {
		if content.Author != "" {
			md.WriteString(fmt.Sprintf("**Author:** %s\n\n", content.Author))
		}
		if content.Excerpt != "" {
			md.WriteString(fmt.Sprintf("**Summary:** %s\n\n", content.Excerpt))
		}
		for key, value := range content.Metadata {
			if key != "title" { // Title already added
				md.WriteString(fmt.Sprintf("**%s:** %s\n\n", strings.Title(key), value))
			}
		}
	}

	// If we have text content from readability, use that as fallback
	if content.TextContent != "" && strings.TrimSpace(content.Content) == "" {
		md.WriteString(cp.CleanNewlines(content.TextContent))
		return md.String()
	}

	// Convert HTML content to markdown using battle-tested library
	htmlContent := content.Content
	if htmlContent == "" {
		md.WriteString(cp.CleanNewlines(content.TextContent))
		return md.String()
	}

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
		),
	)

	result, err := conv.ConvertString(htmlContent)
	if err != nil {
		// Fallback to text content on conversion failure
		md.WriteString(cp.CleanNewlines(content.TextContent))
		return md.String()
	}

	// Strip links if not preserving them
	if !preserveLinks {
		result = cp.stripMarkdownLinks(result)
	}

	md.WriteString(cp.CleanNewlines(result))
	return md.String()
}

// stripMarkdownLinks converts [text](url) -> text
func (cp *ContentProcessor) stripMarkdownLinks(md string) string {
	var result strings.Builder
	i := 0
	for i < len(md) {
		if md[i] == '[' {
			// Find closing ]
			end := i + 1
			for end < len(md) && md[end] != ']' {
				end++
			}
			if end < len(md) && md[end] == ']' {
				// Check if followed by (link)
				linkStart := end + 1
				if linkStart < len(md) && md[linkStart] == '(' {
					linkEnd := linkStart + 1
					for linkEnd < len(md) && md[linkEnd] != ')' {
						linkEnd++
					}
					if linkEnd < len(md) && md[linkEnd] == ')' {
						// It's a link: write just the text
						result.WriteString(md[i+1 : end])
						i = linkEnd + 1
						continue
					}
				}
			}
		}
		result.WriteByte(md[i])
		i++
	}
	return result.String()
}

func (cp *ContentProcessor) wrapText(text string, lineWidth int) string {
	if lineWidth <= 0 {
		return text
	}

	var result strings.Builder
	paragraphs := strings.Split(text, "\n\n")

	for i, paragraph := range paragraphs {
		if i > 0 {
			result.WriteString("\n\n")
		}

		words := strings.Fields(paragraph)
		if len(words) == 0 {
			continue
		}

		currentLine := words[0]
		for _, word := range words[1:] {
			if len(currentLine)+1+len(word) <= lineWidth {
				currentLine += " " + word
			} else {
				result.WriteString(currentLine + "\n")
				currentLine = word
			}
		}
		result.WriteString(currentLine)
	}

	return result.String()
}

func (cp *ContentProcessor) ProcessFromReader(r io.Reader, url string, opts ProcessOptions) (*ProcessedContent, error) {
	htmlBytes, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTML: %w", err)
	}

	return cp.Process(string(htmlBytes), url, opts)
}

// CleanNewlines removes unwanted newlines that break up sentences
func (cp *ContentProcessor) CleanNewlines(text string) string {
	// Remove newlines that are in the middle of sentences
	// This preserves paragraph breaks (double newlines) and intentional line breaks

	// First normalize line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	// Split into paragraphs (preserve double newlines)
	paragraphs := strings.Split(text, "\n\n")

	var cleanedParagraphs []string
	for _, paragraph := range paragraphs {
		// For each paragraph, remove single newlines that break sentences
		lines := strings.Split(paragraph, "\n")
		var cleanedLines []string

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// If this is not the first line and the previous line doesn't end with
			// sentence-ending punctuation, and this line doesn't start with a capital letter
			// or bullet point, then join it with the previous line
			if len(cleanedLines) > 0 {
				prevLine := cleanedLines[len(cleanedLines)-1]

				// Check if previous line ends with sentence-ending punctuation
				endsWithPunctuation := strings.HasSuffix(prevLine, ".") ||
					strings.HasSuffix(prevLine, "!") ||
					strings.HasSuffix(prevLine, "?") ||
					strings.HasSuffix(prevLine, ":") ||
					strings.HasSuffix(prevLine, ";")

				// Check if current line starts with capital letter, number, or bullet
				startsNewSentence := len(line) > 0 &&
					(line[0] >= 'A' && line[0] <= 'Z' ||
						line[0] >= '0' && line[0] <= '9' ||
						strings.HasPrefix(line, "- ") ||
						strings.HasPrefix(line, "* ") ||
						strings.HasPrefix(line, "• "))

				// If previous line doesn't end with punctuation and current line doesn't start new sentence,
				// join them with a space
				if !endsWithPunctuation && !startsNewSentence {
					cleanedLines[len(cleanedLines)-1] = prevLine + " " + line
					continue
				}
			}

			cleanedLines = append(cleanedLines, line)
		}

		if len(cleanedLines) > 0 {
			cleanedParagraphs = append(cleanedParagraphs, strings.Join(cleanedLines, "\n"))
		}
	}

	result := strings.Join(cleanedParagraphs, "\n\n")

	// Clean up multiple spaces
	for strings.Contains(result, "  ") {
		result = strings.ReplaceAll(result, "  ", " ")
	}

	return strings.TrimSpace(result)
}
