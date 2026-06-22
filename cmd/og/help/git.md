# og git

Guarded git operations through the `og` daemon.

This command group replaces the forge-related top-level `ttal push`, `ttal pull`,
and `ttal tag` commands.

Planned V1 parity:

- `og git push [--force]` pushes the current branch to origin. `--force` means
  force-with-lease, matching `ttal push --force`.
- `og git pull` pulls the current branch. The implementation should keep the
  `ttal pull` behavior: fast-forward pull on the default branch, branch pull for
  open/unmatched PR branches, and merged-PR branch cleanup when safe.
- `og git tag <version>` creates and pushes a semver tag.
- `og git tag --bump <major|minor|patch>` computes the next semver tag before
  creating and pushing it.

All operations should go through typed daemon requests, not arbitrary git args.
