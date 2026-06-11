goal reads and mutates the current Lenos session goal file.

The tool reads the target path from $LENOS_GOAL. It only works inside
a Lenos goal session; if the environment variable is unset, every
command fails with a clear error.

## Commands

  goal add    [--status draft|active|blocked|complete]
  goal update
  goal append
  goal get    [--json]
  goal status <draft|active|blocked|complete>

## Status values

  draft    - goal is being written; runtime ignores it
  active   - active goal; runtime enforces exit gate
  blocked  - terminal: goal cannot be completed now
  complete - terminal: goal is finished

## Examples

cat <<'EOF' | goal add --status active
# Goal

Refactor auth.
EOF

cat <<'EOF' | goal update
# Goal

Refactor auth without changing public API.
EOF

cat <<'EOF' | goal append
Verified with make test.
EOF

goal status complete
