package main

const helpRoot = `goal reads and mutates the current Lenos session goal file.

The tool reads the target path from $LENOS_GOAL. It only works inside
a Lenos goal session; if the environment variable is unset, every
command fails with a clear error.

## Commands

  goal add    [--status active|blocked|complete] <body>
  goal update <body>
  goal append <body>
  goal get    [--json]
  goal status <draft|active|blocked|complete>

## Status values

  draft    – goal is being written; runtime ignores it
  active   – active goal; runtime enforces exit gate
  blocked  – terminal: goal cannot be completed now
  complete – terminal: goal is finished

## Examples

  goal add "Add error handling to the login flow"
  goal add --status active "Refactor auth"
  goal status complete
  goal append "Verified with make test"
`
