package githubapp

import "errors"

var (
	// ErrOwnerNotAllowed marks a repository owner outside the configured allowlist.
	ErrOwnerNotAllowed = errors.New("GitHub owner is not allowed")
	// ErrInstallationNotFound marks a repository without an accessible App installation.
	ErrInstallationNotFound = errors.New("GitHub App installation was not found")
)
