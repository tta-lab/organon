package og

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRestartDaemonIgnoresLaunchdNotLoadedStopError(t *testing.T) {
	var calls [][]string
	withRunCommand(t, func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		if len(calls) == 1 {
			return errors.New("launchctl bootout gui/501 /tmp/og.plist: exit status 5: Boot-out failed: 5: Input/output error")
		}
		return nil
	})

	if err := restartDaemonForOS(osDarwin); err != nil {
		t.Fatalf("restartDaemonForOS() error = %v", err)
	}

	want := [][]string{
		{"launchctl", "bootout", "gui/" + userIDString(), launchdPlistPath()},
		{"launchctl", "bootstrap", "gui/" + userIDString(), launchdPlistPath()},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
	}
}

func TestRestartDaemonReturnsUnexpectedLaunchdStopError(t *testing.T) {
	withRunCommand(t, func(name string, args ...string) error {
		return errors.New("launchctl bootout failed: permission denied")
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
		{"launchctl", "bootout", "gui/" + userIDString(), wantPath},
		{"launchctl", "bootstrap", "gui/" + userIDString(), wantPath},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("launchctl calls = %#v, want %#v", calls, want)
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
