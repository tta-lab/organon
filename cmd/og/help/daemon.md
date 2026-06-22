# og daemon

Run and manage the `og` daemon.

V1 behavior:

- `og daemon run` runs the daemon in the foreground.
- `og daemon install` installs the user service.
- `og daemon uninstall` removes the user service.
- `og daemon start`, `stop`, and `restart` control the user service.
- `og daemon status` checks local process/socket state.
- `og daemon health` checks daemon readiness.

The macOS implementation writes a launchd plist for launchctl. Linux writes a
systemd user service.
