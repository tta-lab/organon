Queries Sourcegraph's public GraphQL API to search code across public
repositories. Uses Sourcegraph query syntax: repo:, file:, lang:, type:symbol,
regex patterns, and boolean operators (AND/OR/NOT).

Examples:
  web sgraph "repo:^github\.com/golang/go$ fmt.Println"
  web sgraph "lang:go context.WithTimeout" --count 20
  web sgraph "file:Dockerfile alpine" --context 15 --timeout 60
  web sgraph "lang:typescript useState type:symbol"

Only searches public repositories. Unauthenticated; rate limits may apply.
