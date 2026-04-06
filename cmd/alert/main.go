package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "Send an alert to the message bridge",
		Long: `Send an alert message to the configured bridge endpoint.

Usage:
  alert --from flick "the db is gone"
  cat <<'EOF' | alert --from flick
detailed message
EOF

Requires ALERT_ENDPOINT environment variable to be set.`,
		Args: cobra.RangeArgs(0, 1),
		RunE: runAlert,
	}
	cmd.Flags().String("from", "", "Sender identifier (required)")
	return cmd
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func runAlert(cmd *cobra.Command, args []string) error {
	endpoint := os.Getenv("ALERT_ENDPOINT")
	if endpoint == "" {
		return fmt.Errorf("ALERT_ENDPOINT environment variable is not set")
	}

	sender, err := cmd.Flags().GetString("from")
	if err != nil || sender == "" {
		return fmt.Errorf("--from flag is required")
	}

	var message string
	if len(args) == 1 {
		message = args[0]
	} else if !isStdinPiped() {
		return fmt.Errorf("no message provided; use positional arg or pipe message via stdin")
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		message = strings.TrimRight(string(b), "\r\n")
		if message == "" {
			return fmt.Errorf("no message provided; use positional arg or pipe message via stdin")
		}
	}

	payload, err := json.Marshal(map[string]string{
		"message": message,
		"from":    sender,
	})
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// isStdinPiped returns true if stdin is connected to a pipe or redirect.
func isStdinPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}
