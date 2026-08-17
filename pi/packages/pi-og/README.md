# @tta-lab/pi-og

A [Pi](https://pi.dev) extension for guarded Organon Git and forge operations.
It provides `og_clone`, `og_pull`, `og_push`, `og_pr`, and `og_checks` through
the package-local `og` binary, without MCP configuration. Authentication status
remains available to CLI maintenance users but is not registered as a Pi tool.

## Install

```bash
pi install npm:@tta-lab/pi-og@latest
# or pin a release:
pi install npm:@tta-lab/pi-og@<version>
```

Prereleases use the `beta` dist-tag and must be requested explicitly:

```bash
pi install npm:@tta-lab/pi-og@beta
```

The package is independently installable and includes the matching native
`@tta-lab/pi-og-<platform>` optional dependency at the exact same version.

## Supported platforms

- macOS (Darwin) ARM64
- Linux x64 (amd64)
- Linux ARM64

Unsupported hosts fail at startup instead of downloading or compiling a binary.

## Source

This package is maintained in the Organon monorepo at
[`pi/packages/pi-og`](https://github.com/tta-lab/organon/tree/main/pi/packages/pi-og).
The platform-specific implementation packages live under
[`pi/packages/native`](https://github.com/tta-lab/organon/tree/main/pi/packages/native).
See the [Pi extension documentation](https://github.com/tta-lab/organon/blob/main/pi/README.md)
for the complete tool and installation details.
