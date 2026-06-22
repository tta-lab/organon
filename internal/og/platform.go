package og

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	osDarwin = "darwin"
	osLinux  = "linux"
)

func InstallDaemon() (string, error) {
	switch runtime.GOOS {
	case osDarwin:
		return writeLaunchdPlist()
	case osLinux:
		return writeSystemdService()
	default:
		return "", fmt.Errorf("daemon install is unsupported on %s", runtime.GOOS)
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
	if err := runServiceCommand("stop"); err != nil {
		return err
	}
	return runServiceCommand("start")
}

func writeLaunchdPlist() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := launchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>io.guion.og.daemon</string>
<key>ProgramArguments</key><array><string>%s</string><string>daemon</string><string>run</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
</dict></plist>
`, exe)
	return path, os.WriteFile(path, []byte(content), 0644)
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

func runServiceCommand(action string) error {
	switch runtime.GOOS {
	case osDarwin:
		verb := map[string]string{"start": "bootstrap", "stop": "bootout"}[action]
		if verb == "" {
			return errors.New("unsupported launchd action")
		}
		target := "gui/" + strconv.Itoa(os.Getuid())
		args := []string{verb, target, launchdPlistPath()}
		return runCommand("launchctl", args...)
	case osLinux:
		return runCommand("systemctl", "--user", action, "og.service")
	default:
		return fmt.Errorf("daemon %s is unsupported on %s", action, runtime.GOOS)
	}
}

func runCommand(name string, args ...string) error {
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
