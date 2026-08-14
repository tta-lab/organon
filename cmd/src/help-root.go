package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/replace.md
var helpReplace string

//go:embed help/insert.md
var helpInsert string

//go:embed help/delete.md
var helpDelete string

//go:embed help/comment.md
var helpComment string

//go:embed help/edit.md
var helpEdit string

//go:embed help/symbols.md
var helpSymbols string

//go:embed help/read.md
var helpRead string

//go:embed help/mcp.md
var helpMCP string
