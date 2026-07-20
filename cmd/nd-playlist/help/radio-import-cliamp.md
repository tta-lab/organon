Import Cliamp's built-in internet radio stations into Navidrome.

The command first lists the server's existing stations and compares their stream
URLs, so it does not create duplicates. It previews changes by default:

  nd-playlist radio import-cliamp
  nd-playlist radio import-cliamp --yes

`--yes` creates only the missing stations. This requires a Navidrome admin
account because Internet Radio changes are global.
