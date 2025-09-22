package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	Browser    BrowserConfig    `toml:"browser"`
	Extraction ExtractionConfig `toml:"extraction"`
	Output     OutputConfig     `toml:"output"`
	Network    NetworkConfig    `toml:"network"`
	Parallel   ParallelConfig   `toml:"parallel"`
	Pipe       PipeConfig       `toml:"pipe"`
	Logging    LoggingConfig    `toml:"logging"`
}

type BrowserConfig struct {
	Default string               `toml:"default"`
	Paths   map[string]string    `toml:"paths"`
	Cookies BrowserCookiesConfig `toml:"cookies"`
}

type BrowserCookiesConfig struct {
	Domains []string `toml:"domains"`
	Exclude []string `toml:"exclude"`
}

type ExtractionConfig struct {
	SkipCookieBanners bool   `toml:"skip_cookie_banners"`
	BannerTimeout     int    `toml:"banner_timeout"`
	EnableJavaScript  string `toml:"enable_javascript"`
	JSTimeout         int    `toml:"js_timeout"`
	WaitForSelector   string `toml:"wait_for_selector"`
	MinContentLength  int    `toml:"min_content_length"`
	RemoveAds         bool   `toml:"remove_ads"`
	CleanHTML         bool   `toml:"clean_html"`
}

type OutputConfig struct {
	DefaultFormat   string   `toml:"default_format"`
	IncludeMetadata bool     `toml:"include_metadata"`
	MetadataFields  []string `toml:"metadata_fields"`
	LineWidth       int      `toml:"line_width"`
	PreserveLinks   bool     `toml:"preserve_links"`
}

type NetworkConfig struct {
	Timeout         int    `toml:"timeout"`
	UserAgent       string `toml:"user_agent"`
	FollowRedirects bool   `toml:"follow_redirects"`
	MaxRedirects    int    `toml:"max_redirects"`
	Delay           int    `toml:"delay"`
}

type ParallelConfig struct {
	MaxConcurrency  int  `toml:"max_concurrency"`
	BatchSize       int  `toml:"batch_size"`
	ShowProgress    bool `toml:"show_progress"`
	FailFast        bool `toml:"fail_fast"`
	MaxMemoryMB     int  `toml:"max_memory_mb"`
	CleanupInterval int  `toml:"cleanup_interval"`
}

type PipeConfig struct {
	BufferSize      int    `toml:"buffer_size"`
	OutputSeparator string `toml:"output_separator"`
	NullSeparator   bool   `toml:"null_separator"`
	StreamMode      bool   `toml:"stream_mode"`
}

type LoggingConfig struct {
	Level string `toml:"level"`
	File  string `toml:"file"`
}

func Default() *Config {
	return &Config{
		Browser: BrowserConfig{
			Default: "auto",
			Paths:   map[string]string{},
			Cookies: BrowserCookiesConfig{
				Domains: []string{"*"},
				Exclude: []string{},
			},
		},
		Extraction: ExtractionConfig{
			SkipCookieBanners: true,
			BannerTimeout:     5,
			EnableJavaScript:  "auto",
			JSTimeout:         15,
			WaitForSelector:   "",
			MinContentLength:  100,
			RemoveAds:         true,
			CleanHTML:         true,
		},
		Output: OutputConfig{
			DefaultFormat:   "text",
			IncludeMetadata: false,
			MetadataFields:  []string{"title", "author", "date", "url"},
			LineWidth:       80,
			PreserveLinks:   true,
		},
		Network: NetworkConfig{
			Timeout:         30,
			UserAgent:       "",
			FollowRedirects: true,
			MaxRedirects:    10,
			Delay:           0,
		},
		Parallel: ParallelConfig{
			MaxConcurrency:  5,
			BatchSize:       0,
			ShowProgress:    true,
			FailFast:        false,
			MaxMemoryMB:     512,
			CleanupInterval: 30,
		},
		Pipe: PipeConfig{
			BufferSize:      4096,
			OutputSeparator: "---",
			NullSeparator:   false,
			StreamMode:      true,
		},
		Logging: LoggingConfig{
			Level: "info",
			File:  "",
		},
	}
}

func Load(configFile string) (*Config, error) {
	cfg := Default()

	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return cfg, fmt.Errorf("error finding home directory: %w", err)
			}
			configHome = filepath.Join(home, ".config")
		}

		configDir := filepath.Join(configHome, "scrpr")
		viper.AddConfigPath(configDir)
		viper.SetConfigType("toml")
		viper.SetConfigName("config")

		// Create config directory if it doesn't exist
		if err := os.MkdirAll(configDir, 0755); err != nil {
			return cfg, fmt.Errorf("error creating config directory: %w", err)
		}
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("SCRPR")

	if err := viper.ReadInConfig(); err != nil {
		// Config file not found is not an error, we'll use defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return cfg, fmt.Errorf("error reading config file: %w", err)
		}
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return cfg, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return cfg, nil
}

func (c *Config) CreateExampleConfig(configPath string) error {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	exampleContent := `# scrpr configuration file

[browser]
# Default browser for cookie extraction
default = "auto"  # auto, chrome, firefox, safari, zen

# Specific browser paths (optional, auto-detected if empty)
[browser.paths]
chrome = ""
firefox = ""
safari = ""
zen = ""

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
`

	return os.WriteFile(configPath, []byte(exampleContent), 0644)
}
