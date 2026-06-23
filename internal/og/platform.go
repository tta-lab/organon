package og

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/tta-lab/organon/internal/config"
)

const (
	osDarwin = "darwin"
	osLinux  = "linux"

	launchdLabel = "io.guion.og.daemon"
)

func InstallDaemon() (string, error) {
	return installDaemonForOS(runtime.GOOS)
}

func installDaemonForOS(goos string) (string, error) {
	switch goos {
	case osDarwin:
		return installLaunchdDaemon()
	case osLinux:
		return writeSystemdService()
	default:
		return "", fmt.Errorf("daemon install is unsupported on %s", goos)
	}
}

func UninstallDaemon() error {
	switch runtime.GOOS {
	case osDarwin:
		return os.Remove(launchdPlistPath())
	case osLinux:
		return os.Remove(systemdServicePath())
	default:
		return fmt.Errorf("daemon uninstall is unsupported on %s", runtime.GOOS)
	}
}

func StartDaemon() error {
	return runServiceCommand("start")
}

func StopDaemon() error {
	return runServiceCommand("stop")
}

func RestartDaemon() error {
	return restartDaemonForOS(runtime.GOOS)
}

func restartDaemonForOS(goos string) error {
	if goos == osDarwin {
		if err := runCommand("launchctl", "kickstart", "-k", launchdServiceTarget()); err != nil {
			return err
		}
		return waitForDaemonReady(goos)
	}
	if err := runServiceCommandForOS(goos, "stop"); err != nil {
		return err
	}
	if err := runServiceCommandForOS(goos, "start"); err != nil {
		return err
	}
	return waitForDaemonReady(goos)
}

func isLaunchdNotLoadedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Boot-out failed: 5") ||
		strings.Contains(msg, "No such process") ||
		strings.Contains(msg, "Could not find service") ||
		strings.Contains(msg, "service is not loaded")
}

func userIDString() string {
	return strconv.Itoa(os.Getuid())
}

func runServiceCommand(action string) error {
	return runServiceCommandForOS(runtime.GOOS, action)
}

func runServiceCommandForOS(goos, action string) error {
	switch goos {
	case osDarwin:
		switch action {
		case "start":
			return startLaunchdDaemon()
		case "stop":
			return stopLaunchdDaemon()
		default:
			return errors.New("unsupported launchd action")
		}
	case osLinux:
		return runCommand("systemctl", "--user", action, "og.service")
	default:
		return fmt.Errorf("daemon %s is unsupported on %s", action, goos)
	}
}

var (
	runCommandFunc        = runCommandImpl
	daemonHealthCheckFunc = daemonHealthCheck
	daemonReadyTimeout    = 5 * time.Second
	daemonReadyInterval   = 100 * time.Millisecond
)

func runCommand(name string, args ...string) error {
	return runCommandFunc(name, args...)
}

func runCommandImpl(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "io.guion.og.daemon.plist")
}

func systemdServicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "og.service")
}

func installLaunchdDaemon() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dataDir := config.ResolveDataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}

	_ = stopLaunchdDaemon()

	path := launchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	exe, err := daemonExecutable()
	if err != nil {
		return "", err
	}
	content := buildLaunchdPlist(launchdLabel, exe, dataDir, home)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	if err := startLaunchdDaemon(); err != nil {
		return "", err
	}
	return path, nil
}

func daemonExecutable() (string, error) {
	if exe, err := exec.LookPath("og"); err == nil {
		return exe, nil
	}
	return os.Executable()
}

func startLaunchdDaemon() error {
	path := launchdPlistPath()
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("daemon not installed (run: og daemon install)")
	}
	err := runCommand("launchctl", "bootstrap", "gui/"+userIDString(), path)
	if err != nil && isLaunchdAlreadyBootstrappedError(err) {
		err = runCommand("launchctl", "kickstart", "-k", launchdServiceTarget())
	}
	if err != nil {
		return err
	}
	return waitForDaemonReady(osDarwin)
}

func stopLaunchdDaemon() error {
	err := runCommand("launchctl", "bootout", launchdServiceTarget())
	if err != nil && isLaunchdNotLoadedError(err) {
		return nil
	}
	return err
}

func launchdServiceTarget() string {
	return "gui/" + userIDString() + "/" + launchdLabel
}

func isLaunchdAlreadyBootstrappedError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already bootstrapped") ||
		strings.Contains(msg, "Bootstrap failed: 5") ||
		strings.Contains(msg, "36:")
}

func buildLaunchdPlist(label, exe, dataDir, home string) string {
	daemonPATH := "/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin:" +
		home + "/.local/bin:" + home + "/go/bin:" +
		home + "/.cargo/bin"
	logPath := filepath.Join(dataDir, "og-daemon.log")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>

    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>daemon</string>
        <string>run</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>%s</string>

    <key>StandardErrorPath</key>
    <string>%s</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>%s</string>
    </dict>
</dict>
</plist>
`, label, exe, logPath, logPath, daemonPATH)
}

func waitForDaemonReady(goos string) error {
	deadline := time.Now().Add(daemonReadyTimeout)
	var lastErr error
	for {
		if err := daemonHealthCheckFunc(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(daemonReadyInterval)
	}
	if goos == osDarwin {
		return fmt.Errorf("daemon did not become healthy after launch; check %s: %w", launchdLogPath(), lastErr)
	}
	return fmt.Errorf("daemon did not become healthy after launch: %w", lastErr)
}

func daemonHealthCheck() error {
	resp, err := NewClientFromEnv().Health()
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %s", resp.Status)
	}
	return nil
}

func launchdLogPath() string {
	return filepath.Join(config.ResolveDataDir(), "og-daemon.log")
}

func writeSystemdService() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := systemdServicePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	content := fmt.Sprintf(`[Unit]
Description=og daemon

[Service]
ExecStart=%s daemon run
Restart=always

[Install]
WantedBy=default.target
`, exe)
	return path, os.WriteFile(path, []byte(content), 0644)
}
