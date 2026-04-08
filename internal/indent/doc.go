// Package indent detects and normalizes source file indentation style.
//
// Detection is layered:
//
//	Layer 1 — opinionated languages (Go, Python, Rust, Ruby, Elixir, Lua,
//	          Makefile, YAML): hardcoded style from the canonical formatter.
//	Layer 2 — open languages (JS/TS, C/C++, Java, etc): per-file detection,
//	          scan first 200 non-empty non-JSDoc-continuation lines, 80% majority.
//	Layer 3 — fallback: no detection possible, caller writes literally + warns.
//
// Plane: shared
package indent
