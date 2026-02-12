package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/byteowlz/scrpr/internal/config"
	"github.com/byteowlz/scrpr/internal/extractor"
	"github.com/byteowlz/scrpr/internal/fetcher"
	"github.com/byteowlz/scrpr/internal/processor"
)

// Exit codes for granular error handling
const (
	ExitSuccess       = 0
	ExitNetworkError  = 1
	ExitProcessError  = 2
	ExitInvalidInput  = 3
	ExitConfigError   = 4
	ExitFileIOError   = 5
	ExitPartialError  = 6 // some URLs failed, some succeeded
)

var (
	cfgFile            string
	outputFile         string
	outputFormat       string
	browser            string
	browserAgent       string
	javascript         bool
	noJS               bool
	skipBanners        bool
	timeout            int
	concurrency        int
	batchSize          int
	progress           bool
	separator          string
	nullSeparator      bool
	userAgent          string
	includeMetadata    bool
	verbose            bool
	quiet              bool
	file               string
	continueOnError    bool
	noFollowRedirects  bool
	delay              float64
	extractBackend     string
)

const version = "1.1.0"

var rootCmd = &cobra.Command{
	Use:     "scrpr [urls...]",
	Short:   "Extract main content from websites",
	Long:    `scrpr is a CLI tool that extracts the main content from websites.
It supports multiple extraction backends, browser cookie integration, and pipe operations.`,
	Version:       version,
	RunE:          run,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		if exitErr, ok := err.(*exitErr); ok {
			os.Exit(exitErr.code)
		}
		os.Exit(ExitInvalidInput)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: $XDG_CONFIG_HOME/scrpr/config.toml)")

	// Input/Output flags
	rootCmd.Flags().StringVarP(&file, "file", "f", "", "read URLs from file (one per line)")
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "", "output to file or directory (default: stdout)")
	rootCmd.Flags().StringVar(&outputFormat, "format", "text", "output format (text|markdown)")
	rootCmd.Flags().StringVar(&separator, "separator", "---", "output separator for multiple URLs")
	rootCmd.Flags().BoolVar(&nullSeparator, "null-separator", false, "use null byte separator (for xargs -0)")

	// Parallel processing flags
	rootCmd.Flags().IntVarP(&concurrency, "concurrency", "c", 5, "max concurrent requests")
	rootCmd.Flags().IntVar(&batchSize, "batch-size", 0, "process URLs in batches of N (0 = all at once)")
	rootCmd.Flags().BoolVar(&progress, "progress", false, "show progress bar for multiple URLs")

	// Browser integration flags
	rootCmd.Flags().StringVarP(&browser, "browser", "b", "auto", "browser for cookie extraction (chrome|firefox|safari|zen)")

	// Rendering flags
	rootCmd.Flags().BoolVar(&javascript, "javascript", false, "force JavaScript rendering")
	rootCmd.Flags().BoolVar(&noJS, "no-js", false, "disable JavaScript rendering")
	rootCmd.Flags().BoolVar(&skipBanners, "skip-banners", true, "skip cookie banner dismissal")
	rootCmd.Flags().IntVar(&timeout, "timeout", 30, "request timeout in seconds")

	// Content processing flags
	rootCmd.Flags().BoolVar(&includeMetadata, "include-metadata", false, "include page metadata in output")
	rootCmd.Flags().StringVar(&userAgent, "user-agent", "", "custom user agent string")
	rootCmd.Flags().StringVar(&browserAgent, "browser-agent", "", "browser agent type (auto|chrome|firefox|safari|edge)")

	// Pipeline flags
	rootCmd.Flags().BoolVar(&continueOnError, "continue-on-error", false, "continue processing remaining URLs on error")
	rootCmd.Flags().BoolVar(&noFollowRedirects, "no-follow-redirects", false, "disable following HTTP redirects")
	rootCmd.Flags().Float64Var(&delay, "delay", 0, "delay in seconds between requests (rate limiting)")

	// Extraction backend flags
	rootCmd.Flags().StringVarP(&extractBackend, "extract-backend", "B", "", "extraction backend (readability, tavily, jina)")

	// System flags
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
	rootCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress all non-content output")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Error finding home directory: %v\n", err)
				}
				return
			}
			configHome = filepath.Join(home, ".config")
		}

		configDir := filepath.Join(configHome, "scrpr")
		viper.AddConfigPath(configDir)
		viper.SetConfigType("toml")
		viper.SetConfigName("config")

		// Create config directory if it doesn't exist
		// Handle broken symlinks by removing them first
		if fi, lstatErr := os.Lstat(configDir); lstatErr == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				if _, statErr := os.Stat(configDir); os.IsNotExist(statErr) {
					os.Remove(configDir) // broken symlink
				}
			}
		}
		if mkdirErr := os.MkdirAll(configDir, 0755); mkdirErr != nil && !os.IsExist(mkdirErr) {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Error creating config directory: %v\n", mkdirErr)
			}
		}
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("SCRPR")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Auto-create config on first run
			configPath := getDefaultConfigPath()
			if configPath != "" {
				cfg := config.Default()
				if createErr := cfg.CreateExampleConfig(configPath); createErr == nil {
					if !quiet {
						fmt.Fprintf(os.Stderr, "Created config file: %s\n", configPath)
					}
					// Re-read the newly created config
					viper.ReadInConfig()
				}
			}
		} else if verbose && !quiet {
			fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
		}
	} else if verbose && !quiet {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func getDefaultConfigPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "scrpr", "config.toml")
}

func run(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return exitError(ExitConfigError, "failed to load config: %v", err)
	}

	// Apply config defaults if CLI flags not explicitly set
	if !cmd.Flags().Changed("delay") && cfg.Network.Delay > 0 {
		delay = float64(cfg.Network.Delay)
	}
	if !cmd.Flags().Changed("concurrency") {
		concurrency = cfg.Parallel.MaxConcurrency
	}
	if !cmd.Flags().Changed("continue-on-error") {
		continueOnError = !cfg.Parallel.FailFast
	}
	if !cmd.Flags().Changed("progress") {
		progress = cfg.Parallel.ShowProgress
	}
	if !cmd.Flags().Changed("no-follow-redirects") && !cfg.Network.FollowRedirects {
		noFollowRedirects = true
	}
	if !cmd.Flags().Changed("format") && cfg.Output.DefaultFormat != "" {
		outputFormat = cfg.Output.DefaultFormat
	}
	if !cmd.Flags().Changed("extract-backend") && cfg.Extraction.Backend != "" {
		extractBackend = cfg.Extraction.Backend
	}

	// Collect URLs from various sources
	urls, err := collectURLs(args)
	if err != nil {
		return exitError(ExitInvalidInput, "failed to collect URLs: %v", err)
	}

	if len(urls) == 0 {
		return exitError(ExitInvalidInput, "no URLs provided")
	}

	if verbose && !quiet {
		fmt.Fprintf(os.Stderr, "Processing %d URLs\n", len(urls))
	}

	// Set up output writer
	var output io.Writer = os.Stdout
	var outputDir string
	var singleFileOutput *os.File

	if outputFile != "" {
		// Check if output is a directory (ends with / or already exists as dir)
		info, statErr := os.Stat(outputFile)
		if (statErr == nil && info.IsDir()) || strings.HasSuffix(outputFile, "/") {
			// Directory mode: each URL gets its own file
			outputDir = outputFile
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return exitError(ExitFileIOError, "failed to create output directory: %v", err)
			}
		} else {
			// Single file mode
			singleFileOutput, err = os.Create(outputFile)
			if err != nil {
				return exitError(ExitFileIOError, "failed to create output file %s: %v", outputFile, err)
			}
			defer singleFileOutput.Close()
			output = singleFileOutput
		}
	}

	hadError := false
	successCount := 0

	// Process URLs
	for i, url := range urls {
		if verbose && !quiet {
			fmt.Fprintf(os.Stderr, "Processing [%d/%d]: %s\n", i+1, len(urls), url)
		}

		// Show progress
		if progress && !quiet && len(urls) > 1 {
			pct := float64(i) / float64(len(urls)) * 100
			fmt.Fprintf(os.Stderr, "\r[%3.0f%%] %d/%d URLs processed", pct, i, len(urls))
		}

		result, err := processURL(url, cfg)
		if err != nil {
			hadError = true
			if !quiet {
				fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", url, err)
			}
			if !continueOnError {
				// Determine exit code based on error type
				errStr := err.Error()
				if strings.Contains(errStr, "failed to fetch") || strings.Contains(errStr, "HTTP error") || strings.Contains(errStr, "dial") {
					return exitError(ExitNetworkError, "")
				}
				return exitError(ExitProcessError, "")
			}
			continue
		}

		successCount++

		// Write output
		if outputDir != "" {
			// Directory mode: write each URL to its own file
			filename := urlToFilename(url, outputFormat)
			filePath := filepath.Join(outputDir, filename)
			if err := os.WriteFile(filePath, []byte(result.Content), 0644); err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Error writing file %s: %v\n", filePath, err)
				}
				hadError = true
				if !continueOnError {
					return exitError(ExitFileIOError, "")
				}
				continue
			}
			if verbose && !quiet {
				fmt.Fprintf(os.Stderr, "Saved: %s\n", filePath)
			}
		} else {
			// Single output mode
			fmt.Fprint(output, result.Content)

			// Add separator for multiple URLs (but not after the last one)
			if len(urls) > 1 && i < len(urls)-1 {
				if nullSeparator {
					fmt.Fprint(output, "\x00")
				} else {
					fmt.Fprintf(output, "\n%s\n", separator)
				}
			}
		}

		// Rate limiting delay between requests
		if delay > 0 && i < len(urls)-1 {
			time.Sleep(time.Duration(delay*1000) * time.Millisecond)
		}
	}

	// Final progress line
	if progress && !quiet && len(urls) > 1 {
		fmt.Fprintf(os.Stderr, "\r[100%%] %d/%d URLs processed\n", len(urls), len(urls))
	}

	if hadError && successCount > 0 {
		return &exitErr{code: ExitPartialError, msg: ""}
	} else if hadError && successCount == 0 {
		return &exitErr{code: ExitNetworkError, msg: ""}
	}

	return nil
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func processURL(url string, cfg *config.Config) (*ProcessResult, error) {
	if verbose && !quiet {
		fmt.Fprintf(os.Stderr, "Fetching: %s\n", url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Check if we should use an alternative extraction backend
	backend := extractBackend
	if backend == "" || backend == "readability" {
		return processURLLocal(ctx, url, cfg)
	}

	return processURLBackend(ctx, url, cfg, backend)
}

// processURLLocal uses the built-in readability extraction
func processURLLocal(ctx context.Context, url string, cfg *config.Config) (*ProcessResult, error) {
	// Create fetcher and processor
	simpleFetcher := fetcher.NewSimpleFetcher()

	// Configure redirect policy
	if noFollowRedirects {
		simpleFetcher.SetFollowRedirects(false)
	}

	contentProcessor := processor.NewContentProcessor()

	// Determine browser agent - CLI flag takes precedence over config
	effectiveBrowserAgent := cfg.Network.BrowserAgent
	if browserAgent != "" {
		effectiveBrowserAgent = browserAgent
	}
	if userAgent != "" {
		effectiveBrowserAgent = ""
	}

	// Fetch content
	fetchOpts := fetcher.FetchOptions{
		Mode:         fetcher.FetchModeStatic,
		Timeout:      time.Duration(timeout) * time.Second,
		UserAgent:    userAgent,
		BrowserAgent: effectiveBrowserAgent,
		Cookies:      nil,
	}

	fetchResult, err := simpleFetcher.FetchStatic(ctx, url, fetchOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content: %w", err)
	}

	// Process content
	processOpts := processor.ProcessOptions{
		RemoveAds:        true,
		CleanHTML:        true,
		MinContentLength: 100,
		IncludeMetadata:  includeMetadata,
		MetadataFields:   []string{"title", "author", "description", "date"},
	}

	processed, err := contentProcessor.Process(fetchResult.HTML, url, processOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to process content: %w", err)
	}

	// Format output
	var content string
	switch outputFormat {
	case "markdown":
		content = contentProcessor.ToMarkdown(processed, includeMetadata, true)
	case "text":
		content = contentProcessor.ToText(processed, 0)
	default:
		content = processed.TextContent
	}

	return &ProcessResult{
		URL:     url,
		Title:   processed.Title,
		Content: content,
	}, nil
}

// processURLBackend uses an API-based extraction backend (tavily or jina)
func processURLBackend(ctx context.Context, url string, cfg *config.Config, backendName string) (*ProcessResult, error) {
	var backend extractor.Backend

	switch backendName {
	case "tavily":
		apiKey := cfg.Extraction.Tavily.APIKey
		if envKey := os.Getenv("TAVILY_API_KEY"); envKey != "" {
			apiKey = envKey
		}
		if apiKey == "" {
			return nil, fmt.Errorf("tavily: API key not configured (set extraction.tavily.api_key in config or TAVILY_API_KEY env var)")
		}
		backend = extractor.NewTavilyBackend(
			apiKey,
			cfg.Extraction.Tavily.ExtractDepth,
			time.Duration(timeout)*time.Second,
		)

	case "jina":
		apiKey := cfg.Extraction.Jina.APIKey
		if envKey := os.Getenv("JINA_API_KEY"); envKey != "" {
			apiKey = envKey
		}
		backend = extractor.NewJinaBackend(
			apiKey,
			time.Duration(timeout)*time.Second,
		)

	default:
		return nil, fmt.Errorf("unknown extraction backend: %s (available: readability, tavily, jina)", backendName)
	}

	result, err := backend.Extract(ctx, url, outputFormat)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	return &ProcessResult{
		URL:     result.URL,
		Title:   result.Title,
		Content: result.Content,
	}, nil
}

type ProcessResult struct {
	URL     string
	Title   string
	Content string
}

func collectURLs(args []string) ([]string, error) {
	var urls []string

	// Add URLs from command line arguments
	urls = append(urls, args...)

	// Add URLs from file if specified
	if file != "" {
		fileURLs, err := readURLsFromFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read URLs from file %s: %w", file, err)
		}
		urls = append(urls, fileURLs...)
	}

	// Read URLs from stdin if no args and no file specified, or if stdin has data
	if len(args) == 0 && file == "" {
		stdinURLs, err := readURLsFromStdin()
		if err != nil {
			return nil, fmt.Errorf("failed to read URLs from stdin: %w", err)
		}
		urls = append(urls, stdinURLs...)
	}

	// Clean and validate URLs
	var cleanURLs []string
	for _, url := range urls {
		url = strings.TrimSpace(url)
		if url != "" && isValidURL(url) {
			cleanURLs = append(cleanURLs, url)
		}
	}

	return cleanURLs, nil
}

func readURLsFromFile(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var urls []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			urls = append(urls, line)
		}
	}

	return urls, scanner.Err()
}

func readURLsFromStdin() ([]string, error) {
	// Check if stdin has data
	stat, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}

	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Data is being piped in
		var urls []string
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				urls = append(urls, line)
			}
		}
		return urls, scanner.Err()
	}

	return nil, nil
}

func isValidURL(url string) bool {
	return strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "file://")
}

// urlToFilename converts a URL to a safe filename
func urlToFilename(rawURL string, format string) string {
	// Strip protocol
	name := rawURL
	name = strings.TrimPrefix(name, "https://")
	name = strings.TrimPrefix(name, "http://")

	// Replace unsafe chars
	replacer := strings.NewReplacer(
		"/", "_",
		"?", "_",
		"&", "_",
		"=", "_",
		":", "_",
		"#", "_",
		"%", "_",
	)
	name = replacer.Replace(name)

	// Trim trailing underscores
	name = strings.TrimRight(name, "_")

	// Add extension
	ext := ".txt"
	if format == "markdown" {
		ext = ".md"
	}

	// Truncate if too long
	if len(name) > 200 {
		name = name[:200]
	}

	return name + ext
}

type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string {
	return e.msg
}

func exitError(code int, format string, args ...interface{}) *exitErr {
	msg := fmt.Sprintf(format, args...)
	if msg != "" && !quiet {
		fmt.Fprintf(os.Stderr, "%s\n", msg)
	}
	return &exitErr{code: code, msg: msg}
}

// Unused import guard - sync and sync.WaitGroup will be used when parallel is fully implemented
var _ = sync.WaitGroup{}
