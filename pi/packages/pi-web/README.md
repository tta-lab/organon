# @tta-lab/pi-web

A [Pi](https://pi.dev) extension that provides native Organon web capabilities:
web search, page fetching, library documentation lookup, and Sourcegraph code
search without MCP configuration.

## Install

```bash
pi install npm:@tta-lab/pi-web@latest
# or pin a release:
pi install npm:@tta-lab/pi-web@<version>
```

Prereleases use the `beta` dist-tag and must be requested explicitly:

```bash
pi install npm:@tta-lab/pi-web@beta
```

The package is independently installable and includes the matching native
`@tta-lab/pi-web-<platform>` optional dependency at the exact same version.

## Supported platforms

- macOS (Darwin) ARM64
- Linux x64 (amd64)
- Linux ARM64

Unsupported hosts fail at startup instead of downloading or compiling a binary.

## Source

This package is maintained in the Organon monorepo at
[`pi/packages/pi-web`](https://github.com/tta-lab/organon/tree/main/pi/packages/pi-web).
The platform-specific implementation packages live under
[`pi/packages/native`](https://github.com/tta-lab/organon/tree/main/pi/packages/native).
See the [Pi extension documentation](https://github.com/tta-lab/organon/blob/main/pi/README.md)
for the complete action and installation details.
