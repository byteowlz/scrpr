package browser

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/browserutils/kooky"
	_ "github.com/browserutils/kooky/browser/all" // Import all browser support
)

type BrowserType string

const (
	BrowserAuto    BrowserType = "auto"
	BrowserChrome  BrowserType = "chrome"
	BrowserFirefox BrowserType = "firefox"
	BrowserSafari  BrowserType = "safari"
	BrowserZen     BrowserType = "zen"
)

type CookieExtractor struct {
	browserType BrowserType
	customPaths map[string]string
}

func NewCookieExtractor(browserType BrowserType, customPaths map[string]string) *CookieExtractor {
	return &CookieExtractor{
		browserType: browserType,
		customPaths: customPaths,
	}
}

func (ce *CookieExtractor) ExtractCookies(targetURL string) ([]*http.Cookie, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	var cookies []*http.Cookie

	if ce.browserType == BrowserAuto {
		// Try all browsers in order of preference
		browsers := []BrowserType{BrowserChrome, BrowserFirefox, BrowserZen, BrowserSafari}
		for _, browser := range browsers {
			if browserCookies, err := ce.extractFromBrowser(browser, parsedURL.Host); err == nil && len(browserCookies) > 0 {
				cookies = append(cookies, browserCookies...)
				break
			}
		}
	} else {
		cookies, err = ce.extractFromBrowser(ce.browserType, parsedURL.Host)
		if err != nil {
			return nil, err
		}
	}

	return cookies, nil
}

func (ce *CookieExtractor) extractFromBrowser(browserType BrowserType, domain string) ([]*http.Cookie, error) {
	ctx := context.Background()
	var cookies []*http.Cookie

	// Use TraverseCookies to get all cookies
	cookieSeq := kooky.TraverseCookies(ctx)

	for cookie, err := range cookieSeq {
		if err != nil {
			continue
		}

		// Filter by browser type and domain
		if ce.matchesBrowserType(cookie.Browser, browserType) && ce.matchesDomain(cookie.Domain, domain) {
			cookies = append(cookies, &http.Cookie{
				Name:     cookie.Name,
				Value:    cookie.Value,
				Path:     cookie.Path,
				Domain:   cookie.Domain,
				Expires:  cookie.Expires,
				Secure:   cookie.Secure,
				HttpOnly: cookie.HttpOnly,
			})
		}
	}

	return cookies, nil
}

func (ce *CookieExtractor) matchesBrowserType(browser kooky.BrowserInfo, browserType BrowserType) bool {
	if browserType == BrowserAuto {
		return true
	}

	browserName := strings.ToLower(browser.Browser())
	switch browserType {
	case BrowserChrome:
		return strings.Contains(browserName, "chrome") || strings.Contains(browserName, "chromium")
	case BrowserFirefox:
		return strings.Contains(browserName, "firefox")
	case BrowserSafari:
		return strings.Contains(browserName, "safari")
	case BrowserZen:
		return strings.Contains(browserName, "zen") ||
			(strings.Contains(browserName, "firefox") && strings.Contains(browser.FilePath(), "zen"))
	}

	return false
}

func (ce *CookieExtractor) matchesDomain(cookieDomain, targetDomain string) bool {
	if cookieDomain == "" || targetDomain == "" {
		return false
	}

	// Remove leading dot from cookie domain
	if strings.HasPrefix(cookieDomain, ".") {
		cookieDomain = cookieDomain[1:]
	}

	// Exact match
	if cookieDomain == targetDomain {
		return true
	}

	// Subdomain match
	if strings.HasSuffix(targetDomain, "."+cookieDomain) {
		return true
	}

	return false
}

func (ce *CookieExtractor) getZenCookieStores(domain string) ([]kooky.CookieStore, error) {
	// Zen browser uses Firefox-like profile structure
	zenPath := ce.getZenProfilePath()
	if zenPath == "" {
		return nil, fmt.Errorf("Zen browser profile not found")
	}

	// Use Firefox cookie reader with Zen path
	cookieFile := filepath.Join(zenPath, "cookies.sqlite")
	if _, err := os.Stat(cookieFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("Zen cookies.sqlite not found at %s", cookieFile)
	}

	// Get all stores and filter for Zen
	ctx := context.Background()
	stores := kooky.FindAllCookieStores(ctx)
	var zenStores []kooky.CookieStore

	for _, store := range stores {
		if strings.Contains(store.FilePath(), "zen") || strings.Contains(store.FilePath(), ".zen") {
			zenStores = append(zenStores, store)
		}
	}

	return zenStores, nil
}

func (ce *CookieExtractor) getZenProfilePath() string {
	var basePath string

	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		basePath = filepath.Join(home, ".zen")
	case "linux":
		home, _ := os.UserHomeDir()
		basePath = filepath.Join(home, ".zen")
	case "windows":
		appData := os.Getenv("APPDATA")
		basePath = filepath.Join(appData, "Zen")
	default:
		return ""
	}

	// Check if custom path is provided
	if customPath, exists := ce.customPaths["zen"]; exists && customPath != "" {
		basePath = customPath
	}

	// Find the default profile (usually ends with .default)
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() && strings.Contains(entry.Name(), ".default") {
			return filepath.Join(basePath, entry.Name())
		}
	}

	// If no default profile found, return the first profile directory
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			profilePath := filepath.Join(basePath, entry.Name())
			cookiePath := filepath.Join(profilePath, "cookies.sqlite")
			if _, err := os.Stat(cookiePath); err == nil {
				return profilePath
			}
		}
	}

	return ""
}

func filterChromeStores(stores []kooky.CookieStore) []kooky.CookieStore {
	var chromeStores []kooky.CookieStore
	for _, store := range stores {
		path := strings.ToLower(store.FilePath())
		if strings.Contains(path, "chrome") || strings.Contains(path, "chromium") {
			chromeStores = append(chromeStores, store)
		}
	}
	return chromeStores
}

func filterFirefoxStores(stores []kooky.CookieStore) []kooky.CookieStore {
	var firefoxStores []kooky.CookieStore
	for _, store := range stores {
		path := strings.ToLower(store.FilePath())
		if strings.Contains(path, "firefox") || strings.Contains(path, "mozilla") {
			firefoxStores = append(firefoxStores, store)
		}
	}
	return firefoxStores
}

func filterSafariStores(stores []kooky.CookieStore) []kooky.CookieStore {
	var safariStores []kooky.CookieStore
	for _, store := range stores {
		path := strings.ToLower(store.FilePath())
		if strings.Contains(path, "safari") || strings.Contains(path, "cookies.binarycookies") {
			safariStores = append(safariStores, store)
		}
	}
	return safariStores
}

func (ce *CookieExtractor) DetectAvailableBrowsers() []BrowserType {
	var available []BrowserType

	browsers := []BrowserType{BrowserChrome, BrowserFirefox, BrowserSafari, BrowserZen}

	for _, browser := range browsers {
		if ce.isBrowserAvailable(browser) {
			available = append(available, browser)
		}
	}

	return available
}

func (ce *CookieExtractor) isBrowserAvailable(browserType BrowserType) bool {
	switch browserType {
	case BrowserChrome:
		return ce.checkBrowserPath("chrome", []string{
			"~/.config/google-chrome",
			"~/Library/Application Support/Google/Chrome",
			"%LOCALAPPDATA%/Google/Chrome/User Data",
		})
	case BrowserFirefox:
		return ce.checkBrowserPath("firefox", []string{
			"~/.mozilla/firefox",
			"~/Library/Application Support/Firefox",
			"%APPDATA%/Mozilla/Firefox",
		})
	case BrowserSafari:
		if runtime.GOOS != "darwin" {
			return false
		}
		return ce.checkBrowserPath("safari", []string{
			"~/Library/Cookies",
		})
	case BrowserZen:
		return ce.checkBrowserPath("zen", []string{
			"~/.zen",
			"~/Library/Application Support/Zen",
			"%APPDATA%/Zen",
		})
	}
	return false
}

func (ce *CookieExtractor) checkBrowserPath(browserName string, defaultPaths []string) bool {
	// Check custom path first
	if customPath, exists := ce.customPaths[browserName]; exists && customPath != "" {
		if _, err := os.Stat(expandPath(customPath)); err == nil {
			return true
		}
	}

	// Check default paths
	for _, path := range defaultPaths {
		if _, err := os.Stat(expandPath(path)); err == nil {
			return true
		}
	}

	return false
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}

	if strings.Contains(path, "%LOCALAPPDATA%") {
		localAppData := os.Getenv("LOCALAPPDATA")
		return strings.Replace(path, "%LOCALAPPDATA%", localAppData, 1)
	}

	if strings.Contains(path, "%APPDATA%") {
		appData := os.Getenv("APPDATA")
		return strings.Replace(path, "%APPDATA%", appData, 1)
	}

	return path
}
