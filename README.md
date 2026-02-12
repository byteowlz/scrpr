# scrpr

A fast CLI tool for extracting main content from websites. Supports multiple
extraction backends (local readability, Tavily, Jina Reader), browser cookie
integration, and UNIX pipe operations.

## Features

- **Multiple extraction backends** - local readability (default), Tavily Extract API, Jina Reader API
- **Clean content extraction** using readability algorithms with intelligent newline cleaning
- **Pipe-friendly** - full UNIX pipe support, pairs with `sx` for search-to-content pipelines
- **Multiple output formats** - text or Markdown
- **Batch processing** - process multiple URLs with progress, rate limiting, and error resilience
- **Directory output** - save each URL to its own file with `-o dir/`
- **Browser cookie integration** - extract cookies from Chrome, Firefox, Safari, Zen
- **Quiet mode** - `-q` suppresses all non-content output for clean piping
- **Granular exit codes** - 0=ok, 1=network, 2=parse, 3=input, 4=config, 5=io, 6=partial

## Installation

```bash
git clone https://github.com/byteowlz/scrpr.git
cd scrpr
go build -o scrpr cmd/scrpr/main.go
```

## Quick Start

```bash
# Extract content from a URL
scrpr https://example.com

# Output as Markdown
scrpr https://example.com --format markdown

# Use Jina Reader for JS-heavy sites (no API key needed)
scrpr https://example.com -B jina

# Pipe from sx search
sx "query" -L -n 5 | scrpr --format markdown
```

## Extraction Backends

scrpr supports three extraction backends:

| Backend | Flag | Auth | Best For |
|---------|------|------|----------|
| **readability** (default) | `-B readability` | None | Fast, local, most sites |
| **tavily** | `-B tavily` | API key | JS-heavy sites, better quality |
| **jina** | `-B jina` | Optional | Free, no auth needed, decent quality |

### Readability (Default)

Local extraction using go-readability. No API key needed. Works for most sites.

```bash
scrpr https://example.com
```

### Tavily Extract API

Cloud-based extraction that handles JavaScript-heavy sites better.
Requires an API key ([free tier: 1,000 credits/month](https://tavily.com/)).

```bash
# Via config
# [extraction.tavily]
# api_key = "tvly-xxx"

# Via env var
export TAVILY_API_KEY="tvly-xxx"

scrpr https://js-heavy-site.com -B tavily --format markdown
```

### Jina Reader API

Free extraction via `r.jina.ai`. Works without an API key (rate limited).
Optional key for higher limits.

```bash
scrpr https://example.com -B jina --format markdown
```

## Usage

### Basic

```bash
# Single URL
scrpr https://example.com

# Multiple URLs
scrpr https://a.com https://b.com

# From file
scrpr -f urls.txt

# From pipe
echo "https://example.com" | scrpr
cat urls.txt | scrpr --format markdown
```

### Output Options

```bash
# Save to file
scrpr https://example.com -o article.md --format markdown

# Save each URL to its own file in a directory
scrpr https://a.com https://b.com -o articles/

# Include metadata
scrpr https://example.com --include-metadata
```

### Batch Processing

```bash
# Rate limiting
scrpr -f urls.txt --delay 0.5

# Continue on error
scrpr -f urls.txt --continue-on-error

# Progress indicator
scrpr -f urls.txt --progress

# Quiet mode (content only, no stderr)
scrpr -f urls.txt -q
```

### Pipelines with sx

```bash
# Search and extract content
sx "rust error handling" -L -n 5 | scrpr --format markdown

# Save to directory
sx "query" -L -n 5 | scrpr --format markdown -o articles/

# Use Jina for JS-heavy results
sx "query" -L -n 5 | scrpr -B jina --format markdown

# With rate limiting and error resilience
sx "query" -L -n 10 | scrpr --delay 0.5 --continue-on-error

# Quiet pipeline
sx "query" -L -n 5 | scrpr -q --format markdown > output.md
```

### All Flags

```
Flags:
  -B, --extract-backend string   extraction backend (readability, tavily, jina)
  -f, --file string              read URLs from file
  -o, --output string            output to file or directory
      --format string            text or markdown (default "text")
      --separator string         separator for multiple URLs (default "---")
      --null-separator           null byte separator (for xargs -0)
  -c, --concurrency int          max concurrent requests (default 5)
      --batch-size int           process in batches of N
      --progress                 show progress for batch processing
  -b, --browser string           browser for cookies (chrome/firefox/safari/zen)
      --javascript               force JS rendering
      --no-js                    disable JS rendering
      --skip-banners             skip cookie banners (default true)
      --timeout int              request timeout in seconds (default 30)
      --include-metadata         include page metadata
      --user-agent string        custom user agent
      --browser-agent string     browser agent type
      --continue-on-error        continue on URL failures
      --no-follow-redirects      disable HTTP redirects
      --delay float              seconds between requests
  -v, --verbose                  verbose output
  -q, --quiet                    suppress non-content output
      --config string            config file path
```

## Configuration

Config at `$XDG_CONFIG_HOME/scrpr/config.toml` (auto-created on first run).

```toml
[extraction]
backend = "readability"          # readability, tavily, jina
min_content_length = 100
remove_ads = true

[extraction.tavily]
api_key = ""                     # or set TAVILY_API_KEY env var
extract_depth = "basic"          # basic or advanced

[extraction.jina]
api_key = ""                     # optional, for higher rate limits

[output]
default_format = "text"
preserve_links = true

[network]
timeout = 30
browser_agent = "auto"
follow_redirects = true
delay = 0

[parallel]
max_concurrency = 5
show_progress = true
fail_fast = false
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Network error |
| 2 | Parse/extraction error |
| 3 | Invalid input |
| 4 | Config error |
| 5 | File I/O error |
| 6 | Partial success (some URLs failed) |

## License

MIT License
