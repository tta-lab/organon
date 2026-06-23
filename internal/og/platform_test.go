package og

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRestartDaemonUsesLaunchdKickstart(t *testing.T) {
	withDaemonHealth(t, func() error {
		return nil
	})
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

func TestStartDaemonKicksAlreadyBootstrappedLaunchdService(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestLaunchdPlist(t)
	withDaemonHealth(t, func() error {
		return nil
	})

	var calls [][]string
	withRunCommand(t, func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		if len(calls) == 1 {
			return errors.New("launchctl bootstrap failed: Bootstrap failed: 5: Input/output error")
		}
		return nil
	})

	if err := startLaunchdDaemon(); err != nil {
		t.Fatalf("startLaunchdDaemon() error = %v", err)
	}

	want := [][]string{
		{"launchctl", "bootstrap", "gui/" + userIDString(), launchdPlistPath()},
		{"launchctl", "kickstart", "-k", "gui/" + userIDString() + "/io.guion.og.daemon"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}

func TestStartDaemonReturnsHealthErrorAfterLaunchdStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestLaunchdPlist(t)
	withDaemonReadyTiming(t, 1, 1)
	withRunCommand(t, func(name string, args ...string) error {
		return nil
	})
	withDaemonHealth(t, func() error {
		return errors.New("dial unix og.sock: connect: no such file or directory")
	})

	err := startLaunchdDaemon()
	if err == nil {
		t.Fatal("startLaunchdDaemon() error = nil, want health error")
	}
	if !strings.Contains(err.Error(), "daemon did not become healthy") {
		t.Fatalf("error = %q, want health context", err.Error())
	}
	if !strings.Contains(err.Error(), "og-daemon.log") {
		t.Fatalf("error = %q, want log path", err.Error())
	}
}

func TestInstallDaemonBootstrapsLaunchdService(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	withDaemonHealth(t, func() error {
		return nil
	})

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

func withDaemonHealth(t *testing.T, fn func() error) {
	t.Helper()
	old := daemonHealthCheckFunc
	daemonHealthCheckFunc = fn
	t.Cleanup(func() {
		daemonHealthCheckFunc = old
	})
}

func withDaemonReadyTiming(t *testing.T, timeoutMs, intervalMs int) {
	t.Helper()
	oldTimeout := daemonReadyTimeout
	oldInterval := daemonReadyInterval
	daemonReadyTimeout = time.Duration(timeoutMs) * time.Millisecond
	daemonReadyInterval = time.Duration(intervalMs) * time.Millisecond
	t.Cleanup(func() {
		daemonReadyTimeout = oldTimeout
		daemonReadyInterval = oldInterval
	})
}

func writeTestLaunchdPlist(t *testing.T) {
	t.Helper()
	path := launchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir plist dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("<plist/>"), 0o600); err != nil {
		t.Fatalf("write plist: %v", err)
	}
}
