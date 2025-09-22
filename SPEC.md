# scrpr - Website Content Extraction CLI

## Overview

scrpr is a command-line tool that extracts the main content from websites and outputs it to the terminal or saves it as a markdown file. It handles JavaScript-heavy sites, bypasses cookie banners, and can utilize browser cookies for authenticated content access.

## Core Features

### 1. Content Extraction
- Extract main article/content from web pages
- Support for both static and JavaScript-rendered content
- Intelligent content detection using readability algorithms
- Clean output with minimal noise (ads, navigation, footers removed)

### 2. Browser Integration
- Extract and utilize cookies from supported browsers
- Support for Chrome, Firefox, Safari, and Zen browsers
- Configurable browser selection
- Domain-specific cookie injection

### 3. JavaScript Rendering
- Automatic detection of JavaScript-heavy sites
- Chromium-based rendering engine for dynamic content
- Configurable timeout and rendering options
- Cookie banner detection and dismissal

### 4. Output Formats
- Clean text output to terminal
- Markdown file generation
- Configurable metadata inclusion
- UTF-8 encoding support

### 5. Parallel Processing
- Concurrent fetching of multiple URLs
- Configurable concurrency limits
- Load balancing across browser instances
- Progress reporting for batch operations

### 6. Pipe Support
- Read URLs from stdin (pipe input)
- Output content to stdout (pipe output) 
- Stream processing for large datasets
- UNIX-style command chaining compatibility

## Command Line Interface

### Basic Usage
```bash
scrpr <url>                    # Output to terminal
scrpr <url> -o article.md      # Save as markdown
scrpr <url> --format markdown  # Markdown to terminal

# Multiple URLs
scrpr url1 url2 url3           # Process multiple URLs
scrpr -f urls.txt              # Read URLs from file
scrpr url1 url2 -o dir/        # Save each to separate files

# Pipe Support
echo "https://example.com" | scrpr              # Single URL from pipe
cat urls.txt | scrpr                            # Multiple URLs from pipe
scrpr url1 | grep "keyword"                     # Pipe output to other commands
find . -name "*.html" | xargs scrpr             # Process file URLs
curl -s api.com/urls | jq -r '.[]' | scrpr      # Chain with other tools
```

### Flags
```bash
scrpr [urls...] [flags]

Arguments:
  urls                    Website URLs to extract content from (space-separated)
                          If no URLs provided, reads from stdin

Flags:
  # Input/Output
  -f, --file string       Read URLs from file (one per line)
  -o, --output string     Output to file or directory (default: stdout)
  --format string         Output format (text|markdown) (default: "text")
  --separator string      Output separator for multiple URLs (default: "---")
  --null-separator        Use null byte separator (for xargs -0)
  
  # Parallel Processing
  -c, --concurrency int   Max concurrent requests (default: 5)
  --batch-size int        Process URLs in batches of N (default: 0 = all at once)
  --progress              Show progress bar for multiple URLs
  
  # Browser Integration  
  -b, --browser string    Browser for cookie extraction (chrome|firefox|safari|zen) (default: "auto")
  
  # Rendering
  -js, --javascript       Force JavaScript rendering (default: auto-detect)
  --no-js                 Disable JavaScript rendering
  --skip-banners         Skip cookie banner dismissal (default: true)
  --timeout int          Request timeout in seconds (default: 30)
  
  # Content Processing
  --include-metadata     Include page metadata in output
  --user-agent string    Custom user agent string
  
  # System
  --verbose              Verbose logging
  --config string        Custom config file path
  -h, --help             Show help
  -v, --version          Show version
```

## Configuration

### Config File Location
- Primary: `$XDG_CONFIG_HOME/scrpr/config.toml`
- Fallback: `~/.config/scrpr/config.toml`
- Custom: `--config /path/to/config.toml`

### Default Configuration
```toml
[browser]
# Default browser for cookie extraction
default = "auto"  # auto, chrome, firefox, safari, zen

# Specific browser paths (optional)
[browser.paths]
chrome = ""     # Auto-detect if empty
firefox = ""    # Auto-detect if empty
safari = ""     # Auto-detect if empty
zen = ""        # Auto-detect if empty

# Domain patterns for cookie injection
[browser.cookies]
domains = ["*"]  # Inject cookies for all domains by default
exclude = []     # Domains to exclude from cookie injection

[extraction]
# Cookie banner handling
skip_cookie_banners = true
banner_timeout = 5  # seconds to wait for banner dismissal

# JavaScript rendering
enable_javascript = "auto"  # auto, always, never
js_timeout = 15            # seconds to wait for JS execution
wait_for_selector = ""     # CSS selector to wait for (optional)

# Content extraction
min_content_length = 100   # Minimum content length to consider valid
remove_ads = true          # Remove advertisement blocks
clean_html = true          # Clean HTML before processing

[output]
# Default output format
default_format = "text"    # text, markdown

# Metadata inclusion
include_metadata = false
metadata_fields = ["title", "author", "date", "url"]

# Text formatting
line_width = 80           # Max line width for text output (0 = unlimited)
preserve_links = true     # Keep links in markdown output

[network]
# Request settings
timeout = 30              # seconds
user_agent = ""           # Custom user agent (empty = default)
follow_redirects = true
max_redirects = 10

# Rate limiting
delay = 0                 # seconds between requests (for multiple URLs)

[parallel]
# Parallel processing settings
max_concurrency = 5       # Maximum concurrent requests
batch_size = 0            # Process in batches (0 = process all at once)
show_progress = true      # Show progress bar for multiple URLs
fail_fast = false         # Stop on first error (false = continue processing)

# Resource management
max_memory_mb = 512       # Maximum memory usage in MB
cleanup_interval = 30     # Clean up resources every N seconds

[pipe]
# Pipe handling settings
buffer_size = 4096        # Input buffer size for reading from pipes
output_separator = "---"  # Separator between multiple URL outputs
null_separator = false    # Use null bytes as separators
stream_mode = true        # Process URLs as they arrive (vs batch mode)

[logging]
level = "info"            # debug, info, warn, error
file = ""                 # Log file path (empty = stderr only)
```

## Architecture

### Core Components

#### 1. Browser Cookie Extractor
- **Libraries**: `kooky` (primary), custom Zen support
- **Function**: Extract cookies from browser storage
- **Browsers**: Chrome, Firefox, Safari, Zen
- **Output**: Cookie jar for HTTP requests

#### 2. Content Fetcher
- **Libraries**: `chromedp` (JS rendering), `net/http` (static)
- **Function**: Fetch page content with optional JavaScript execution
- **Features**: Cookie injection, banner dismissal, timeout handling

#### 3. Content Processor
- **Libraries**: `readability`, `goquery`
- **Function**: Extract main content from HTML
- **Features**: Content cleaning, metadata extraction, format conversion

#### 4. Output Manager
- **Function**: Format and output extracted content
- **Formats**: Plain text, Markdown
- **Targets**: stdout, file, directory (for multiple URLs)

#### 5. Parallel Coordinator
- **Libraries**: Go goroutines, `sync.WaitGroup`, worker pools
- **Function**: Manage concurrent URL processing
- **Features**: Rate limiting, resource management, progress tracking

#### 6. Pipe Handler
- **Libraries**: `bufio.Scanner`, `os.Stdin`
- **Function**: Handle stdin/stdout pipe operations
- **Features**: Stream processing, buffered I/O, separator handling

### Data Flow
```
URL Input (args/file/pipe) → URL Queue → Cookie Extraction → Content Fetching → Content Processing → Output Generation (stdout/file/pipe)
```

### Pipe Processing Flow
```
stdin → URL Parser → URL Queue → Parallel Processor → Content Formatter → stdout
```

## Browser Support

### Chrome/Chromium
- Cookie database: `~/.config/google-chrome/Default/Cookies`
- Format: SQLite database
- Library: `kooky.ChromeCookies()`

### Firefox
- Cookie database: `~/.mozilla/firefox/*/cookies.sqlite`
- Format: SQLite database  
- Library: `kooky.FirefoxCookies()`

### Safari
- Cookie database: `~/Library/Cookies/Cookies.binarycookies`
- Format: Binary plist
- Library: `kooky.SafariCookies()`

### Zen Browser
- Cookie database: Similar to Firefox (`~/.zen/*/cookies.sqlite`)
- Format: SQLite database
- Implementation: Custom profile detection + Firefox cookie reader

## JavaScript Rendering

### Auto-Detection Triggers
- Presence of popular SPA frameworks (React, Vue, Angular)
- Minimal initial content with loading indicators
- Heavy reliance on `<script>` tags for content
- Meta tags indicating SPA (`name="generator"`, etc.)

### Banner Dismissal Strategy
```javascript
// Common selectors for cookie banners
const bannerSelectors = [
  '[id*="cookie"]', '[class*="cookie"]',
  '[id*="consent"]', '[class*="consent"]', 
  '[id*="gdpr"]', '[class*="gdpr"]',
  '.cookie-banner', '.consent-banner',
  '#cookieConsent', '#cookie-notice'
];

// Dismissal actions
const dismissActions = [
  'click button[id*="accept"]',
  'click button[class*="accept"]',
  'click .cookie-accept',
  'click [data-action="accept"]'
];
```

## Content Extraction Algorithm

### 1. HTML Preprocessing
- Remove script tags and style blocks
- Clean HTML entities
- Normalize whitespace

### 2. Content Scoring
- Apply readability algorithm
- Score content blocks by:
  - Text length
  - Paragraph density
  - Link density
  - HTML tag semantics

### 3. Content Selection
- Select highest-scoring content blocks
- Merge adjacent high-scoring blocks
- Apply minimum length threshold

### 4. Post-processing
- Remove remaining ads/navigation
- Clean formatting
- Extract metadata

## Error Handling

### Network Errors
- Timeout handling with configurable limits
- Retry logic for temporary failures
- Graceful fallback for JavaScript rendering failures

### Content Errors
- Handle empty or invalid content
- Fallback to alternative extraction methods
- Clear error messages for debugging

### Browser Integration Errors
- Handle missing browser installations
- Graceful fallback when cookies unavailable
- Cross-platform path detection

## Pipe Integration Examples

### Common Use Cases
```bash
# Extract content from bookmarks
cat bookmarks.txt | scrpr --format markdown > articles/

# Process URLs from API responses
curl -s "api.example.com/articles" | jq -r '.urls[]' | scrpr

# Filter and process URLs
grep "news" urls.txt | scrpr | grep "breaking"

# Batch process with xargs
echo -e "url1\nurl2\nurl3" | scrpr --null-separator | xargs -0 process_text

# Chain with other text processing tools
scrpr url1 url2 | pandoc -f markdown -t pdf > articles.pdf

# Save individual articles with generated filenames
cat urls.txt | scrpr -o articles/ --format markdown

# Stream processing large URL lists
curl -s "huge-url-list.txt" | scrpr --stream-mode
```

### Input Formats
- **Line-separated URLs**: Default format, one URL per line
- **Space-separated URLs**: Command line arguments
- **Null-separated URLs**: For use with `xargs -0`
- **JSON arrays**: Parse URLs from JSON with external tools

### Output Formats for Pipes
- **Raw text**: Clean extracted content
- **Markdown**: Formatted with headers and metadata
- **Separated**: Multiple URLs separated by `---` or custom separator
- **JSON**: Structured output for further processing

## Future Extensions

### Browser Plugin Interface
- WebSocket communication channel
- Real-time cookie sharing
- Enhanced banner dismissal
- Content pre-processing

### Plugin API
```javascript
// Browser extension communication
const scrprAPI = {
  getCookies: (domain) => Promise<Cookie[]>,
  dismissBanners: () => Promise<boolean>,
  waitForContent: (selector) => Promise<boolean>,
  extractContent: () => Promise<string>
};
```

## Installation & Distribution

### Go Module
```bash
go install github.com/username/scrpr@latest
```

### Binary Releases
- Cross-platform binaries for Linux, macOS, Windows
- Homebrew formula for macOS
- Snap package for Linux

### Dependencies
- Go 1.21+
- Chrome/Chromium for JavaScript rendering
- No external dependencies required

## Security Considerations

### Cookie Handling
- Read-only access to browser cookie stores
- No cookie modification or storage
- Respect browser security boundaries
- Clear cookie data after use

### Network Security
- HTTPS-first approach
- Certificate validation
- No credential storage
- User-agent transparency

### Data Privacy
- No telemetry or analytics
- Local-only processing
- No content caching beyond session
- Configurable logging levels

## Testing Strategy

### Unit Tests
- Cookie extraction for each browser
- Content processing algorithms
- Configuration parsing
- Output formatting

### Integration Tests
- End-to-end content extraction
- JavaScript rendering scenarios
- Error handling paths
- Cross-platform compatibility
- Pipe input/output scenarios
- Parallel processing with various concurrency levels

### Test Sites
- Static content sites
- JavaScript-heavy SPAs
- Sites with cookie banners
- Paywalled content (with valid cookies)