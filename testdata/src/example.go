package example

import "fmt"

// Config holds server configuration.
type Config struct {
	Host string
	Port int
}

// Validate checks the config.
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host required")
	}
	return nil
}

func main() {
	c := &Config{Host: "localhost", Port: 8080}
	_ = c.Validate()
}
