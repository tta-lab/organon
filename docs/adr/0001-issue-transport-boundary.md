# Issue writes and multiline input belong to MCP

Issue workflows support both GitHub and Forgejo. Reads are exposed through
CLI and MCP; all issue writes use typed MCP tools, including title updates.
This deliberately gives up CLI write parity in favor of a single structured
write interface for agents. All future capabilities accepting potentially
multiline input are also MCP-only, rather than adding stdin or file-based CLI
adapters. Existing CLI implementations are not removed by this design session.

Issue creation requires both title and body. Subsequent title updates, whole-body
replacement, and old/new body editing are separate tools: replacing the body
does not change the title. Comments are separate discussion entries, with
reading and adding comments included in the initial scope.
