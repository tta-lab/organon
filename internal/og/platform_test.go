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

func TestStartDaemonKickstartsAfterBootstrap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	writeTestLaunchdPlist(t)
	withDaemonHealth(t, func() error {
		return nil
	})

	var calls [][]string
	withRunCommand(t, func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
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
	writeTestLaunchdLog(t, "first line\nfatal: bad startup\n")
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
	if !strings.Contains(err.Error(), "fatal: bad startup") {
		t.Fatalf("error = %q, want log tail", err.Error())
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
		{"launchctl", "kickstart", "-k", "gui/" + userIDString() + "/io.guion.og.daemon"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}

func TestInstallDaemonRemovesStaleSocketAfterBootout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	socketPath := SocketPath()
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatalf("mkdir socket dir: %v", err)
	}
	if err := os.WriteFile(socketPath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale socket: %v", err)
	}
	withDaemonHealth(t, func() error {
		if _, err := os.Stat(socketPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale socket still exists: %v", err)
		}
		return nil
	})
	withRunCommand(t, func(name string, args ...string) error {
		return nil
	})

	if _, err := installDaemonForOS(osDarwin); err != nil {
		t.Fatalf("installDaemonForOS() error = %v", err)
	}
}

func TestLaunchdPlistUsesTtalRuntimePattern(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, ".local", "share", "ttal")
	t.Setenv("HTTPS_PROXY", "http://proxy.example:7890/path?x=1&y=2")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")
	t.Setenv("GITHUB_TOKEN", "do-not-write-this")

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
		"<key>HTTPS_PROXY</key>",
		"<string>http://proxy.example:7890/path?x=1&amp;y=2</string>",
		"<key>NO_PROXY</key>",
		"<string>localhost,127.0.0.1</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
	for _, forbidden := range []string{"GITHUB_TOKEN", "FORGEJO_TOKEN", "do-not-write-this"} {
		if strings.Contains(plist, forbidden) {
			t.Fatalf("plist should not bake %q:\n%s", forbidden, plist)
		}
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

func writeTestLaunchdLog(t *testing.T, content string) {
	t.Helper()
	path := launchdLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write log: %v", err)
	}
}
