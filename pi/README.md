# Organon Pi Extensions

Four independently installable [Pi](https://pi.dev) extension packages give Pi
native access to Organon capabilities without MCP configuration. Each package
carries a matching native Go binary for your platform, so installing the npm
package is sufficient on a supported host. `pi-src`, `pi-web`, and `pi-project`
each register capability tools; `pi-og` registers five guarded forge tools.

| Package               | Tool(s)                                                | Capability                                                               |
| --------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------ |
| `@tta-lab/pi-src`     | `read`, `edit`                                         | Structure-aware file reading and editing through exact-name Pi overrides |
| `@tta-lab/pi-web`     | `web_search`, `web_fetch`, `web_docs`, `web_sgraph`    | Web search, page fetch, library documentation, Sourcegraph code search   |
| `@tta-lab/pi-project` | `project_list`, `project_find`, `project_get`          | List, find, and get registered projects                                  |
| `@tta-lab/pi-og`      | `og_clone`, `og_pull`, `og_push`, `og_pr`, `og_checks` | Guarded Git and forge operations through the package-local og binary     |

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
- Windows x64 (Pi Web only)

Windows x64 support is limited to `@tta-lab/pi-web`; the other extension packages
retain their Darwin/Linux native targets. Unsupported platforms fail at extension
startup with an actionable error.
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

The first returns the current outline. The second reads exactly one source
symbol or Markdown heading section, with offset and limit relative to that
selected content. Source symbols and every Markdown heading section, including
H1, use the same outline → opaque `symbol_id` workflow. Outline results do not accept pagination
fields. IDs are deterministic from canonical symbol or heading labels, so body,
content, and line-only edits normally preserve unchanged IDs; renames or
structural changes may not. Treat the latest returned outline as authoritative,
and never use a display name as an ID.

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

Source symbols and Markdown heading sections use the same opaque-ID workflow
for symbol-scoped `read`, `replace`, `insert`, and `delete`; source comment
operations use that same `symbol_id` form for documentation. Each
successful symbol mutation returns the typed current post-edit outline in the
same result, including an empty outline when no symbols remain. Continue from
that returned outline instead of making a redundant outline read; refresh only
when a later edit may have made IDs stale or truncation omitted the needed
entry. Combine disjoint exact replacements in one `edit` call. Symbol mutation
results use Pi's built-in-compatible edit details: display diff, unified patch,
and optional first changed line. Added symbol forms may not have a
pre-execution exact-text preview, but the inherited edit renderer can render
their returned diff normally.

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

The Pi `web_fetch` tool uses the bundled TypeScript Defuddle implementation
and does not require a globally installed `defuddle` CLI. `web_search`,
`web_docs`, and `web_sgraph` remain thin adapters over the Go CLI. The grouped
`web_docs` tool uses `resolve` and `fetch` actions.

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

`make pi-build` detects supported Darwin/Linux hosts, builds the four CGO-disabled
Go binaries directly into the matching native packages, installs the frozen Pi
lockfile, and builds all four extension bundles. The Windows web package is
cross-compiled from the GoReleaser `windows/amd64` web target and is smoke-tested
on the Windows CI runner. Staging recreates missing native `bin` directories and
rejects unsupported or wrong-platform artifacts.

Releases are tag-driven: `scripts/sync-version.mjs` maps the tag to the single
package publish plan, `scripts/stage-natives.mjs` copies the
cross-compiled Go binaries into the native packages (failing on a missing or
wrong-platform artifact), and `scripts/publish-packages.mjs` supplies the one
native-first plan consumed by both local bootstrap and the release workflow.
`scripts/release-dry-run.mjs` validates that same plan together with exact-version
and GoReleaser artifact invariants before publishing.

## Publishing and releases

Organon publishes four independently installable main packages and the
platform packages discovered by the release plan. Every release uses one
synchronized version and publishes all native packages before the main packages:

| Main package          | Native optional packages                                                                                                |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `@tta-lab/pi-src`     | `@tta-lab/pi-src-darwin-arm64`, `@tta-lab/pi-src-linux-x64`, `@tta-lab/pi-src-linux-arm64`                              |
| `@tta-lab/pi-web`     | `@tta-lab/pi-web-darwin-arm64`, `@tta-lab/pi-web-linux-x64`, `@tta-lab/pi-web-linux-arm64`, `@tta-lab/pi-web-win32-x64` |
| `@tta-lab/pi-project` | `@tta-lab/pi-project-darwin-arm64`, `@tta-lab/pi-project-linux-x64`, `@tta-lab/pi-project-linux-arm64`                  |
| `@tta-lab/pi-og`      | `@tta-lab/pi-og-darwin-arm64`, `@tta-lab/pi-og-linux-x64`, `@tta-lab/pi-og-linux-arm64`                                 |

Stable SemVer versions use the `latest` dist-tag. SemVer prereleases use
`beta`, so a beta must be installed explicitly with (for example)
`pi install npm:@tta-lab/pi-src@beta`. Before each package publish, the shared
plan queries the public registry for the exact name and version; an exact
existing version is skipped, while authentication, connectivity, malformed
metadata, and ambiguous registry failures stop the release. Versions are never
overwritten or unpublished.

### One-time local npm bootstrap

The first beta is a maintainer-only operation after this change has merged. Run
it only from a clean temporary checkout of the intended release commit; do not
bootstrap from a dirty development tree. The commands below use a temporary npm
config so the login credential is removed with the checkout:

```bash
set -eu
RELEASE_COMMIT=<intended-release-commit>
RELEASE_VERSION=<next-version>-beta.1
TMP_HOME="$(mktemp -d)"
NPM_CONFIG_USERCONFIG="$(mktemp "${TMPDIR:-/tmp}/organon-npmrc.XXXXXX")"
export HOME="$TMP_HOME"
export NPM_CONFIG_USERCONFIG
export NPM_CONFIG_PROVENANCE=false
trap 'rm -f "$NPM_CONFIG_USERCONFIG"; rm -rf "$TMP_HOME"' EXIT

# og derives the checkout under this disposable HOME; it is not registered in
# the maintainer's normal project registry. The local tag below is never pushed.
og clone https://github.com/tta-lab/organon.git
cd "$HOME/code/projects/tta-lab/organon"
git checkout --detach "$RELEASE_COMMIT"
test -z "$(git status --porcelain)"

case "$RELEASE_VERSION" in
  *-*) ;;
  *) echo "bootstrap version must be a beta prerelease" >&2; exit 1 ;;
esac
RELEASE_TAG="v$RELEASE_VERSION"
# Use npm >= 11.10.0, log in to the account with account-level 2FA enabled,
# and complete the interactive 2FA challenge when npm asks for it.
npm login --registry=https://registry.npmjs.org
npm whoami --registry=https://registry.npmjs.org
# If account-level 2FA is not already enabled, do this once before publishing:
# npm profile enable-2fa auth-and-writes --registry=https://registry.npmjs.org

(cd pi && pnpm install --frozen-lockfile && pnpm -r --filter './packages/pi-*' run build)
# GoReleaser reads the exact version from this ephemeral local tag. --skip=publish
# creates dist artifacts and metadata without creating a GitHub release; never
# run og push, git push, or a publish command against this checkout's tag.
git tag --force "$RELEASE_TAG" "$RELEASE_COMMIT"
goreleaser release --clean --skip=publish
(cd pi && node scripts/stage-natives.mjs ../dist)
for f in pi/packages/native/pi-*/bin/*; do
  test -x "$f"
done

(cd pi && node scripts/test-release-invariants.mjs ../dist)
(cd pi && node scripts/sync-version.mjs "$RELEASE_VERSION")
(cd pi && node scripts/release-dry-run.mjs "$RELEASE_VERSION" ../dist)
(cd pi && pnpm exec vitest run)
(cd pi && node scripts/publish-packages.mjs)
```

The last command is the same native-first npm CLI publish plan used by the
routine release. It uses `npm view` for exact-version resumability and
`npm publish --access public --tag beta`; it does not use `pnpm publish`. If a
publish is interrupted, rerun the checks and the same command: immutable
versions already on the registry are skipped.

### One-time npm Trusted Publisher setup

After all packages in the publish plan exist, bind each package to the same GitHub workflow
and protected Environment. npm trust commands require npm >= 11.10.0, package
write permission, and account-level 2FA. Run this loop once from the repository
root of the clean bootstrap checkout; the first request may ask for 2FA.
The two-second delay avoids npm rate limiting:

```bash
set -eu
cd pi
for package in $(node --input-type=module -e '
  import { packagePublishPlan } from "./scripts/publish-packages.mjs";
  for (const entry of packagePublishPlan()) console.log(entry.name);
'); do
  npm trust github "$package" \
    --repository tta-lab/organon \
    --file release.yaml \
    --environment npm \
    --allow-publish \
    --yes \
    --registry https://registry.npmjs.org
  sleep 2
done
```

Verify every relationship before enabling routine releases. Each command must
report exactly one GitHub trusted publisher for `tta-lab/organon`, workflow
`.github/workflows/release.yaml`, Environment `npm`, and publish permission:

```bash
cd pi
for package in $(node --input-type=module -e '
  import { packagePublishPlan } from "./scripts/publish-packages.mjs";
  for (const entry of packagePublishPlan()) console.log(entry.name);
'); do
  echo "--- $package ---"
  npm trust list "$package" --json --registry=https://registry.npmjs.org
done
```

Do not enable the normal release path until every publish-plan output matches those
claims. If setup must be corrected, inspect the trust ID with `npm trust list`,
revoke only the incorrect relationship, and configure that package again.

Finally, configure the token restriction separately for each package in the npm
website: open the package **Settings**, open **Publishing access**, and select
**Require two-factor authentication and disallow tokens**. Verify that setting
on every package page in the publish plan before enabling routine releases.
This is the npm policy that blocks traditional publish tokens while retaining
the configured OIDC Trusted Publishers; verify it in the package settings rather
than relying on a CLI 2FA setting.

### GitHub Environment and routine OIDC release

Configure GitHub separately from npm:

1. Create a repository Environment named `npm`.
2. Add the required maintainer reviewer(s), leave maintainer self-approval
   enabled, and restrict deployment branches/tags to release tags matching `v*`.
3. Do not add an npm publish token to the Environment. The gated job uses a
   GitHub-hosted runner, Node 24, npm >= 11.10.0, `contents: write`, and
   `id-token: write`; npm Trusted Publishing supplies the OIDC identity.

The tag workflow has two jobs. `preflight` is ungated and runs the Go gates, Pi
format/typecheck/build/tests, GoReleaser snapshot, native staging, and release
invariants. Its job summary shows the tag, commit, package version, derived
package inventory, target dist-tag, and passed checks. Only after that job succeeds
does the single `release` job enter the `npm` Environment and wait for one
approval. Approving your own deployment is allowed; rejecting or leaving it
unapproved publishes neither the GitHub/Homebrew release nor npm packages.

After approval, that same job performs the real GoReleaser release, builds and
stages the Pi packages, synchronizes the tag version, reruns release invariants
against the real artifacts, and invokes the shared npm CLI plan. There are no
per-package approvals and no token fallback.

For a completed release, verify installation and dist-tags with the public
registry:

```bash
VERSION=<release-version>
for package in @tta-lab/pi-src @tta-lab/pi-web @tta-lab/pi-project @tta-lab/pi-og; do
  npm view "$package@$VERSION" version dist-tags --json --registry=https://registry.npmjs.org
  pi install "npm:$package@$VERSION"
done
```

OIDC releases on the public repository and public packages automatically carry
npm provenance. Verify each main package's attestation and inspect its npm
package page's Provenance section:

```bash
for package in @tta-lab/pi-src @tta-lab/pi-web @tta-lab/pi-project @tta-lab/pi-og; do
  npm view "$package@$VERSION" dist.attestations --json --registry=https://registry.npmjs.org
done
```

The local bootstrap beta intentionally has no provenance; provenance begins
with the gated Trusted Publishing release path.

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
`linux-arm64`, `win32-x64`) and the tool as needed.
