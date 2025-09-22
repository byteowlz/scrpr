package processor

import (
	"fmt"
	"io"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-shiori/go-readability"
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

func (cp *ContentProcessor) removeAds(content string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return content
	}

	// Common ad selectors
	adSelectors := []string{
		"[id*='ad']",
		"[class*='ad']",
		"[id*='advertisement']",
		"[class*='advertisement']",
		"[id*='sponsor']",
		"[class*='sponsor']",
		"[id*='promo']",
		"[class*='promo']",
		".google-ads",
		".adsystem",
		".ad-container",
		".advertisement",
		".sponsored",
		".banner-ad",
	}

	for _, selector := range adSelectors {
		doc.Find(selector).Remove()
	}

	// Remove elements with ad-related text
	doc.Find("*").Each(func(i int, s *goquery.Selection) {
		text := strings.ToLower(s.Text())
		if strings.Contains(text, "advertisement") ||
			strings.Contains(text, "sponsored content") ||
			(strings.Contains(text, "ad") && len(strings.TrimSpace(s.Text())) < 50) {
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

	// Convert HTML content to markdown-like format
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content.Content))
	if err != nil {
		// Fallback to text content if HTML parsing fails
		md.WriteString(content.TextContent)
		return md.String()
	}

	// Find the body content or use the whole document
	bodyContent := doc.Find("body")
	if bodyContent.Length() == 0 {
		bodyContent = doc.Selection
	}

	cp.convertToMarkdown(bodyContent, &md, preserveLinks)

	// If no content was generated, fallback to text content
	result := md.String()
	if strings.TrimSpace(result) == strings.TrimSpace(fmt.Sprintf("# %s", content.Title)) {
		md.WriteString(cp.CleanNewlines(content.TextContent))
		result = md.String()
	}

	return result
}

func (cp *ContentProcessor) convertToMarkdown(sel *goquery.Selection, md *strings.Builder, preserveLinks bool) {
	sel.Contents().Each(func(i int, s *goquery.Selection) {
		node := s.Get(0)
		if node.Type == 1 { // Element node
			tagName := strings.ToLower(node.Data)

			switch tagName {
			case "h1", "h2", "h3", "h4", "h5", "h6":
				level := int(tagName[1] - '0')
				md.WriteString(fmt.Sprintf("%s %s\n\n", strings.Repeat("#", level), strings.TrimSpace(s.Text())))
			case "p":
				// Process paragraph content recursively to handle nested elements
				var pContent strings.Builder
				cp.convertToMarkdown(s, &pContent, preserveLinks)
				text := strings.TrimSpace(pContent.String())
				if text != "" {
					md.WriteString(fmt.Sprintf("%s\n\n", text))
				}
			case "br":
				md.WriteString("\n")
			case "a":
				if preserveLinks {
					href, exists := s.Attr("href")
					if exists && href != "" {
						md.WriteString(fmt.Sprintf("[%s](%s)", s.Text(), href))
					} else {
						md.WriteString(s.Text())
					}
				} else {
					md.WriteString(s.Text())
				}
			case "strong", "b":
				md.WriteString(fmt.Sprintf("**%s**", s.Text()))
			case "em", "i":
				md.WriteString(fmt.Sprintf("*%s*", s.Text()))
			case "code":
				md.WriteString(fmt.Sprintf("`%s`", s.Text()))
			case "pre":
				md.WriteString(fmt.Sprintf("```\n%s\n```\n\n", s.Text()))
			case "blockquote":
				lines := strings.Split(s.Text(), "\n")
				for _, line := range lines {
					if strings.TrimSpace(line) != "" {
						md.WriteString(fmt.Sprintf("> %s\n", strings.TrimSpace(line)))
					}
				}
				md.WriteString("\n")
			case "ul", "ol":
				cp.convertList(s, md, tagName == "ol", 0)
			case "img":
				if src, exists := s.Attr("src"); exists {
					alt := s.AttrOr("alt", "")
					md.WriteString(fmt.Sprintf("![%s](%s)\n\n", alt, src))
				}
			case "div", "section", "article", "main", "header", "footer", "aside", "nav":
				// For container elements, just process their contents
				cp.convertToMarkdown(s, md, preserveLinks)
			default:
				// For unknown elements, process their contents
				cp.convertToMarkdown(s, md, preserveLinks)
			}
		} else if node.Type == 3 { // Text node
			text := strings.TrimSpace(node.Data)
			if text != "" {
				md.WriteString(text)
			}
		}
	})
}

func (cp *ContentProcessor) convertList(sel *goquery.Selection, md *strings.Builder, ordered bool, depth int) {
	prefix := strings.Repeat("  ", depth)

	sel.Find("li").Each(func(i int, s *goquery.Selection) {
		marker := "- "
		if ordered {
			marker = fmt.Sprintf("%d. ", i+1)
		}

		md.WriteString(fmt.Sprintf("%s%s%s\n", prefix, marker, strings.TrimSpace(s.Text())))

		// Handle nested lists
		s.Find("ul, ol").Each(func(j int, nested *goquery.Selection) {
			cp.convertList(nested, md, nested.Is("ol"), depth+1)
		})
	})

	md.WriteString("\n")
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
