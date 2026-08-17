# @tta-lab/pi-src

A [Pi](https://pi.dev) extension that overrides Pi's built-in `read` and `edit`
tools with Organon’s structure-aware source and Markdown navigation, symbol
scoped mutations, and atomic exact-text edits.

## Install

```bash
pi install npm:@tta-lab/pi-src@latest
# or pin a release:
pi install npm:@tta-lab/pi-src@<version>
```

Prereleases use the `beta` dist-tag and must be requested explicitly:

```bash
pi install npm:@tta-lab/pi-src@beta
```

The package is independently installable and includes the matching native
`@tta-lab/pi-src-<platform>` optional dependency at the exact same version.

## Supported platforms

- macOS (Darwin) ARM64
- Linux x64 (amd64)
- Linux ARM64

Unsupported hosts fail at startup instead of downloading or compiling a binary.

## Source

This package is maintained in the Organon monorepo at
[`pi/packages/pi-src`](https://github.com/tta-lab/organon/tree/main/pi/packages/pi-src).
The platform-specific implementation packages live under
[`pi/packages/native`](https://github.com/tta-lab/organon/tree/main/pi/packages/native).
See the [Pi extension documentation](https://github.com/tta-lab/organon/blob/main/pi/README.md)
for the complete tool forms and symbol-ID workflow.
