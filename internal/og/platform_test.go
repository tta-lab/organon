package og

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRestartDaemonUsesLaunchdKickstart(t *testing.T) {
	var calls [][]string
	withRunCommand(t, func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	})

	if err := restartDaemonForOS(osDarwin); err != nil {
		t.Fatalf("restartDaemonForOS() error = %v", err)
	}

	want := [][]string{
		{"launchctl", "kickstart", "-k", "gui/" + userIDString() + "/io.guion.og.daemon"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}

func TestRestartDaemonReturnsLaunchdKickstartError(t *testing.T) {
	withRunCommand(t, func(name string, args ...string) error {
		return errors.New("launchctl kickstart failed: permission denied")
	})

	err := restartDaemonForOS(osDarwin)
	if err == nil {
		t.Fatal("restartDaemonForOS() error = nil, want error")
	}
}

func TestInstallDaemonBootstrapsLaunchdService(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	var calls [][]string
	withRunCommand(t, func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		if len(calls) == 1 {
			return errors.New("launchctl bootout gui/501 /tmp/og.plist: exit status 5: Boot-out failed: 5: Input/output error")
		}
		return nil
	})

	path, err := installDaemonForOS(osDarwin)
	if err != nil {
		t.Fatalf("installDaemonForOS() error = %v", err)
	}

	wantPath := filepath.Join(home, "Library", "LaunchAgents", "io.guion.og.daemon.plist")
	if path != wantPath {
		t.Fatalf("installDaemonForOS() path = %q, want %q", path, wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("installed plist missing: %v", err)
	}

	want := [][]string{
		{"launchctl", "bootout", "gui/" + userIDString() + "/io.guion.og.daemon"},
		{"launchctl", "bootstrap", "gui/" + userIDString(), wantPath},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}

func TestLaunchdPlistUsesTtalRuntimePattern(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, ".local", "share", "ttal")

	plist := buildLaunchdPlist("io.guion.og.daemon", "/opt/bin/og", dataDir, home)

	for _, want := range []string{
		"<string>io.guion.og.daemon</string>",
		"<string>/opt/bin/og</string>",
		"<string>daemon</string>",
		"<string>run</string>",
		"<key>StandardOutPath</key>",
		"<string>" + dataDir + "/og-daemon.log</string>",
		"<key>StandardErrorPath</key>",
		"<key>EnvironmentVariables</key>",
		"<key>PATH</key>",
		home + "/go/bin",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
	if strings.Contains(plist, "GITHUB_TOKEN") || strings.Contains(plist, "FORGEJO_TOKEN") {
		t.Fatalf("plist should not bake credentials:\n%s", plist)
	}
}

func withRunCommand(t *testing.T, fn func(string, ...string) error) {
	t.Helper()
	old := runCommandFunc
	runCommandFunc = fn
	t.Cleanup(func() {
		runCommandFunc = old
	})
}
