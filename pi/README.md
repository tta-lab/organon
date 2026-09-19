# Organon Pi extension

`@tta-lab/pi-src` is the sole native Organon Pi extension. It overrides Pi's
built-in `read` and `edit` tools with structure-aware `src` operations, while
preserving Pi-compatible exact multi-edit behavior.

## Install

```bash
pi install npm:@tta-lab/pi-src@<version>
```

The package selects one platform-matched optional native binary on supported
hosts: darwin/arm64, linux/x64, or linux/arm64. Project discovery and guarded
forge operations are available through the existing `og mcp` integration rather
than through a native Pi package.

## Read and edit workflow

`pi-src` is a native override for Pi's normal `read` and `edit` capabilities.
It keeps ordinary path reads and exact text edits compatible with Pi while
adding symbol-aware navigation for supported source files and Markdown heading
sections.

For structured work, first request an outline and copy the exact returned ID:

```ts
read({ path: "internal/service.go", symbols: true });
read({ path: "internal/service.go", symbol_id: "bK" });
edit({ path: "internal/service.go", operation: "replace", symbol_id: "bK", content: "..." });
```

Symbol IDs are opaque. Do not substitute a displayed name, line number, or a
guessed ID. IDs normally survive body and line-only edits, but a rename or
structural change can invalidate them. Use the outline returned after a symbol
mutation as the next authoritative view, and refresh the outline when another
edit could have made IDs stale.

### Read forms

- `read({ path })` and `read({ path, offset, limit })` retain normal Pi reads.
- `read({ path, symbols: true })` returns the current source/heading outline.
- `read({ path, symbol_id, offset, limit })` reads or paginates one returned
  source symbol or Markdown section.

`symbols: true` is outline mode and rejects pagination. A selected `symbol_id`
read supports `offset` and `limit`. Large ordinary reads use Pi-compatible
truncation; use a symbol read when the relevant region is known.

### Edit forms

- `edit({ path, edits: [{ oldText, newText }] })` applies exact text edits.
- `replace` requires `symbol_id` and `content`.
- `insert` requires `symbol_id`, `position: "before" | "after"`, and `content`.
- `delete` requires `symbol_id`.
- `comment` requires a source-symbol `symbol_id` and documentation `content`.

Exact edits are preferable when the original text is already known. Each
`oldText` must identify the intended current text exactly; use a single call
for multiple disjoint replacements. Keep exact-text and symbol operations in
separate calls, and treat an exact edit as potentially invalidating older IDs.
Symbol mutations return a diff, patch, and updated outline; exact edits return
their diff and patch details for practical review.

## Operator notes

The package contains only JavaScript plus a platform-matched `src` binary. A
failed native resolution usually means the host is unsupported or optional npm
dependencies were omitted. Reinstall without omitting optional dependencies;
do not copy a binary across platforms or replace an installed binary manually.

Pi does not receive a project or forge native adapter. Configure its existing
MCP client to use `og mcp` when those typed tools are needed; this package does
not create or modify that client configuration.

## Development

From this directory, use the checked-in pnpm lockfile:

```bash
pnpm install --frozen-lockfile
pnpm format:check
pnpm typecheck
pnpm build
pnpm test
```

`make pi-build` builds and stages only the host `src` binary before building the
workspace. Tests create disposable files, packages, and installation roots; do
not test against installed native binaries or live Pi configuration.

When changing extension behavior, run the narrow Pi checks from this directory:

```bash
pnpm format:check
pnpm typecheck
pnpm build
pnpm test
node scripts/test-release-invariants.mjs
```

## Releases

The release plan contains three `@tta-lab/pi-src-<platform>` native packages
followed by `@tta-lab/pi-src`. GoReleaser still releases the normal `og` CLI,
but og is not part of native Pi packaging. Validate the offline release plan:

```bash
node scripts/test-release-invariants.mjs
```

Release publication is handled only by the repository release workflow. This
documentation neither publishes packages nor changes installed Pi clients.
