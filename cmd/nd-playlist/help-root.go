package main

import _ "embed"

//go:embed help/root.md
var helpRoot string

//go:embed help/ping.md
var helpPing string

//go:embed help/list.md
var helpList string

//go:embed help/show.md
var helpShow string

//go:embed help/search.md
var helpSearch string

//go:embed help/resolve.md
var helpResolve string

//go:embed help/diff.md
var helpDiff string

//go:embed help/apply.md
var helpApply string

//go:embed help/export.md
var helpExport string

//go:embed help/export-all.md
var helpExportAll string

//go:embed help/radio-diff.md
var helpRadioDiff string

//go:embed help/radio-apply.md
var helpRadioApply string

//go:embed help/radio-export.md
var helpRadioExport string
