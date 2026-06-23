# og daemon

Run and manage the `og` daemon.

V1 behavior:

- `og daemon run` runs the daemon in the foreground.
- `og daemon install` installs the user service.
- `og daemon uninstall` removes the user service.
- `og daemon start`, `stop`, and `restart` control the user service.
- `og daemon status` checks local process/socket state.
- `og daemon health` checks daemon readiness.

The daemon listens on a Unix socket by default. The macOS implementation follows
the ttal launchd pattern: the plist contains the daemon command, log paths, and
PATH only. Secrets are loaded by `og daemon run` from `~/.config/ttal/.env` when
the process starts, without overriding existing environment variables. Linux
writes a systemd user service.
