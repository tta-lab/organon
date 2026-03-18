package fetch

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type defuddleCLIBackend struct{}

// NewDefuddleCLIBackend creates a backend that shells out to the defuddle CLI.
// Requires defuddle to be installed and on PATH.
func NewDefuddleCLIBackend() Backend {
	return &defuddleCLIBackend{}
}

func (b *defuddleCLIBackend) Fetch(ctx context.Context, url string) (string, error) {
	cmd := exec.CommandContext(ctx, "defuddle", "parse", url, "--markdown")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("defuddle parse failed: %w\noutput: %s", err, strings.TrimSpace(string(out)))
	}
	return TruncateContent(string(out)), nil
}
