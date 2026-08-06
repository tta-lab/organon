package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/pr.md
var helpPR string

//go:embed help/daemon.md
var helpDaemon string

//go:embed help/mcp.md
var helpMCP string
