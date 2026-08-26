package fetcher

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sync"
)

// uaMemory remembers which user agent succeeded per domain. It is persisted
// as JSON in the XDG state directory so subsequent runs skip straight to the
// known-working UA instead of retrying through the whole fallback list.
type uaMemory struct {
	mu      sync.Mutex
	path    string
	entries map[string]string
}

func newUAMemory(path string) *uaMemory {
	m := &uaMemory{
		path:    path,
		entries: make(map[string]string),
	}
	data, err := os.ReadFile(path)
	if err == nil {
		var stored map[string]string
		if json.Unmarshal(data, &stored) == nil {
			for k, v := range stored {
				if v != "" {
					m.entries[k] = v
				}
			}
		}
	}
	return m
}

func (m *uaMemory) Get(host string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[host]
}

func (m *uaMemory) Set(host, ua string) {
	if host == "" || ua == "" {
		return
	}
	m.mu.Lock()
	m.entries[host] = ua
	m.mu.Unlock()
	m.save()
}

func (m *uaMemory) save() {
	data, err := json.MarshalIndent(m.entries, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return
	}
	_ = os.WriteFile(m.path, data, 0600)
}

// hostFromURL extracts the hostname from a URL, empty string on parse failure.
func hostFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
