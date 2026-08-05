# og daemon

Run and manage the `og` daemon.

Behavior:

- `og daemon run` runs the daemon in the foreground.
- `og daemon install` installs the user service.
- `og daemon uninstall` removes the user service.
- `og daemon start`, `stop`, and `restart` control the user service.
- `og daemon status` checks local process/socket state.
- `og daemon health` checks daemon readiness.

The daemon listens on a Unix socket by default. On startup it loads the GitHub
App configuration from `~/.config/ttal/og.toml` and its private key from the
configured file source. Invalid configuration or key material prevents startup.
The key should be readable only by the user and must stay outside repositories.

The macOS implementation follows the ttal launchd pattern: the plist contains
the daemon command, log paths, PATH, and standard proxy variables present when
`og daemon install` runs. Reinstall after changing the proxy. `~/.config/ttal/.env`
is still loaded for non-GitHub services such as Forgejo, without overriding
existing environment variables. Linux writes a systemd user service.
