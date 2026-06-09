---
name: organon-token
description: Use token to count LLM tokens in text or files. Supports positional arg as text or file path, auto-detects which. Run token --help for details.
category: tool
---
# token -- LLM Token Counter

Count LLM tokens using the cl100k_base tokenizer (GPT-4, Claude).
Run `--help` for full reference:

    token --help

Quick syntax:

    token "hello world"            # literal text
    token ./path/to/file.go       # reads file
    token -f ./file.go            # force file mode
    token -v "hello world"        # show token names
