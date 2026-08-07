package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/mcp.md
var helpMCP string
