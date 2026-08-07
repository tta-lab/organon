# og daemon

Run and manage the `og` daemon.

Behavior:

- `og daemon run` runs the daemon in the foreground.
- `og daemon validate` checks every registry remote against the complete og
  configuration and validates the configured GitHub App key without binding a
  socket or making a network request.
- `og daemon install` installs the user service.
- `og daemon uninstall` removes the user service.
- `og daemon start`, `stop`, and `restart` control the user service.
- `og daemon status` checks local process/socket state.
- `og daemon health` checks daemon readiness.

The daemon listens on a Unix socket by default. On startup it loads the GitHub
App and Forgejo trust configuration from `~/.config/ttal/og.toml`, the hot
project registry, and the configured GitHub App private key. Invalid
configuration or key material prevents startup.
The key should be readable only by the user and must stay outside repositories.

Only Forgejo server roots listed under `forgejo.allowed_base_urls` can receive
Forgejo credentials or API calls. Other HTTPS origins are anonymous generic Git
and support clone plus fast-forward-only pull; provider APIs and remote writes
are rejected. Unlisted HTTP and non-HTTP(S) origins are rejected.

The macOS implementation follows the ttal launchd pattern: the plist contains
the daemon command, log paths, PATH, and standard proxy variables present when
`og daemon install` runs. Reinstall after changing the proxy. `~/.config/ttal/.env`
is still loaded for non-GitHub services such as Forgejo, without overriding
existing environment variables. Linux writes a systemd user service.
