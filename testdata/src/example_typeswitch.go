package example

// Config holds server configuration.
type Config struct {
	Host string
	Port int
}

// Describe returns a description of the value.
func Describe(value any) string {
	switch typed := value.(type) {
	case nil:
		return "nil"
	case string:
		return "string: " + typed
	case int:
		return "int"
	case *Config:
		return "config: " + typed.Host
	default:
		return "unknown"
	}
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{Host: "localhost", Port: 8080}
}
