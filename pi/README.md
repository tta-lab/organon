# Organon Pi Extensions

Four independently installable [Pi](https://pi.dev) extension packages give Pi
native access to Organon capabilities without MCP configuration. Each package
registers exactly one global tool and carries a matching native Go binary for
your platform, so installing the npm package is sufficient on a supported
host.

| Package               | Tool      | Capability                                                                                       |
| --------------------- | --------- | ------------------------------------------------------------------------------------------------ |
| `@tta-lab/pi-src`     | `src`     | Structure-aware file reading and editing (replaces Pi's built-in `read` and `edit` while active) |
| `@tta-lab/pi-web`     | `web`     | Web search, page fetch, library documentation, Sourcegraph code search                           |
| `@tta-lab/pi-project` | `project` | List and get registered projects                                                                 |
| `@tta-lab/pi-og`      | `og`      | Guarded Git and forge operations through the package-local og binary                             |

## Install

```bash
pi install npm:@tta-lab/pi-src@<version>
pi install npm:@tta-lab/pi-web@<version>
pi install npm:@tta-lab/pi-project@<version>
pi install npm:@tta-lab/pi-og@<version>
```

Install only the capabilities you need; each package is independent. All
packages in one release share a version, and each main package pins its native
platform packages to that exact version, so schemas and binaries can never
drift.

### Supported platforms

- macOS (Darwin) ARM64
- Linux x64 (amd64)
- Linux ARM64

Unsupported platforms fail at extension startup with an actionable error.
Runtime binaries are never downloaded from GitHub releases, compiled during
installation, or resolved from `PATH`.

## `src` replaces built-in `read` and `edit`

While `@tta-lab/pi-src` is active, Pi's built-in `read` and `edit` tools are
deactivated and `src` becomes the single file inspection and editing interface.
At session start the extension removes only the active built-ins it displaces;
at session shutdown it restores exactly that remembered subset, preserving every
other active-tool choice you have made. `write` is never disabled: creating
files and whole-file replacement remain separate capabilities.

`src` resolves paths relative to Pi's current working directory (or accepts
absolute paths), has no project-registry dependency, and normalizes an
accidental leading `@` like Pi's built-in tools do.

### Symbol IDs are opaque

The `src` tool returns a typed outline from its `symbols` action. Each symbol
has an **opaque ID** (for example `bK`) and a **display name** (for example
`handleRequest`). Symbol-scoped operations (`read`, `replace`, `insert`,
`delete`, `comment`) accept only the exact ID returned by `symbols` — never the
display name. IDs can change after structural edits, so refresh the outline
before another symbol-scoped operation.

### Exact text edits

The `edit` action performs exact multi-edit replacement on one file without any
symbol lookup: every `oldText` must match one unique region of the original
file, entries must not overlap, and all replacements are applied atomically in a
single write (BOM and line endings preserved).

## MCP servers

The `web`, `project`, `og`, and `skill` MCP servers continue to serve non-Pi
clients and are unchanged. Pi users who install the matching `web`, `project`,
`og`, or `src` extension should remove that capability's duplicate MCP server
configuration from Pi, so it exposes one native tool per capability. The old
project-scoped `src` MCP server has been removed: Pi uses the local-path `src`
extension instead, and CLI users use the normal `src` command. If you previously
configured `organon-src` in an MCP client, remove that server entry.

A native TypeScript/Defuddle implementation of `web.fetch` is a separate future
spec; this repository's Web extension is a thin adapter over the Go CLI.

## CLI JSON output

The commands behind these extensions also accept `--json` for machine-readable
output (for example `project list --json`, `web fetch <url> --json`, `src read
<file> --json`, `og pr find --project <alias> --json`). Diagnostics stay on
stderr; stdout carries exactly one JSON document.

## Development

This workspace is pnpm-based. The shared TypeScript subprocess adapter is
private and bundled into each extension (there is no fifth published runtime
package).

```bash
pnpm install
pnpm exec prettier --write .
pnpm exec tsc -p tsconfig.json --noEmit
pnpm exec vitest run
pnpm -r --filter './packages/pi-*' run build
node scripts/test-release-invariants.mjs
```

Releases are tag-driven: `scripts/sync-version.mjs` maps the tag to all sixteen
manifests, `scripts/stage-natives.mjs` copies the cross-compiled Go binaries
into the native packages (failing on a missing or wrong-platform artifact),
and `scripts/publish-packages.mjs` supplies the one native-first publish plan
consumed by the release workflow. `scripts/release-dry-run.mjs` validates that
same plan together with exact-version and GoReleaser artifact invariants before
publishing.

### Local debugging

An extension resolves only its host platform's native package binary (for
example `@tta-lab/pi-src-darwin-arm64/bin/src`); it never reads `PATH` or
`$GOBIN`. To debug against a real Go binary, build the current-platform binary
into the matching native package, then load the extension directly or install
the package locally:

```bash
pnpm --dir pi install
pnpm --dir pi run build

CGO_ENABLED=0 go build -o pi/packages/native/pi-src-darwin-arm64/bin/src ./cmd/src
file pi/packages/native/pi-src-darwin-arm64/bin/src   # Mach-O 64-bit arm64

pi -e ./pi/packages/pi-src/dist/index.js
# or
pi install /Users/neil/code/guion-opensource/organon/pi/packages/pi-src
```

The pi-src/web/project/og tests resolve their binaries from the workspace native
packages during the run; vitest's global setup stages the fixture binaries
there and its teardown removes them immediately after, so a local session never
resolves a leftover fixture. The offline pack smoke works entirely from
throwaway copies and never writes into the workspace native packages. Replace
the host-platform binary name (`darwin-arm64`, `linux-x64`, `linux-arm64`) and
the tool as needed.
