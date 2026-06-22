package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/pr.md
var helpPR string
