package fetcher

import (
	"math/rand"
	"strings"
	"time"
)

type UserAgentType string

const (
	UserAgentAuto    UserAgentType = "auto"
	UserAgentChrome  UserAgentType = "chrome"
	UserAgentFirefox UserAgentType = "firefox"
	UserAgentSafari  UserAgentType = "safari"
	UserAgentEdge    UserAgentType = "edge"
)

var userAgents = map[UserAgentType][]string{
	UserAgentChrome: {
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
	},
	UserAgentFirefox: {
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.1; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:120.0) Gecko/20100101 Firefox/120.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.0; rv:120.0) Gecko/20100101 Firefox/120.0",
	},
	UserAgentSafari: {
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_1_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_1_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Safari/605.1.15",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_1_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (iPad; CPU OS 17_1_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
	},
	UserAgentEdge: {
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36 Edg/119.0.0.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
	},
}

type UserAgentSelector struct {
	rng *rand.Rand
}

func NewUserAgentSelector() *UserAgentSelector {
	return &UserAgentSelector{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// GetUserAgent returns a user agent string based on the specified type
// If uaType is "auto" or empty, it randomly selects from all available user agents
// If a specific browser type is specified, it randomly selects from that browser's user agents
func (uas *UserAgentSelector) GetUserAgent(uaType string) string {
	// Normalize the input
	uaType = strings.ToLower(strings.TrimSpace(uaType))

	// If empty, use auto
	if uaType == "" {
		uaType = "auto"
	}

	switch UserAgentType(uaType) {
	case UserAgentAuto:
		return uas.getRandomFromAll()
	case UserAgentChrome, UserAgentFirefox, UserAgentSafari, UserAgentEdge:
		return uas.getRandomFromType(UserAgentType(uaType))
	default:
		// If it's a custom string, return it as-is
		return uaType
	}
}

// getRandomFromAll selects a random user agent from all available types
func (uas *UserAgentSelector) getRandomFromAll() string {
	allUAs := []string{}
	for _, uas := range userAgents {
		allUAs = append(allUAs, uas...)
	}

	if len(allUAs) == 0 {
		// Fallback to a default Chrome user agent
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}

	return allUAs[uas.rng.Intn(len(allUAs))]
}

// getRandomFromType selects a random user agent from a specific browser type
func (uas *UserAgentSelector) getRandomFromType(uaType UserAgentType) string {
	agents, ok := userAgents[uaType]
	if !ok || len(agents) == 0 {
		// Fallback to auto if type not found
		return uas.getRandomFromAll()
	}

	return agents[uas.rng.Intn(len(agents))]
}
