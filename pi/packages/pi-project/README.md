# @tta-lab/pi-project

A [Pi](https://pi.dev) extension for discovering registered Organon projects.
Its `project_list`, `project_find`, and `project_get` tools list projects, find
matching registrations, and return the full record for a selected project
without MCP configuration.

## Install

```bash
pi install npm:@tta-lab/pi-project@latest
# or pin a release:
pi install npm:@tta-lab/pi-project@<version>
```

Prereleases use the `beta` dist-tag and must be requested explicitly:

```bash
pi install npm:@tta-lab/pi-project@beta
```

The package is independently installable and includes the matching native
`@tta-lab/pi-project-<platform>` optional dependency at the exact same version.

## Supported platforms

- macOS (Darwin) ARM64
- Linux x64 (amd64)
- Linux ARM64

Unsupported hosts fail at startup instead of downloading or compiling a binary.

## Source

This package is maintained in the Organon monorepo at
[`pi/packages/pi-project`](https://github.com/tta-lab/organon/tree/main/pi/packages/pi-project).
The platform-specific implementation packages live under
[`pi/packages/native`](https://github.com/tta-lab/organon/tree/main/pi/packages/native).
See the [Pi extension documentation](https://github.com/tta-lab/organon/blob/main/pi/README.md)
for the complete tool and installation details.
