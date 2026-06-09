package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/search.md
var helpSearch string

//go:embed help/fetch.md
var helpFetch string

//go:embed help/docs.md
var helpDocs string

//go:embed help/sgraph.md
var helpSgraph string
