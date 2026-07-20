package navidrome

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadRadioSpec(t *testing.T) {
	spec, err := ReadRadioSpec(strings.NewReader(`stations:
  - name: Synthwave
    stream_url: https://radio.example/synthwave
    homepage_url: https://example.com
`))
	if err != nil {
		t.Fatalf("ReadRadioSpec: %v", err)
	}
	if len(spec.Stations) != 1 || spec.Stations[0].Name != "Synthwave" {
		t.Fatalf("spec = %+v", spec)
	}

	var out bytes.Buffer
	if err := WriteRadioSpec(&out, spec); err != nil {
		t.Fatalf("WriteRadioSpec: %v", err)
	}
	if !strings.Contains(out.String(), "stream_url: https://radio.example/synthwave") {
		t.Fatalf("output = %q", out.String())
	}
}
