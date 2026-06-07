package fetch

import (
	"fmt"
	"os/exec"
	"strings"
)

// IsBinaryContentType returns true for content types that are never human-readable text.
func IsBinaryContentType(mediatype string) bool {
	binaryTypes := []string{
		"application/octet-stream",
		"application/zip",
		"application/gzip",
		"application/x-gzip",
		"application/x-tar",
		"application/pdf",
		"application/msword",
		"application/vnd.ms-",
		"application/vnd.openxmlformats",
		"image/",
		"audio/",
		"video/",
		"font/",
		"application/x-msdownload",
		"application/x-executable",
		"application/x-mach-binary",
	}
	for _, bt := range binaryTypes {
		if strings.HasPrefix(mediatype, bt) {
			return true
		}
	}
	return false
}

// IsBinaryBody checks the first 8KB for null bytes.
func IsBinaryBody(data []byte) bool {
	max := 8192
	if len(data) < max {
		max = len(data)
	}
	for _, b := range data[:max] {
		if b == 0 {
			return true
		}
	}
	return false
}

// BinaryFetchError returns an error telling the agent to use a download tool.
// Suggests aria2c if available, then wget, then curl.
func BinaryFetchError(url, contentType string) error {
	ct := contentType
	if ct == "" {
		ct = "(none)"
	}

	var tool string
	var example string
	switch {
	case toolExists("aria2c"):
		tool = "aria2c"
		example = fmt.Sprintf("  aria2c -x4 %s", url)
	case toolExists("wget"):
		tool = "wget"
		example = fmt.Sprintf("  wget %s", url)
	case toolExists("curl"):
		tool = "curl"
		example = fmt.Sprintf("  curl -L -O %s", url)
	default:
		tool = "curl"
		example = fmt.Sprintf("  curl -L -O %s", url)
	}

	return fmt.Errorf(
		"binary content at %s (Content-Type: %s)\n\n"+
			"web fetch only handles text. Use %s to download:\n%s",
		url, ct, tool, example)
}

func toolExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
