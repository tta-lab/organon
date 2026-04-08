package indent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		source   string
		wantKind Kind
		wantW    int
		wantSrc  string
	}{
		{
			name:     "go file any content",
			filename: "foo.go",
			source:   "package main\n",
			wantKind: Tab,
			wantW:    0,
			wantSrc:  "layer1:go",
		},
		{
			name:     "py file any content",
			filename: "foo.py",
			source:   "def foo():\n  pass\n",
			wantKind: Space,
			wantW:    4,
			wantSrc:  "layer1:python",
		},
		{
			name:     "Makefile basename",
			filename: "Makefile",
			source:   "all:\n\tgo build\n",
			wantKind: Tab,
			wantW:    0,
			wantSrc:  "layer1:makefile",
		},
		{
			name:     "ts file 90 percent tab-indented",
			filename: "foo.ts",
			source:   tenOf(9, "\tfunc foo()") + tenOf(1, "    func bar()"),
			wantKind: Tab,
			wantW:    0,
			wantSrc:  "layer2:majority-tab",
		},
		{
			name:     "ts file 90 percent sp2-indented",
			filename: "foo.ts",
			source:   tenOf(9, "  func foo()") + tenOf(1, "\tfunc bar()"),
			wantKind: Space,
			wantW:    2,
			wantSrc:  "layer2:majority-space-2",
		},
		{
			name:     "ts file 90 percent sp4-indented",
			filename: "foo.ts",
			source:   tenOf(9, "    func foo()") + tenOf(1, "\tfunc bar()"),
			wantKind: Space,
			wantW:    4,
			wantSrc:  "layer2:majority-space-4",
		},
		{
			name:     "ts file with JSDoc mixed with tab code",
			filename: "foo.ts",
			source: "/**\n" +
				" * JSDoc comment\n" +
				" * more lines\n" +
				" */\n" +
				"\tfunction foo() {\n" +
				"\t\treturn 1;\n" +
				"\t}\n",
			wantKind: Tab,
			wantW:    0,
			wantSrc:  "layer2:majority-tab",
		},
		{
			name:     "ts file 50/50 mixed",
			filename: "foo.ts",
			source:   tenOf(5, "\tfunc foo()") + tenOf(5, "  func bar()"),
			wantKind: Unknown,
			wantW:    0,
			wantSrc:  "layer3:fallback",
		},
		{
			name:     "empty ts file",
			filename: "foo.ts",
			source:   "",
			wantKind: Unknown,
			wantW:    0,
			wantSrc:  "layer3:fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Detect(tt.filename, []byte(tt.source))
			assert.Equal(t, tt.wantKind, got.Kind, "Kind mismatch")
			assert.Equal(t, tt.wantW, got.Width, "Width mismatch")
			assert.Equal(t, tt.wantSrc, got.Source, "Source mismatch")
		})
	}
}

func TestDetectByContent(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		wantKind Kind
		wantW    int
		wantSrc  string
	}{
		{
			name:     "content 90 percent tab-indented",
			source:   tenOf(9, "\tfunc foo()") + tenOf(1, "  func bar()"),
			wantKind: Tab,
			wantW:    0,
			wantSrc:  "layer2:majority-tab",
		},
		{
			name:     "content 90 percent sp4-indented",
			source:   tenOf(9, "    func foo()") + tenOf(1, "\tfunc bar()"),
			wantKind: Space,
			wantW:    4,
			wantSrc:  "layer2:majority-space-4",
		},
		{
			name:     "empty content",
			source:   "",
			wantKind: Unknown,
			wantW:    0,
			wantSrc:  "layer3:fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectByContent([]byte(tt.source))
			assert.Equal(t, tt.wantKind, got.Kind, "Kind mismatch")
			assert.Equal(t, tt.wantW, got.Width, "Width mismatch")
			assert.Equal(t, tt.wantSrc, got.Source, "Source mismatch")
		})
	}
}

func TestDetectByContent_IgnoresLayer1(t *testing.T) {
	// Even though filename is .go, DetectByContent should NOT use Layer 1.
	// If the content is sp4-indented, it should report sp4, not tab.
	source := tenOf(9, "    func foo()") + tenOf(1, "      func bar()")
	got := DetectByContent([]byte(source))
	assert.Equal(t, Space, got.Kind)
	assert.Equal(t, 4, got.Width)
	assert.Equal(t, "layer2:majority-space-4", got.Source)
}

// tenOf returns n copies of s, each on its own line.
func tenOf(n int, s string) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s + "\n"
	}
	return result
}
