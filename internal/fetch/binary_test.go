package fetch

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBinaryContentType(t *testing.T) {
	tests := []struct {
		name     string
		ctype    string
		expected bool
	}{
		{name: "text/html", ctype: "text/html", expected: false},
		{name: "text/plain", ctype: "text/plain", expected: false},
		{name: "empty", ctype: "", expected: false},
		{name: "json", ctype: "application/json", expected: false},
		{name: "javascript", ctype: "application/javascript", expected: false},
		{name: "img png", ctype: "image/png", expected: true},
		{name: "img jpg", ctype: "image/jpeg", expected: true},
		{name: "img svg", ctype: "image/svg+xml", expected: true},
		{name: "audio", ctype: "audio/mpeg", expected: true},
		{name: "video", ctype: "video/mp4", expected: true},
		{name: "font", ctype: "font/woff2", expected: true},
		{name: "octet-stream", ctype: "application/octet-stream", expected: true},
		{name: "pdf", ctype: "application/pdf", expected: true},
		{name: "zip", ctype: "application/zip", expected: true},
		{name: "gzip", ctype: "application/gzip", expected: true},
		{name: "x-gzip", ctype: "application/x-gzip", expected: true},
		{name: "tar", ctype: "application/x-tar", expected: true},
		{name: "ms excel", ctype: "application/vnd.ms-excel", expected: true},
		{name: "ms word", ctype: "application/msword", expected: true},
		{name: "openxml", ctype: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", expected: true},
		{name: "exe", ctype: "application/x-msdownload", expected: true},
		{name: "elf", ctype: "application/x-executable", expected: true},
		{name: "mach-o", ctype: "application/x-mach-binary", expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsBinaryContentType(tt.ctype))
		})
	}
}

func TestIsBinaryBody(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{name: "empty", data: []byte{}, expected: false},
		{name: "text", data: []byte("hello world"), expected: false},
		{name: "json body", data: []byte(`{"key":"value"}`), expected: false},
		{name: "null at byte 0", data: []byte{0, 'h', 'e', 'l', 'l', 'o'}, expected: true},
		{name: "null mid body", data: []byte("hello\x00world"), expected: true},
		{name: "null at byte 8191", data: makeBodyWithNull(8191), expected: true},
		{name: "null at byte 8192", data: makeBodyWithNull(8192), expected: false},
		{name: "null at byte 8193", data: makeBodyWithNull(8193), expected: false},
		{name: "under 8KB no null", data: makeBodyNoNull(4096), expected: false},
		{name: "under 8KB with null", data: makeBodyWithNull(4096), expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsBinaryBody(tt.data))
		})
	}
}

func TestBinaryFetchError(t *testing.T) {
	err := BinaryFetchError("https://example.com/video.mp4", "video/mp4")
	assert.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "binary content at https://example.com/video.mp4")
	assert.Contains(t, msg, "Content-Type: video/mp4")
	assert.Contains(t, msg, "web fetch only handles text")
	// Must suggest at least one tool
	hasAria := strings.Contains(msg, "aria2c")
	hasWget := strings.Contains(msg, "wget")
	hasCurl := strings.Contains(msg, "curl")
	assert.True(t, hasAria || hasWget || hasCurl, "error should suggest a download tool")
}

func TestBinaryFetchError_NoContentType(t *testing.T) {
	err := BinaryFetchError("https://example.com/file", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Content-Type: (none)")
}

func TestToolExists(t *testing.T) {
	// Tools that should always exist in a reasonable environment
	assert.True(t, toolExists("sh"))
	assert.True(t, toolExists("true"))

	// Tool that almost certainly doesn't exist
	assert.False(t, toolExists("definitely-not-a-real-tool-xyzzy"))
}

func makeBodyWithNull(nullPos int) []byte {
	buf := bytesRepeat('x', 16384) // 16KB — larger than 8KB scan window
	if nullPos < len(buf) {
		buf[nullPos] = 0
	}
	return buf
}

func makeBodyNoNull(size int) []byte {
	return bytesRepeat('x', size)
}

func bytesRepeat(b byte, count int) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = b
	}
	return out
}
