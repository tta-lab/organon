package og

import (
	"context"
	"fmt"
	"strings"

	"github.com/tta-lab/organon/internal/githubapp"
)

type patBroker struct{ owner, file, configDir string }

func (b *patBroker) Token(_ context.Context, owner, _ string, _ githubapp.Purpose) (string, error) {
	if !strings.EqualFold(owner, b.owner) {
		return "", fmt.Errorf("%w: GitHub PAT owner %q does not match configured owner", githubapp.ErrOwnerNotAllowed, owner)
	}
	return githubapp.ReadPrivateToken(b.configDir, b.file)
}
func (b *patBroker) Status(ctx context.Context, owner, repo string) (githubapp.InstallationStatus, error) {
	_, err := b.Token(ctx, owner, repo, githubapp.PurposeAPI)
	return githubapp.InstallationStatus{Repository: owner + "/" + repo}, err
}
func (b *patBroker) Invalidate(_, _ string, _ githubapp.Purpose, _ string) error { return nil }
