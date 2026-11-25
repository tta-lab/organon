# Let Git provide GitHub credentials

Organon uses the execution user's configured Git credential helper for GitHub.
GitHub Git, pull request, and issue operations retain their existing scope, while
Organon retires its GitHub App identity and PAT file configuration. This keeps
credential storage and rotation with the tools the user already uses for Git,
including on native Windows.
