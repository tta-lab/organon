package og

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

func TestServiceValidateClassifiesEveryRegisteredRemote(t *testing.T) {
	registry := filepath.Join(t.TempDir(), "projects.toml")
	if err := os.WriteFile(registry, []byte(
		"[test]\npath = \"/work/test\"\nremote = \"http://forgejo.localhost:17480/owner/repo.git\"\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewServiceWithConfig(nil, project.NewStore(registry), ogconfig.Config{})
	if err := service.Validate(); err == nil || !strings.Contains(err.Error(), "is not allowed") {
		t.Fatalf("Validate error = %v, want unlisted HTTP remote rejection", err)
	}
}
