package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tta-lab/organon/internal/srcview"
)

func newSymbolsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "symbols", Args: cobra.ExactArgs(1), RunE: runSymbolsJSON}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func newReadCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "read", Args: cobra.ExactArgs(1), RunE: runReadJSON}
	cmd.Flags().String("symbol-id", "", "")
	cmd.Flags().Int("offset", 0, "")
	cmd.Flags().Int("limit", 0, "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func writeGoFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	f := filepath.Join(dir, "sample.go")
	require.NoError(t, os.WriteFile(f, []byte(content), 0o644))
	return f
}

func decodeOutline(t *testing.T, stdout string) symbolOutlineJSON {
	t.Helper()
	var out symbolOutlineJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	return out
}

func decodeRead(t *testing.T, stdout string) readJSON {
	t.Helper()
	var out readJSON
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	return out
}

func TestSymbolsHumanDefaultPrintsTree(t *testing.T) {
	f := writeGoFile(t, "package sample\n\nfunc Foo() {}\n")
	cmd := newSymbolsCmd()
	out := captureStdout(t, func() {
		require.NoError(t, runSymbols(cmd, []string{f}))
	})
	assert.Contains(t, out, "Foo")
	assert.Contains(t, out, "func")
}

func TestReadHumanDefaultPrintsContent(t *testing.T) {
	f := writeGoFile(t, "package sample\n\nfunc Foo() {}\n")
	cmd := newReadCmd()
	out := captureStdout(t, func() {
		require.NoError(t, runRead(cmd, []string{f}))
	})
	assert.Equal(t, "package sample\n\nfunc Foo() {}\n", out)
}
func TestSymbolsJSONGoOutline(t *testing.T) {
	f := writeGoFile(t, "package sample\n\n// Foo does work.\nfunc Foo() {}\n\ntype Bar struct {\n\tBaz int\n}\n")
	out := decodeOutline(t, captureStdout(t, func() {
		require.NoError(t, runSymbolsJSON(newSymbolsCmd(), []string{f}))
	}))
	assert.Equal(t, "go", out.Language)
	sample := []byte("package sample\n\n// Foo does work.\nfunc Foo() {}\n\ntype Bar struct {\n\tBaz int\n}\n")
	assert.Equal(t, len(sample), out.TotalBytes)
	require.NotEmpty(t, out.Symbols)
	ids := map[string]bool{}
	for _, s := range out.Symbols {
		require.NotEmpty(t, s.ID, "symbol %q has empty ID", s.Name)
		require.False(t, ids[s.ID], "duplicate symbol ID %q", s.ID)
		ids[s.ID] = true
		assert.True(t, s.Targetable)
		assert.GreaterOrEqual(t, s.EndByte, s.StartByte)
		assert.GreaterOrEqual(t, s.EndLine, s.StartLine)
	}
	var foo *srcview.Symbol
	for i := range out.Symbols {
		if out.Symbols[i].Name == "Foo" && out.Symbols[i].Kind == "function" {
			foo = &out.Symbols[i]
		}
	}
	require.NotNil(t, foo, "Foo function symbol missing")
	assert.True(t, foo.HasDoc)
}

func TestSymbolsJSONMarkdownSections(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "doc.md")
	content := []byte("# Title\n\n## Section One\n\nBody.\n\n### Nested\n\nMore.\n")
	require.NoError(t, os.WriteFile(f, content, 0o644))
	out := decodeOutline(t, captureStdout(t, func() {
		require.NoError(t, runSymbolsJSON(newSymbolsCmd(), []string{f}))
	}))
	assert.Equal(t, "markdown", out.Language)
	assert.Equal(t, "Title", out.Title)
	require.NotEmpty(t, out.Symbols)
	nonTargetable := 0
	for _, s := range out.Symbols {
		assert.Equal(t, "section", s.Kind)
		if !s.Targetable {
			nonTargetable++
		}
	}
	// The H1 title heading is not targetable; every other section is.
	assert.Equal(t, 1, nonTargetable)
	for _, s := range out.Symbols {
		if s.Level > 1 {
			assert.True(t, s.Targetable, "section %q must be targetable", s.Name)
		}
	}
}

func TestSymbolsJSONRejectsNoStructureFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(f, []byte("plain text\n"), 0o644))
	err := runSymbolsJSON(newSymbolsCmd(), []string{f})
	require.Error(t, err)
}

func TestReadJSONWholeFile(t *testing.T) {
	f := writeGoFile(t, "package sample\n\nfunc Foo() {}\n")
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(newReadCmd(), []string{f}))
	}))
	assert.Equal(t, f, out.Path)
	assert.Equal(t, "package sample\n\nfunc Foo() {}\n", out.Content)
	assert.Equal(t, 1, out.StartLine)
	assert.Equal(t, 3, out.TotalLines)
	assert.False(t, out.Truncated)
}

func TestReadJSONPlainTextWithoutStructure(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "plain.txt")
	require.NoError(t, os.WriteFile(f, []byte("line one\nline two\n"), 0o644))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(newReadCmd(), []string{f}))
	}))
	assert.Equal(t, "line one\nline two\n", out.Content)
	assert.Equal(t, 2, out.TotalLines)
}

func TestReadJSONEmptyFileIsAZeroLineRead(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "empty.txt")
	require.NoError(t, os.WriteFile(f, nil, 0o644))

	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(newReadCmd(), []string{f}))
	}))
	assert.Equal(t, "", out.Content)
	assert.Zero(t, out.StartLine)
	assert.Zero(t, out.TotalLines)
	assert.Zero(t, out.TotalBytes)
	assert.False(t, out.Truncated)

	lineOne := newReadCmd()
	require.NoError(t, lineOne.Flags().Set("offset", "1"))
	lineOneOut := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(lineOne, []string{f}))
	}))
	assert.Zero(t, lineOneOut.TotalLines)

	beyond := newReadCmd()
	require.NoError(t, beyond.Flags().Set("offset", "2"))
	err := runReadJSON(beyond, []string{f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "beyond end")
}

func TestReadJSONSymbolID(t *testing.T) {
	f := writeGoFile(t, "package sample\n\n// Foo docs.\nfunc Foo() {\n\t// body\n}\n\nfunc Bar() {}\n")
	outline := decodeOutline(t, captureStdout(t, func() {
		require.NoError(t, runSymbolsJSON(newSymbolsCmd(), []string{f}))
	}))
	var fooID string
	for _, s := range outline.Symbols {
		if s.Name == "Foo" {
			fooID = s.ID
		}
	}
	require.NotEmpty(t, fooID)
	cmd := newReadCmd()
	require.NoError(t, cmd.Flags().Set("symbol-id", fooID))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(cmd, []string{f}))
	}))
	assert.Equal(t, fooID, out.SymbolID)
	assert.Contains(t, out.Content, "Foo docs")
	assert.Equal(t, 1, out.StartLine)  // symbol-relative first line
	assert.Equal(t, 4, out.TotalLines) // doc comment + function body
	assert.False(t, out.Truncated)
}

func TestReadJSONSymbolIDRejectsDisplayName(t *testing.T) {
	f := writeGoFile(t, "package sample\n\nfunc Foo() {}\n")
	cmd := newReadCmd()
	require.NoError(t, cmd.Flags().Set("symbol-id", "Foo"))
	err := runReadJSON(cmd, []string{f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestReadJSONExplicitLimitSetsContinuation(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "window.txt")
	lines := make([]string, 0, 10)
	for i := 1; i <= 10; i++ {
		lines = append(lines, "line "+strings.Repeat("x", i))
	}
	require.NoError(t, os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	cmd := newReadCmd()
	require.NoError(t, cmd.Flags().Set("limit", "3"))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(cmd, []string{f}))
	}))
	assert.False(t, out.Truncated)
	assert.Equal(t, 3, out.OutputLines)
	assert.Equal(t, 10, out.TotalLines)
	// The caller-limited window must still advertise the next offset.
	assert.Equal(t, 4, out.NextOffset)
}
func TestReadJSONOffsetLimitPagination(t *testing.T) {
	lines := make([]string, 0, 12)
	for i := 1; i <= 12; i++ {
		lines = append(lines, "line "+strings.Repeat("x", i))
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "paged.txt")
	require.NoError(t, os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0o644))

	cmd := newReadCmd()
	require.NoError(t, cmd.Flags().Set("offset", "3"))
	require.NoError(t, cmd.Flags().Set("limit", "2"))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(cmd, []string{f}))
	}))
	assert.Equal(t, "line xxx\nline xxxx", out.Content)
	assert.Equal(t, 3, out.StartLine)
	assert.Equal(t, 12, out.TotalLines)
	assert.False(t, out.Truncated)
}

func TestReadJSONRejectsOffsetPastLastRealLine(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "two.txt")
	// Two real lines; the trailing newline produces a phantom third split
	// element that is not a line an agent can page to.
	require.NoError(t, os.WriteFile(f, []byte("a\nb\n"), 0o644))

	cmd := newReadCmd()
	require.NoError(t, cmd.Flags().Set("offset", "3"))
	err := runReadJSON(cmd, []string{f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "beyond end")

	// The last real line is still readable.
	cmd2 := newReadCmd()
	require.NoError(t, cmd2.Flags().Set("offset", "2"))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(cmd2, []string{f}))
	}))
	assert.Equal(t, "b\n", out.Content)
	assert.Equal(t, 2, out.TotalLines)
}
func TestReadJSONOffsetBeyondEnd(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "short.txt")
	require.NoError(t, os.WriteFile(f, []byte("one\n"), 0o644))
	cmd := newReadCmd()
	require.NoError(t, cmd.Flags().Set("offset", "99"))
	err := runReadJSON(cmd, []string{f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "beyond end")
}

func TestReadJSONSymbolRelativeOffsetLimit(t *testing.T) {
	f := writeGoFile(t, "package sample\n\nfunc Foo() {\n\t// a\n\t// b\n\t// c\n\t// d\n}\n")
	outline := decodeOutline(t, captureStdout(t, func() {
		require.NoError(t, runSymbolsJSON(newSymbolsCmd(), []string{f}))
	}))
	var fooID string
	for _, s := range outline.Symbols {
		if s.Name == "Foo" {
			fooID = s.ID
		}
	}
	require.NotEmpty(t, fooID)
	cmd := newReadCmd()
	require.NoError(t, cmd.Flags().Set("symbol-id", fooID))
	require.NoError(t, cmd.Flags().Set("offset", "2"))
	require.NoError(t, cmd.Flags().Set("limit", "2"))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(cmd, []string{f}))
	}))
	assert.Equal(t, "\t// a\n\t// b", out.Content)
	assert.Equal(t, 2, out.StartLine) // symbol-relative line 2
}

func TestReadJSONLineTruncationContinuation(t *testing.T) {
	lines := make([]string, 0, 2100)
	for i := 0; i < 2100; i++ {
		lines = append(lines, "line")
	}
	dir := t.TempDir()
	f := filepath.Join(dir, "big.txt")
	require.NoError(t, os.WriteFile(f, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(newReadCmd(), []string{f}))
	}))
	assert.True(t, out.Truncated)
	assert.Equal(t, "lines", out.TruncatedBy)
	assert.Equal(t, 2000, out.OutputLines)
	assert.Equal(t, 2001, out.NextOffset)
	assert.Equal(t, 2100, out.TotalLines)
}

func TestReadJSONFirstLineExceedsLimit(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "huge-line.txt")
	require.NoError(t, os.WriteFile(f, []byte(strings.Repeat("x", 60*1024)+"\n"), 0o644))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(newReadCmd(), []string{f}))
	}))
	assert.True(t, out.FirstLineExceedsLimit)
	assert.True(t, out.Truncated)
	assert.Equal(t, "", out.Content)
}

func TestReadJSONMediaSignatureDetection(t *testing.T) {
	png := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, []byte{0, 0, 0, 13, 'I', 'H', 'D', 'R'}...)
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	gif := []byte("GIF89a")
	webp := append([]byte("RIFF"), append(make([]byte, 4), []byte("WEBP")...)...)
	bmp := make([]byte, 54)
	copy(bmp, "BM")
	bmp[2], bmp[3], bmp[4], bmp[5] = 100, 0, 0, 0    // file size (pixel data follows)
	bmp[10], bmp[11], bmp[12], bmp[13] = 54, 0, 0, 0 // pixel data offset
	bmp[14], bmp[15], bmp[16], bmp[17] = 40, 0, 0, 0 // DIB header size
	bmp[26], bmp[27] = 1, 0                          // color planes
	bmp[28], bmp[29] = 24, 0                         // bits per pixel
	cases := []struct {
		name string
		data []byte
		mime string
	}{
		{"png", png, "image/png"},
		{"jpeg", jpeg, "image/jpeg"},
		{"gif", gif, "image/gif"},
		{"webp", webp, "image/webp"},
		{"bmp", bmp, "image/bmp"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			f := filepath.Join(dir, "image."+tc.name)
			require.NoError(t, os.WriteFile(f, tc.data, 0o644))
			out := decodeRead(t, captureStdout(t, func() {
				require.NoError(t, runReadJSON(newReadCmd(), []string{f}))
			}))
			require.NotNil(t, out.Media, "media missing for %s", tc.name)
			assert.Equal(t, "image", out.Media.Kind)
			assert.Equal(t, tc.mime, out.Media.Mime)
			assert.NotEmpty(t, out.Media.DataBase64)
			// The adapter must never receive decoded UTF-8 text for media.
			assert.Equal(t, "", out.Content)
		})
	}
}

func TestReadJSONRejectsAnimatedPNGVisibly(t *testing.T) {
	// PNG signature + IHDR chunk + acTL chunk before IDAT is an animated PNG.
	pngChunk := func(typ string, data []byte) []byte {
		out := make([]byte, 0, 4+len(typ)+len(data)+4)
		out = append(out, 0, 0, 0, byte(len(data)))
		out = append(out, []byte(typ)...)
		out = append(out, data...)
		out = append(out, 0, 0, 0, 0) // CRC placeholder
		return out
	}
	animated := append([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, pngChunk("IHDR", make([]byte, 13))...)
	animated = append(animated, pngChunk("acTL", make([]byte, 8))...)
	dir := t.TempDir()
	f := filepath.Join(dir, "animated.png")
	require.NoError(t, os.WriteFile(f, animated, 0o644))
	err := runReadJSON(newReadCmd(), []string{f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestReadJSONRejectsInvalidUTF8WithoutNUL(t *testing.T) {
	// 0xFF is invalid UTF-8 but contains no NUL byte; the read must fail visibly
	// instead of emitting JSON replacement characters.
	dir := t.TempDir()
	f := filepath.Join(dir, "broken.txt")
	require.NoError(t, os.WriteFile(f, []byte{'a', 0xFF, 'b', '\n'}, 0o644))
	out := captureStdout(t, func() {
		err := runReadJSON(newReadCmd(), []string{f})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "binary file")
	})
	assert.Equal(t, "", out, "no stdout may carry replacement characters")
}
func TestReadJSONRejectsUnsupportedBinaryVisibly(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "blob.dat")
	require.NoError(t, os.WriteFile(f, []byte("PK\x03\x04binary\x00zip\n"), 0o644))
	err := runReadJSON(newReadCmd(), []string{f})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "binary file")
}

func TestReadJSONMissingFile(t *testing.T) {
	err := runReadJSON(newReadCmd(), []string{"/nonexistent/file.go"})
	require.Error(t, err)
}

func TestReadJSONCRLFAndBOM(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "crlf.go")
	bom := []byte("\xef\xbb\xbf")
	require.NoError(t, os.WriteFile(f, append(bom, []byte("package p\r\n\r\nfunc F() {}\r\n")...), 0o644))
	out := decodeRead(t, captureStdout(t, func() {
		require.NoError(t, runReadJSON(newReadCmd(), []string{f}))
	}))
	assert.Equal(t, "\xef\xbb\xbfpackage p\r\n\r\nfunc F() {}\r\n", out.Content)
	assert.Equal(t, 3, out.TotalLines)
}
