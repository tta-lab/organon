package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/list.md
var helpList string

//go:embed help/get.md
var helpGet string

//go:embed help/resolve.md
var helpResolve string

//go:embed help/jump.md
var helpJump string

//go:embed help/org.md
var helpOrg string
