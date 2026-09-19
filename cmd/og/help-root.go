package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/pr.md
var helpPR string

//go:embed help/mcp.md
var helpMCP string

//go:embed help/project.md
var helpProject string

//go:embed help/project-list.md
var helpProjectList string

//go:embed help/project-find.md
var helpProjectFind string

//go:embed help/project-get.md
var helpProjectGet string

//go:embed help/project-resolve.md
var helpProjectResolve string

//go:embed help/project-jump.md
var helpProjectJump string
