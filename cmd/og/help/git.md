# og git

Guarded git operations.

This command group replaces the forge-related top-level `ttal push`, `ttal pull`,
and `ttal tag` commands.

V1 behavior:

- `og git push [--force]` pushes the current branch to origin. `--force` means
  force-with-lease, matching `ttal push --force`.
- `og git pull` fast-forward pulls the current branch from origin.
- `og git tag <version>` creates and pushes a semver tag.
- `og git tag --bump <major|minor|patch>` computes the next semver tag before
  creating and pushing it.

Commands resolve the current repository from git metadata and do not accept
arbitrary git args.
