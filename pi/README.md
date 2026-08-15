# Organon Pi Extensions

Four independently installable [Pi](https://pi.dev) extension packages give Pi
native access to Organon capabilities without MCP configuration. Each package
carries a matching native Go binary for your platform, so installing the npm
package is sufficient on a supported host. `pi-src`, `pi-web`, and `pi-project`
each register capability tools; `pi-og` registers six guarded forge tools.

| Package               | Tool(s)                                                                  | Capability                                                               |
| --------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------------------------ |
| `@tta-lab/pi-src`     | `read`, `edit`                                                           | Structure-aware file reading and editing through exact-name Pi overrides |
| `@tta-lab/pi-web`     | `web`                                                                    | Web search, page fetch, library documentation, Sourcegraph code search   |
| `@tta-lab/pi-project` | `project`                                                                | List, find, and get registered projects                                  |
| `@tta-lab/pi-og`      | `og_auth_status`, `og_clone`, `og_pull`, `og_push`, `og_pr`, `og_checks` | Guarded Git and forge operations through the package-local og binary     |

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

## `pi-src` overrides `read` and `edit`

`@tta-lab/pi-src` registers exactly two global tools named `read` and `edit`.
They are exact-name execution overrides of Pi's built-in tools, not a separate
`src` Pi tool. `write` and unrelated tools remain untouched. Ordinary Pi users
can keep using the familiar built-in forms, while the installed Pi version's
live built-in schemas, argument preparation, prompt contribution, and renderer
contract remain the baseline for the overrides.

The same package works in ordinary Pi and in a Fabric release that supports
effective compatible core overrides. Install that Fabric release first and
Organon second. Without the Fabric release, ordinary Pi still supports the
built-in-compatible forms; Fabric's static guest gate simply will not advertise
the added Organon fields yet. The extension has no Fabric dependency.

There is no active-tool takeover or restoration lifecycle. Pi's exact-name
registration is the only replacement mechanism, so reloading or ending a
session does not toggle `read` or `edit` or change the active `write` tool.

### `read` capability forms

In addition to the complete installed Pi built-in read input, the override
accepts these closed object forms:

```json
{"path":"main.go","symbols":true}
{"path":"main.go","symbol_id":"bK","offset":1,"limit":40}
```

The first returns the current outline. The second reads exactly one symbol or
Markdown section, with offset and limit relative to that selected content.
Outline results do not accept pagination fields. Symbol IDs are opaque and must
come from the latest outline; a display name is never a valid ID.

### `edit` capability forms

The complete installed Pi built-in exact-edit input remains valid and keeps its
atomic `edits[]` batch behavior:

```json
{
  "path": "config.yaml",
  "edits": [
    { "oldText": "old value", "newText": "new value" },
    { "oldText": "other", "newText": "updated" }
  ]
}
```

When the exact original text is already known, use this form directly; symbol
discovery is not required. For structure-aware edits, first call `read` with
`symbols: true`, copy an exact opaque ID, and use one of these forms:

```json
{"path":"main.go","operation":"replace","symbol_id":"bK","content":"func handle() {}"}
{"path":"main.go","operation":"insert","symbol_id":"bK","position":"after","content":"func next() {}"}
{"path":"main.go","operation":"delete","symbol_id":"bK"}
{"path":"main.go","operation":"comment","symbol_id":"bK","content":"Handles requests."}
```

Structural edits refresh the file's outline before another symbol operation;
IDs can change after a mutation. Combine disjoint exact replacements in one
`edit` call. Symbol mutation results use Pi's built-in-compatible edit details:
display diff, unified patch, and optional first changed line. Added symbol forms
may not have a pre-execution exact-text preview, but the inherited edit renderer
can render their returned diff normally.

All paths are absolute or relative to Pi's invocation working directory and
retain leading-`@` normalization. The `src` CLI remains available as the public
command name and is unchanged. Exact edits reject missing, ambiguous,
overlapping, nested, duplicate, and no-op replacements atomically while
preserving BOM and line endings.

## MCP servers

The `web`, `project`, `og`, and `skill` MCP servers continue to serve non-Pi
clients and are unchanged. Pi users who install the matching `web`, `project`,
`pi-og`, or `src` extension should remove that capability's duplicate MCP
server configuration from Pi, so it exposes native capability tools without
transport duplicates. The old project-scoped `src` MCP server has been removed:
Pi uses the local-path `read`/`edit` extension, and CLI users use the normal
`src` command. If you previously configured `organon-src` in an MCP client,
remove that server entry.

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
make pi-build
cd pi
pnpm exec prettier --write .
pnpm exec tsc -p tsconfig.json --noEmit
pnpm exec vitest run
node scripts/test-release-invariants.mjs
```

`make pi-build` detects the supported host, builds the four CGO-disabled Go
binaries directly into the matching native packages, installs the frozen Pi
lockfile, and builds all four extension bundles. It recreates missing native
`bin` directories and rejects unsupported hosts instead of staging a binary
under the wrong platform name.

Releases are tag-driven: `scripts/sync-version.mjs` maps the tag to all sixteen
manifests, `scripts/stage-natives.mjs` copies the cross-compiled Go binaries
into the native packages (failing on a missing or wrong-platform artifact), and
`scripts/publish-packages.mjs` supplies the one native-first publish plan
consumed by the release workflow. `scripts/release-dry-run.mjs` validates that
same plan together with exact-version and GoReleaser artifact invariants before
publishing.

### Local debugging

An extension resolves only its host platform's native package binary (for
example `@tta-lab/pi-src-darwin-arm64/bin/src`); it never reads `PATH` or
`$GOBIN`. To debug against real Go binaries, run `make pi-build`, then load the
extension directly or install the package locally:

```bash
make pi-build
file pi/packages/native/pi-src-darwin-arm64/bin/src   # Mach-O 64-bit arm64

pi -e ./pi/packages/pi-src/dist/index.js
# or
pi install /Users/neil/code/guion-opensource/organon/pi/packages/pi-src
```

The pi-src/web/project/og tests resolve fixture binaries from a fresh
temporary package-shaped workspace. Vitest never creates, replaces, or removes
files in repository `packages/native/*/bin` directories, so running the tests
with real binaries present leaves them byte-for-byte unchanged—and running with
them absent leaves them absent. The offline pack smoke also uses throwaway
copies. Replace the host-platform binary name (`darwin-arm64`, `linux-x64`,
`linux-arm64`) and the tool as needed.
