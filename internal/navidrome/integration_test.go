package navidrome

import (
	"context"
	"os"
	"testing"
)

func TestIntegrationPing(t *testing.T) {
	server := os.Getenv("NAVIDROME_TEST_URL")
	user := os.Getenv("NAVIDROME_TEST_USER")
	password := os.Getenv("NAVIDROME_TEST_PASSWORD")
	if server == "" || user == "" || password == "" {
		t.Skip("set NAVIDROME_TEST_URL, NAVIDROME_TEST_USER, and NAVIDROME_TEST_PASSWORD")
	}

	client := NewClient(Config{
		Server:     server,
		Username:   user,
		Password:   password,
		Client:     defaultClient,
		APIVersion: defaultAPIVersion,
	})
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
