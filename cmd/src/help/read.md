Read a file, an exact symbol or Markdown section, or a line range of either.
With --json the result is a single JSON document on stdout carrying the
selected content plus line metadata and Pi-equivalent truncation fields.

--offset is a 1-indexed line offset and --limit a maximum line count; both are
relative to the selected content (the whole file, or the exact symbol/section
when --symbol-id is present). Text output follows the Pi 2,000-line / 50-KB
truncation contract with actionable continuation fields. Omitting --symbol-id
never requires structural support.

    src read main.go --json
    src read main.go --symbol-id bK --json
    src read main.go --offset 100 --limit 50 --json
