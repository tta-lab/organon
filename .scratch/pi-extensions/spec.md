Status: ready-for-agent

## Problem Statement

Pi currently reaches Organon through four separately configured MCP servers. That adds installation and transport setup, exposes duplicate tools alongside Pi's built-ins, and prevents Organon from shaping Pi-native tool selection and prompts. The existing CLIs also lack consistent machine-readable output, so a thin Pi adapter cannot reliably call them. In particular, the current project-scoped, read-only `src` MCP does not fit Pi: Pi already supplies the session working directory, and the desired `src` experience is one local-file tool that replaces Pi's built-in `read` and `edit` with both structure-aware operations and exact multi-edit replacement.

## Solution

Publish four independently installable Pi extension packages—`@tta-lab/pi-src`, `@tta-lab/pi-web`, `@tta-lab/pi-project`, and `@tta-lab/pi-og`—from a pnpm workspace in this repository. Each package registers one Pi tool with an action-discriminated schema and is a thin adapter over its corresponding bundled Go CLI binary. Add normal `--json` output to the CLIs wherever the extension needs structured output; do not add a Pi-specific CLI command or transport.

At install time, each extension package selects one matching native npm package for the host platform from exact-version optional dependencies. Support Darwin ARM64, Linux x64, and Linux ARM64. Version the JavaScript package, native package, Go binary, npm release, and GitHub release together.

`pi-src` is the deliberate exception to MCP transport parity: it uses absolute paths or paths relative to Pi's current `ctx.cwd`, has no project-registry dependency, removes the obsolete project-scoped `src` MCP, and deactivates Pi's built-in `read` and `edit` while active. It exposes ordinary file reads, symbol-aware reads and mutations, and an exact multi-edit action matching Pi's built-in edit contract.

## User Stories

1. As a Pi user, I want to install only the Organon capabilities I need, so that `src`, `web`, `project`, and `og` are independently usable without MCP configuration.
2. As a Pi user, I want each extension to carry a compatible native binary, so that installing the npm package is sufficient on a supported platform.
3. As an agent, I want `src` paths to resolve relative to the Pi session working directory or accept an absolute path, so that source operations do not require project registration or aliases.
4. As an agent, I want `src` to replace Pi's built-in `read` and `edit`, so that one tool is the authoritative single-file inspection and editing interface rather than a fallback layered beside weaker tools.
5. As an agent, I want ordinary line-oriented reads to work for text and supported image files, with Pi-equivalent truncation and continuation behavior, so that disabling built-in `read` does not remove known capabilities.
6. As an agent, I want to inspect a structured source or Markdown file's symbols and then read, replace, insert, delete, or comment using the exact returned symbol ID, so that large-file work is context-efficient and does not rely on reproducing source text.
7. As an agent that already knows exact source text, I want to edit one file with multiple disjoint replacements in one call without first fetching symbols, so that precise changes take one atomic operation.
8. As an agent, I want prompts to state that symbol IDs are opaque returned IDs rather than symbol names, so that I do not guess IDs from function, class, method, variable, or section names.
9. As a CLI user, I want machine-readable JSON from the commands used by the extensions, so that the same structured behavior is useful outside Pi.
10. As a Pi user, I want `web`, `project`, and `og` actions to preserve their current MCP-visible inputs, defaults, results, validation, and policy behavior, so that moving from MCP to the extension changes transport rather than domain semantics.
11. As an `og` user, I want all Git and forge mutations to continue through the guarded daemon and registered-project policy, so that the extension cannot bypass branch, remote, archive, or credential protections.
12. As a maintainer, I want a tagged release to produce GitHub archives and matching npm extension/native packages, so that clients cannot combine incompatible schemas and binaries.

## Delivery Boundary

This spec is implemented and reviewed as one PR. It may be decomposed into multiple tickets on the same branch when that makes execution easier.

## Implementation Decisions

### Package and release shape

Use pnpm for JavaScript development, testing, building, and publishing. Keep shared TypeScript subprocess, truncation, result, and binary-resolution implementation private to the workspace and bundle it into each public extension; do not publish a fifth shared runtime package.

Publish four main Pi packages. For each main package, publish three platform-specific native packages named `@tta-lab/pi-<tool>-darwin-arm64`, `@tta-lab/pi-<tool>-linux-x64`, and `@tta-lab/pi-<tool>-linux-arm64`. There are therefore four main packages and twelve native packages. A main package declares exact-version optional dependencies on its three native variants; npm selects the package whose `os` and `cpu` constraints match. Each native package contains only that extension's one Go binary. Unsupported platforms fail at extension startup with an actionable platform error rather than downloading GitHub `latest`, compiling Go, or falling back to an unrelated binary on `PATH`.

A release tag is the single version source. GoReleaser continues to cross-compile with CGO disabled; Linux ARM64 does not require an ARM build runner. The release workflow stages the relevant binaries into generated npm package directories, publishes native packages before their main package, and publishes all package and GitHub artifacts at the same version. Never resolve a runtime binary from the GitHub `latest` release.

### Extension interface and process adapter

Each package registers exactly one global Pi tool named after the capability: `src`, `web`, `project`, or `og`. Each schema is a closed TypeBox union of action-specific objects: unknown fields are rejected, required fields stay action-specific, and enum fields use Pi's Google-compatible string-enum helper. The extension passes fixed argv arrays to its package-local binary, sends multiline content through stdin, propagates the tool abort signal to the child process, treats nonzero exit as a concise tool error using stderr, and parses exactly one JSON document from stdout. It does not invoke a shell and does not start a persistent process.

CLI JSON mode writes only the documented JSON result to stdout; progress and diagnostics go to stderr. Human output remains the default. For exact batch editing, the public `src edit --edits-json --json <file>` form reads `{ "edits": [{ "oldText": "...", "newText": "..." }] }` from stdin; ordinary human `src edit` retains its existing BEFORE/AFTER stdin format. Other multiline mutation bodies remain raw stdin. JSON behavior and MCP behavior call the same existing internal service seam rather than one adapter calling or parsing the other. Add CLI flags and normal subcommands where needed, including project selection for guarded `og` commands, rather than adding `pi-call` or another private protocol.

Large text results use Pi's standard 2,000-line/50-KB limits and actionable continuation metadata. Structured raw data needed for rendering remains in tool-result details. Mutating `src` actions resolve the real absolute target from the input path and `ctx.cwd`, then hold Pi's per-file mutation queue for the full child-process read-modify-write window.

### `src` domain and interface

`src` has no dependency on the project registry or `pi-project`. A relative path is resolved against the current tool context's `cwd`; an absolute path is normalized and used directly. Normalize Pi's conventional accidental leading `@` before path resolution. Do not silently fall back to built-in tools.

Remove the `src mcp` command, its project-scoped adapter and tests, and documentation advertising that server. Remove project-scoped source-view code that has no remaining caller after the CLI and extension share the trusted-file inspector. No replacement path-based MCP server is introduced.

Add normal public `src symbols <file> --json` and `src read <file> --json` subcommands for the extension's two read paths; `src read` accepts optional symbol ID and line offset/limit flags. Preserve the existing human-oriented root invocation and mutation commands. Every mutation command used by Pi accepts `--json` output, while batch exact editing additionally uses the public `--edits-json` stdin mode described above.

The public `src` input union is fixed as follows; `content`, `oldText`, and `newText` may contain multiline text:

- `{ action: "symbols", path }` returns the typed outline for a structured source file or Markdown document, including each opaque ID, display name, kind, parent, byte and line ranges, targetability, and attached-doc status. Pi uses the CLI's established depth of 2; this first interface does not expose depth because every later symbol operation must resolve IDs from the same outline shape.
- `{ action: "read", path, symbol_id?, offset?, limit? }` reads either the whole file or the exact symbol/section. `offset` is a one-indexed line offset and `limit` is a maximum line count, both relative to the selected content when `symbol_id` is present. Omitting `symbol_id` never requires structural support. Text output follows Pi's 2,000-line/50-KB truncation and continuation contract.
- `{ action: "replace", path, symbol_id, content }` replaces one code symbol or Markdown section.
- `{ action: "insert", path, symbol_id, position, content }`, where `position` is `before` or `after`, inserts relative to one code symbol or Markdown section. The adapter maps this to the established CLI before/after flags.
- `{ action: "delete", path, symbol_id }` deletes one code symbol or Markdown section.
- `{ action: "comment", path, symbol_id, read: true }` reads an existing code doc comment, while `{ action: "comment", path, symbol_id, content }` writes one. Comment operations remain unsupported for Markdown, as in the current CLI.
- `{ action: "edit", path, edits: [{ oldText, newText }, ...] }` performs exact text replacement on one existing file.

For image input, `read` recognizes the same signatures as Pi's built-in read—non-animated PNG, JPEG, GIF, WebP, and BMP—and returns a Pi image content block plus a concise text note. The CLI JSON representation carries explicit media kind, MIME type, and base64 data for the adapter to convert; it never decodes an image as UTF-8 text. Preserve Pi-equivalent orientation/size safety before attaching media to the model. Animated PNG and unsupported binary files fail visibly rather than producing corrupted text.

The `edit` action matches Pi's current built-in contract. It requires at least one replacement and must not retain the current srcop 100-KB file cap, because the built-in edit being replaced has no such lower ordinary-text limit. Every `oldText` must identify one unique region in the original file. All replacements are located against the original content, not content produced by preceding entries. Overlapping or nested edits are rejected, nearby changes should be represented as one edit, and any validation failure leaves the file untouched. A successful call applies all replacements in one write, preserves BOM and line endings, and returns one aggregate diff/patch and first changed line. Implement a true batch operation in the shared Go editing core; do not loop the current single-edit operation, because that would make matching incremental and could permit partial success. The existing single-edit human syntax remains supported but is not a second Pi schema.

While `pi-src` is active, update Pi's active tool set incrementally by source provenance: remove only active tools whose provenance is Pi builtin and whose names are `read` or `edit`, retain all unrelated active tools, and activate `src`. At `session_start`, remember which of those two built-ins this extension instance actually displaced. At `session_shutdown`, re-add only that remembered subset while preserving every other current active-tool choice; a replacement extension instance will apply the policy again after reload or session replacement. Do not disable `write`, because creation and whole-file replacement remain separate capabilities.

The active `src` prompt must communicate these rules explicitly, and every flat `promptGuidelines` bullet must name `src` so Pi does not append ambiguous guidance:

- Prefer `src` symbol-aware operations for structured source and Markdown files because they usually require less content and are more efficient.
- Before a symbol-scoped `src` read or mutation, call `src` with action `symbols` for the current file and copy the exact returned ID.
- A `src` symbol ID is the opaque ID returned by `src` action `symbols`; it is not the displayed symbol name and must never be guessed or constructed from a function, class, method, variable, or section name.
- `src` action `edit` does not require `symbols` when the exact original text is already known.
- When exact text is not already known, prefer `src` action `symbols` followed by a symbol-aware read or mutation.
- Refresh `src` symbols after a structural edit before another symbol-scoped operation because IDs may have changed.
- For multiple disjoint exact replacements in one file, use one `src` action `edit` with multiple entries. Entries match the original file and must not overlap.

### `web` interface

The `web` tool exposes these MCP-equivalent action objects: `{ action: "search", query }`; `{ action: "fetch", url, tree?, section_id?, full?, tree_threshold? }`; `{ action: "docs_resolve", query }`; `{ action: "docs_fetch", library_id, topic?, tokens? }`; and `{ action: "sgraph", query, count?, context?, timeout? }`. Preserve the current MCP defaults and validation, including fetch tree/section/full/tree-threshold controls, docs topic/token controls, and Sourcegraph count/context/timeout controls. Preserve the existing structured result shapes rather than returning human CLI text inside JSON. Add structured CLI output through the existing internal Web service so CLI, MCP, and extension retain the same backend selection, configuration, cancellation, caching, and error behavior.

This PR does not replace Web fetching with TypeScript. Defuddle is known to be usable as an npm dependency, but native TypeScript `web.fetch` is a separate design and delivery spec.

### `project` interface

The `project` tool exposes `{ action: "list", include_archived? }` and `{ action: "get", alias }`, equivalent to `project_list` and `project_get`. Preserve the false default for `include_archived`, exact alias validation, registry reload-on-call behavior, and the five-field project record. The results remain `{ projects: [...] }` and `{ project: ... }` respectively. Reuse and complete the CLI's existing JSON support rather than introducing another adapter-specific representation.

### `og` interface

The `og` tool exposes the twelve existing MCP operations with these action names and fields:

- `auth_status` and `pull`: required `project`.
- `push`: required `project`, optional `force` defaulting to false.
- `clone`: exactly one selector mode—an exact registered `project`, or an HTTP(S) `url` with optional `alias` and `reference` defaulting to false.
- `pr_create`: required `project` and `title`, optional `body`.
- `pr_find`: required `project`, optional `state` defaulting to `open` and limited to `open`, `closed`, or `all`.
- `pr_get` and `pr_checks`: required `project`, optional positive `pr_id`.
- `pr_modify`: required `project`, optional positive `pr_id`, and at least one of `title` or `body`; an empty body explicitly clears it.
- `pr_comment`: required `project` and nonblank `body`, optional positive `pr_id`.
- `pr_log` and `pr_failures`: required `project`, optional positive `pr_id`, and `tail` defaulting to 50 with range 0–1000.

Preserve the current MCP structured result shapes for project/auth/PR/comment/message/clone/line results. Multiline PR bodies and comments travel through stdin at the CLI seam.

The extension and CLI must not accept caller-selected worktree paths, roots, file URIs, credentials, or token environment names. Registered operations resolve an exact project alias through the existing project store; clone preserves the existing project-or-URL selector. All calls continue through the OG daemon. Force push remains force-with-lease and forbidden on the default branch; archive and remote trust restrictions remain unchanged.

### Repository documentation

Document installation of each package independently, supported platforms, removal of obsolete Pi MCP configuration, the `src` replacement behavior, and the distinction between symbol IDs and display names. Retain MCP documentation for web, project, og, and skill; remove src MCP claims. Document that the four extensions are Pi-native adapters while the remaining MCP servers continue serving non-Pi clients.

## Testing Decisions

Use existing internal service and CLI command tests as the highest Go seams. Expand them to prove JSON output by decoding stdout and asserting domain results, defaults, validation errors, stderr separation, and cancellation where applicable. Keep MCP contract tests for web, project, and og and add parity cases that send equivalent inputs through CLI JSON and MCP adapters to the same fake service/daemon seam. Do not test source text or documentation wording.

For `src`, move reusable inspector behavior out of MCP-only tests and verify it through trusted-file and CLI seams. Cover relative and absolute path resolution at the extension seam; ordinary text reads and symbol-relative reads with one-indexed line pagination and 2,000-line/50-KB continuation; unsupported-structure files still readable as text; Markdown sections; code symbols with attached docs; UTF-8, BOM, CRLF, oversized first lines, and clear errors for missing files and invalid symbol IDs. Verify that symbol names are not accepted as IDs. Cover signature-based PNG/JPEG/GIF/WebP/BMP reads, image content conversion, orientation/size safety, non-vision-model notes, animated PNG rejection, and unsupported binary rejection.

Test batch edit behavior through the Go editing interface and CLI JSON integration: multiple disjoint replacements succeed atomically; every match is computed from original content; empty, missing, duplicate, overlapping, nested, and no-op replacements fail without modifying the file; BOM and line endings survive; and one aggregate diff/patch describes the result. Verify symbol mutations still use exact returned IDs and that IDs can change after structural edits.

At the extension seam, instantiate each extension against a test Pi extension API or narrow fake and assert registered tool names, discriminated schemas, prompt contributions, fixed argv/stdin mapping, JSON result conversion, cancellation, and error conversion. For `pi-src`, additionally verify provenance-based deactivation/restoration of only built-in read/edit and that every mutation uses the resolved target's file queue. Exercise one end-to-end tool call per action family against test binaries rather than asserting adapter source shape.

Package tests build and pack each main package, stage representative native artifacts, and verify platform resolution and exact dependency versions without network access. CI runs pnpm formatting/typechecking/tests/build plus full Go CI because the change spans several CLIs, shared source behavior, repository build tooling, and release automation. Cross-build all three supported targets; execute behavioral tests on available host architectures and mechanically inspect non-host artifacts for the expected executable and package metadata.

Release-workflow tests or dry-run scripts must prove that a tag version maps to all sixteen npm package manifests and GoReleaser artifacts, that native packages are ordered before main packages, and that no package references `latest` or an unmatched version.

## Out of Scope

- A TypeScript/Defuddle implementation of `web.fetch` or any other Web action.
- Windows and Intel macOS binaries.
- Runtime downloads from GitHub Releases, source compilation during npm installation, or PATH fallback.
- A replacement `src` MCP server or backward compatibility for the removed project-scoped src MCP.
- Removing the web, project, og, or skill MCP servers.
- Replacing Pi's built-in `write`, grep, find, ls, or bash tools.
- Publishing a combined umbrella package that installs all four extensions.
- New Organon capabilities unrelated to the existing CLI/MCP operations and the required src read/edit replacement contract.

## Further Notes

The repository has no ADR directory; its README and package-level tests are the current architectural record. Existing Web service fakes, in-memory MCP sessions, project-store fixtures, OG daemon caller fakes, source inspector tests, and CLI subprocess tests provide the relevant prior art.

This is intentionally one release feature despite touching four adapters: the observable outcome is a version-locked, independently installable Pi tool family backed by machine-readable Organon CLIs. Work may be ticketed by shared packaging/release, src, project, web, and og slices, but all slices land in the same PR so package versions and release automation cannot drift.
