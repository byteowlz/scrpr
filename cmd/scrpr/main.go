package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/byteowlz/scrpr/internal/config"
	"github.com/byteowlz/scrpr/internal/fetcher"
	"github.com/byteowlz/scrpr/internal/processor"
)

var (
	cfgFile         string
	outputFile      string
	outputFormat    string
	browser         string
	browserAgent    string
	javascript      bool
	noJS            bool
	skipBanners     bool
	timeout         int
	concurrency     int
	batchSize       int
	progress        bool
	separator       string
	nullSeparator   bool
	userAgent       string
	includeMetadata bool
	verbose         bool
	file            string
)

var rootCmd = &cobra.Command{
	Use:   "scrpr [urls...]",
	Short: "Extract main content from websites",
	Long: `scrpr is a CLI tool that extracts the main content from websites.
It supports JavaScript rendering, cookie extraction from browsers, and pipe operations.`,
	RunE: run,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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

	// System flags
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error finding home directory: %v\n", err)
				return
			}
			configHome = filepath.Join(home, ".config")
		}

		configDir := filepath.Join(configHome, "scrpr")
		viper.AddConfigPath(configDir)
		viper.SetConfigType("toml")
		viper.SetConfigName("config")

		// Create config directory if it doesn't exist
		if err := os.MkdirAll(configDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating config directory: %v\n", err)
		}
	}

	viper.AutomaticEnv()
	viper.SetEnvPrefix("SCRPR")

	if err := viper.ReadInConfig(); err == nil && verbose {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", viper.ConfigFileUsed())
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Collect URLs from various sources
	urls, err := collectURLs(args)
	if err != nil {
		return fmt.Errorf("failed to collect URLs: %w", err)
	}

	if len(urls) == 0 {
		return fmt.Errorf("no URLs provided")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Processing %d URLs\n", len(urls))
	}

	// Process URLs (for now sequentially, parallel processing comes later)
	for i, url := range urls {
		if verbose {
			fmt.Fprintf(os.Stderr, "Processing [%d/%d]: %s\n", i+1, len(urls), url)
		}

		result, err := processURL(url, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", url, err)
			continue
		}

		// Output result
		if outputFile != "" {
			// TODO: Handle file output
			fmt.Printf("Would save to file: %s\n", outputFile)
		}

		fmt.Print(result.Content)

		// Add separator for multiple URLs
		if len(urls) > 1 && i < len(urls)-1 {
			if nullSeparator {
				fmt.Print("\x00")
			} else {
				fmt.Printf("\n%s\n", separator)
			}
		}
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
	if verbose {
		fmt.Fprintf(os.Stderr, "Fetching: %s\n", url)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Create fetcher and processor
	simpleFetcher := fetcher.NewSimpleFetcher()
	contentProcessor := processor.NewContentProcessor()

	// Determine browser agent - CLI flag takes precedence over config
	effectiveBrowserAgent := cfg.Network.BrowserAgent
	if browserAgent != "" {
		// CLI flag overrides config
		effectiveBrowserAgent = browserAgent
	}
	if userAgent != "" {
		// If custom user agent provided via CLI, it will be used directly
		effectiveBrowserAgent = ""
	}

	// Fetch content
	fetchOpts := fetcher.FetchOptions{
		Mode:         fetcher.FetchModeStatic,
		Timeout:      time.Duration(timeout) * time.Second,
		UserAgent:    userAgent,
		BrowserAgent: effectiveBrowserAgent,
		Cookies:      nil, // TODO: Add cookie support
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
		content = contentProcessor.ToText(processed, 0) // 0 = no line wrapping
	default:
		content = processed.TextContent
	}

	return &ProcessResult{
		URL:     url,
		Title:   processed.Title,
		Content: content,
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
