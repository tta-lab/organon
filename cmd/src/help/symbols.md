Show the typed symbol outline for a structured source file or Markdown
document. With --json the outline is emitted as a single JSON document on
stdout with the path, language, optional title, total bytes, and the symbol
list. Each symbol carries its opaque ID, display name, kind, parent, byte and
line ranges, targetability, and attached-doc status.

The outline always uses the established depth of 2, matching the Pi src
tool. Symbol IDs are opaque returned IDs; they are never derived from symbol
names.

    src symbols main.go --json
