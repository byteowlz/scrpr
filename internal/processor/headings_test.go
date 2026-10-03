package processor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-shiori/go-readability"
)

// cssModuleDocsPage mimics a docs site (e.g. ghostty.org) whose section headings
// sit in wrappers with CSS-module class names containing "Header". Readability's
// unlikely-candidate regex matches the "header" substring and drops them all.
func cssModuleDocsPage(sections int) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html><html><head><title>Action Reference</title></head><body>
<nav class="Sidebar-module__q1__nav"><ul><li><a href="/a">Docs</a></li><li><a href="/b">Config</a></li></ul></nav>
<main class="DocsPage-module__x9__main"><div class="Content-module__k2__content">
<h1 class="Text-module__t1__text">Action Reference</h1>
<p class="Text-module__t1__body">This is a reference of all keybinding actions and what each of them does.</p>`)
	for i := 0; i < sections; i++ {
		name := fmt.Sprintf("action_%d", i)
		fmt.Fprintf(&b, `<div class="JumplinkHeader-module__S1__jumplinkHeader" id="%s">
<div class="JumplinkHeader-module__S1__content"><h2 class="JumplinkHeader-module__S1__text"><code>%s</code></h2>
<a href="#%s" class="JumplinkHeader-module__S1__jumplinkCopy"></a></div></div>
<p class="Text-module__t1__body">Description of %s, which performs a specific operation on the terminal surface when it is triggered by a keybinding.</p>
`, name, name, name, name)
	}
	b.WriteString(`</div></main><footer class="Footer-module__f1__footer"><p>Copyright</p></footer></body></html>`)
	return b.String()
}

func TestReadabilityDropsCSSModuleHeadings(t *testing.T) {
	// Guards the premise: if go-readability stops dropping these headings,
	// recoverHeadings is no longer needed for this case.
	article, err := readability.FromReader(strings.NewReader(cssModuleDocsPage(12)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n := countHeadings(article.Content); n*2 >= 12 {
		t.Skipf("readability now keeps %d/12 headings; premise no longer holds", n)
	}
}

func TestProcessRecoversDroppedHeadings(t *testing.T) {
	const sections = 12
	cp := NewContentProcessor()
	p, err := cp.Process(cssModuleDocsPage(sections), "http://example.com/", ProcessOptions{
		RemoveAds:        true,
		CleanHTML:        true,
		MinContentLength: 100,
	})
	if err != nil {
		t.Fatal(err)
	}

	md := cp.ToMarkdown(p, false, true)
	for i := 0; i < sections; i++ {
		heading := fmt.Sprintf("## `action_%d`", i)
		if !strings.Contains(md, heading) {
			t.Errorf("markdown missing heading %q", heading)
		}
	}
	if !strings.Contains(md, `Description of action\_11`) {
		t.Errorf("markdown missing section body")
	}
	if strings.Contains(md, "Copyright") {
		t.Errorf("markdown includes footer outside the main container")
	}
	if t.Failed() {
		t.Logf("markdown:\n%s", md)
	}
	if !strings.Contains(p.TextContent, "action_5") {
		t.Errorf("text content missing heading text")
	}
}

func TestRecoverHeadingsKeepsGoodExtraction(t *testing.T) {
	html := `<!DOCTYPE html><html><head><title>Guide</title></head><body><article>
<h1>Guide</h1>
<h2>Install</h2><p>Install the tool with your package manager of choice and verify the version afterwards.</p>
<h2>Configure</h2><p>Create a config file in the XDG config directory and adjust the defaults to taste.</p>
<h2>Run</h2><p>Run the tool against a URL and inspect the extracted markdown output on stdout.</p>
<h2>Troubleshoot</h2><p>When extraction fails, enable verbose logging and retry with another backend.</p>
</article></body></html>`
	article, err := readability.FromReader(strings.NewReader(html), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := recoverHeadings(html, article); got != nil {
		t.Errorf("expected original extraction to be kept, got replacement")
	}
}

func TestRecoverHeadingsIgnoresPagesWithFewHeadings(t *testing.T) {
	html := `<html><head><title>Note</title></head><body><main>
<div class="PageHeader-module__a__header"><h2>Only heading</h2></div>
<p>A short note with a single heading wrapped in a CSS-module header class name.</p>
</main></body></html>`
	article, err := readability.FromReader(strings.NewReader(html), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := recoverHeadings(html, article); got != nil {
		t.Errorf("expected no recovery below %d source headings", minSourceHeadings)
	}
}
