package search

import "fmt"

// Config controls runtime web search provider selection. An empty Provider uses
// the EXA_API_KEY → BRAVE_API_KEY → DuckDuckGo fallback.
type Config struct {
	Provider string
}

// ValidateProvider checks an explicit provider selection. The empty value
// selects the documented environment-based fallback.
func ValidateProvider(provider string) error {
	switch provider {
	case "", "exa", "brave", "duckduckgo":
		return nil
	default:
		return fmt.Errorf("unsupported search provider %q", provider)
	}
}
