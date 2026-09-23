package srcview

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSearchTruncationBoundary(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one.txt")
	for _, tc := range []struct {
		contents  string
		count     int
		truncated bool
	}{
		{"missing\n", 0, false},
		{"needle\n", 1, false},
		{"needle\nneedle\n", 2, false},
		{"needle\nneedle\nneedle\n", 2, true},
	} {
		if err := os.WriteFile(path, []byte(tc.contents), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := Search(context.Background(), root, "needle", 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Matches) != tc.count || result.Truncated != tc.truncated {
			t.Fatalf("contents %q: result = %+v", tc.contents, result)
		}
	}
}

func TestSearchReportsProcessParseAndTimeoutFailures(t *testing.T) {
	for _, tc := range []struct {
		name, script string
		timeout      time.Duration
	}{
		{"process", "#!/bin/sh\nexit 2\n", time.Second},
		{"parse", "#!/bin/sh\nprintf 'not-json\\n'\n", time.Second},
		{"timeout", "#!/bin/sh\nwhile :; do :; done\n", 50 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "rg"), []byte(tc.script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			ctx, cancel := context.WithTimeout(context.Background(), tc.timeout)
			defer cancel()
			if _, err := Search(ctx, t.TempDir(), "needle", 2); err == nil {
				t.Fatal("expected visible search error")
			}
		})
	}
}

func TestSearchStopsAtLimitPlusOne(t *testing.T) {
	bin := t.TempDir()
	script := `#!/bin/sh
printf '%s\n' '{"type":"match","data":{"path":{"text":"a"},"lines":{"text":"needle"},"line_number":1}}'
printf '%s\n' '{"type":"match","data":{"path":{"text":"a"},"lines":{"text":"needle"},"line_number":2}}'
printf '%s\n' '{"type":"match","data":{"path":{"text":"a"},"lines":{"text":"needle"},"line_number":3}}'
while :; do :; done
`
	if err := os.WriteFile(filepath.Join(bin, "rg"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := Search(ctx, t.TempDir(), "needle", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 || !result.Truncated {
		t.Fatalf("result = %+v", result)
	}
}

func TestSearchReportsCancellationAndScannerErrors(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, "rg"), []byte("#!/bin/sh\nwhile :; do :; done\n"), 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(30*time.Millisecond, cancel)
		defer cancel()
		if _, err := Search(ctx, t.TempDir(), "needle", 2); err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	})
	t.Run("scanner", func(t *testing.T) {
		bin := t.TempDir()
		payload := filepath.Join(t.TempDir(), "oversize")
		if err := os.WriteFile(payload, bytes.Repeat([]byte("x"), 1024*1024+1), 0600); err != nil {
			t.Fatal(err)
		}
		script := []byte("#!/bin/sh\nexec /bin/cat \"$FAKE_RG_OUTPUT\"\n")
		if err := os.WriteFile(filepath.Join(bin, "rg"), script, 0700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin)
		t.Setenv("FAKE_RG_OUTPUT", payload)
		_, err := Search(context.Background(), t.TempDir(), "needle", 2)
		if err == nil || !strings.Contains(err.Error(), "rg output") {
			t.Fatalf("scanner error = %v", err)
		}
	})
}
