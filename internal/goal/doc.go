// Package goal reads and mutates Lenos session goal files.
//
// A goal file is a Markdown document with YAML frontmatter containing
// status, created_at, and updated_at fields. The package provides
// parsing, validation, and atomic write operations.
//
// Plane: shared
package goal
