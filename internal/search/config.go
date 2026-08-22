package search

import "fmt"

// Config controls runtime web search provider selection. An empty Provider uses
// EXA_API_KEY → BRAVE_API_KEY selection and otherwise returns an error.
type Config struct {
	Provider string
}

// ValidateProvider checks an explicit provider selection. The empty value
// selects the documented environment-based fallback.
func ValidateProvider(provider string) error {
	switch provider {
	case "", "exa", "brave":
		return nil
	default:
		return fmt.Errorf("unsupported search provider %q", provider)
	}
}
