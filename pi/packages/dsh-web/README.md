# @tta-lab/dsh-web

DeepSeek Harness rc.8 Web bundle and browser settings client for Organon search.

Install it into the existing Web profile:

```bash
dsh plugin --profile web add @tta-lab/dsh-web
```

The package owns the Web profile patch and does not require a custom profile or
PTC preset. It leaves the root `tool-web` row disabled and routes stock PTC's
batched `web_search` through the Organon provider seam.

Provider selection is explicit and persists in the `organon-web` settings
namespace. Exa and Brave API keys use namespaced write-only DSH credentials;
settings expose only configured/source/writable metadata.

The published package is a dual host/browser bundle. Its host and client artifacts,
profile patch, and exact DSH `0.1.0-rc.8` peer contract are included in the npm
package. It reuses the optional `@tta-lab/pi-web-*` native package family for
search, docs, and Sourcegraph; Windows x64 uses `@tta-lab/pi-web-win32-x64` and
`web.exe`. There is no DSH-specific native binary family.

The first public release requires a maintainer to associate `@tta-lab/dsh-web`
with the repository's npm Trusted Publisher for `.github/workflows/release.yaml`
and Environment `npm`. Tests and local pack/smoke commands never configure that
relationship or publish a package; see the release checklist in `pi/README.md`.
