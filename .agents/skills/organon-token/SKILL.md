---
name: organon-token
description: Use token to count LLM tokens in text or files. Supports positional arg as text or file path, with -v for verbose token-level breakdown. Uses tiktoken-go with cl100k_base tokenizer (Claude / GPT-4).
---
# token — LLM Token Counting
Use `token` to count how many LLM tokens a text or file consumes. Uses the cl100k_base tokenizer (Claude / GPT-4) via tiktoken-go.
## Basic Usage
```bash
token "hello world"              # positional text
token ./path/to/file.go          # auto-detected file
token -f ./file.go               # force file mode
```
The CLI auto-detects whether the argument is a file path or literal text. Use `-f` to override.
## Verbose Mode
Use `-v` to see individual token names:
```bash
token -v "hello world"
# tokenizer: cl100k_base
# count: 2
# tokens: hello  world
token -v "#+BEGIN_SRC bash"
# tokenizer: cl100k_base
# count: 4
# tokens: #+ BEGIN _SRC  bash
```
The space before `world` and `bash` is part of the token — BPE tokenizers treat whitespace as significant.
## Flags
```bash
token [text | file path] [flags]
  -f, --file      Force arg to be treated as a file path
  -v, --verbose   Show individual token names
  -h, --help      help for token
```
## Tokenizer Info
- **Tokenizer**: cl100k_base (100,000 token vocabulary)
- **Used by**: GPT-4, GPT-3.5-turbo, Claude (Anthropic)
- **Source**: tiktoken-go library
- **Fallback**: regex-based approximate count on tokenizer error
Common patterns:
- Digit sequences (123, 2024) often get their own token
- UUIDs are expensive (~18 tokens) due to hex digit fragmentation
- Whitespace is significant — ` bash` and `bash` are different tokens
