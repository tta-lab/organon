Resolve an existing absolute directory to its nearest verified linked worktree
of an active registered project. The root and directories within it are accepted,
including directories inside a nested independent repository. Filesystem aliases
are resolved. On success stdout contains exactly one JSON object:

  {"project":"canonical-alias","worktree":"/canonical/linked/root"}

Relative paths, files, registered checkouts, unrelated clones, stale or
unverifiable roots, and archived or ambiguous ownership fail with a nonzero exit.
This local read-only query needs no forge access or credentials.

  og project resolve-worktree /absolute/directory
