Count LLM tokens using the cl100k_base tokenizer (GPT-4, Claude).
Auto-detects whether the argument is a file path or literal text.

## When to use
  - Estimating prompt or response cost before sending
  - Checking token counts for context window limits
  - Understanding how text tokenizes (with -v)

## Examples
  token "hello world"            # literal text (2 tokens)
  token ./path/to/file.go       # reads file, counts tokens
  token -f ./file.go            # force file mode
  token -v "hello world"        # show token names: hello  world

## Notes
  - cl100k_base is a 100k BPE tokenizer; whitespace is significant
  - UUIDs are expensive (~18 tokens) due to hex digit fragmentation
  - Digit sequences often get their own token
