# scrpr

A fast, clean CLI tool for extracting main content from websites. Supports JavaScript rendering, browser cookie integration, and UNIX pipe operations.

## Features

- **Clean Content Extraction**: Uses readability algorithms to extract main article content with intelligent newline cleaning
- **JavaScript Support**: Handles dynamic websites with ChromeDP (coming soon)
- **Browser Integration**: Extract and use cookies from Chrome, Firefox, Safari, and Zen browsers
- **Pipe-Friendly**: Full UNIX pipe support for command chaining
- **Multiple Formats**: Output as clean text or Markdown
- **Parallel Processing**: Process multiple URLs concurrently (coming soon)
- **Cookie Banner Handling**: Automatically dismiss cookie banners (coming soon)

## Installation

### From Source

```bash
git clone https://github.com/byteowlz/scrpr.git
cd scrpr
go build -o scrpr cmd/scrpr/main.go
```

### Binary Releases (Coming Soon)

Pre-built binaries will be available for Linux, macOS, and Windows.

## Quick Start

```bash
# Extract content from a single URL (with clean text flow)
scrpr https://openshovelshack.com/blog/the-octopus-and-the-rake

# Output as Markdown
scrpr https://example.com --format markdown

# Process multiple URLs
scrpr https://example.com https://news.ycombinator.com

# Use pipes
echo "https://example.com" | scrpr
cat urls.txt | scrpr --format markdown
```

## Usage

### Basic Syntax

```bash
scrpr [urls...] [flags]
```

### Arguments

- `urls` - Website URLs to extract content from (space-separated)
- If no URLs provided, reads from stdin

### Flags

#### Input/Output

- `-f, --file string` - Read URLs from file (one per line)
- `-o, --output string` - Output to file or directory (default: stdout)
- `--format string` - Output format: text, markdown (default: text)
- `--separator string` - Output separator for multiple URLs (default: "---")
- `--null-separator` - Use null byte separator (for xargs -0)

#### Browser Integration

- `-b, --browser string` - Browser for cookie extraction: chrome, firefox, safari, zen (default: auto)

#### Content Processing

- `--include-metadata` - Include page metadata in output
- `--user-agent string` - Custom user agent string

#### Network

- `--timeout int` - Request timeout in seconds (default: 30)

#### System

- `-v, --verbose` - Verbose logging
- `--config string` - Custom config file path

## Examples

### Basic Usage

```bash
# Simple content extraction
scrpr https://news.ycombinator.com

# Save as markdown file
scrpr https://example.com --format markdown -o article.md

# Include metadata
scrpr https://example.com --include-metadata
```

### Multiple URLs

```bash
# Process multiple URLs
scrpr https://example.com https://news.ycombinator.com

# Custom separator
scrpr url1 url2 --separator "==="

# From file
scrpr -f urls.txt --format markdown
```

### Pipe Operations

```bash
# Basic pipe input
echo "https://example.com" | scrpr

# Chain with other commands
cat bookmarks.txt | scrpr | grep "important"

# Process API responses
curl -s "api.example.com/articles" | jq -r '.urls[]' | scrpr

# Use with xargs
cat urls.txt | scrpr --null-separator | xargs -0 process_text

# Save individual articles
cat urls.txt | while read url; do
  scrpr "$url" --format markdown -o "$(basename "$url").md"
done
```

### Advanced Usage

```bash
# Custom timeout and user agent
scrpr https://example.com --timeout 60 --user-agent "MyBot/1.0"

# Verbose output
scrpr https://example.com --verbose

# Use cookies from specific browser
scrpr https://example.com --browser chrome
```

## Configuration

scrpr uses a TOML configuration file located at:

- `$XDG_CONFIG_HOME/scrpr/config.toml`
- `~/.config/scrpr/config.toml` (fallback)

### Example Configuration

```toml
[browser]
default = "auto"  # auto, chrome, firefox, safari, zen

[extraction]
skip_cookie_banners = true
min_content_length = 100
remove_ads = true

[output]
default_format = "text"  # text, markdown
include_metadata = false
preserve_links = true

[network]
timeout = 30
user_agent = ""
follow_redirects = true

[parallel]
max_concurrency = 5
show_progress = true

[logging]
level = "info"
```

See `examples/config.toml` for a complete configuration example.

## Browser Support

scrpr can extract and use cookies from:

- **Chrome/Chromium** - `~/.config/google-chrome/Default/Cookies`
- **Firefox** - `~/.mozilla/firefox/*/cookies.sqlite`  
- **Safari** (macOS only) - `~/Library/Cookies/Cookies.binarycookies`
- **Zen Browser** - `~/.zen/*/cookies.sqlite`

Cookie extraction is read-only and secure - no cookies are modified or stored.

## Output Formats

### Text Format (default)

Clean, readable text with intelligent newline cleaning. Removes unwanted line breaks that split sentences while preserving paragraph structure.

```bash
scrpr https://example.com --format text
```

**Newline Cleaning Features:**
- Automatically joins lines that break mid-sentence
- Preserves paragraph breaks and intentional formatting
- Maintains proper spacing between words
- Recognizes sentence-ending punctuation
- Handles bullet points and numbered lists correctly

### Markdown Format

Structured Markdown with preserved formatting and links.

```bash
scrpr https://example.com --format markdown
```

## Integration Examples

### Content Archiving

```bash
# Archive articles from RSS feed
curl -s "https://feeds.example.com/rss.xml" | \
  grep -oE 'https://[^<]+' | \
  scrpr --format markdown -o archive/
```

### Research Pipeline

```bash
# Extract and analyze content
cat research_urls.txt | \
  scrpr --format text | \
  grep -i "machine learning" | \
  sort | uniq -c
```

### Documentation Generation  

```bash
# Convert web pages to documentation
echo -e "url1\nurl2\nurl3" | \
  scrpr --format markdown | \
  pandoc -f markdown -t pdf > docs.pdf
```

## Development Status

✅ **Completed**

- CLI interface and argument parsing
- Content extraction with readability
- Intelligent newline cleaning for better text flow
- Pipe input/output support  
- Browser cookie extraction
- Text and Markdown output formats
- Configuration system

🚧 **In Progress**

- JavaScript rendering with ChromeDP
- Cookie banner dismissal
- Parallel processing
- Enhanced metadata extraction

📋 **Planned**

- Browser plugin interface
- Custom extraction rules
- Output to various formats (JSON, PDF)
- Web UI for configuration

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Similar Projects

- [Trafilatura](https://github.com/adbar/trafilatura) (Python) - Web scraping and content extraction
- [Mercury Parser](https://github.com/postlight/mercury-parser) (JavaScript) - Content extraction
- [newspaper3k](https://github.com/codelucas/newspaper) (Python) - Article scraping
