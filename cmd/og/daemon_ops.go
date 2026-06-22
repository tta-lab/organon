package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/og"
)

const cmdStatus = "status"

func runDaemonRun(cmd *cobra.Command, args []string) error {
	socketPath := og.SocketPath()
	cmd.Printf("og daemon listening on unix://%s\n", socketPath)
	return og.ListenAndServeUnix(socketPath, og.NewMux(og.Service{}))
}

func runDaemonInstall(cmd *cobra.Command, args []string) error {
	path, err := og.InstallDaemon()
	if err != nil {
		return err
	}
	switch {
	case strings.Contains(path, "LaunchAgents"):
		cmd.Printf("Installed launchd plist: %s\n", path)
	default:
		cmd.Printf("Installed systemd user service: %s\n", path)
	}
	return nil
}

func runDaemonUninstall(cmd *cobra.Command, args []string) error {
	return og.UninstallDaemon()
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	return og.StartDaemon()
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	return og.StopDaemon()
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	return og.RestartDaemon()
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	resp, err := og.NewClientFromEnv().Health()
	if err != nil {
		cmd.Println("Daemon: not running")
		return nil
	}
	if resp.StatusCode == http.StatusOK {
		cmd.Println("Daemon: running")
		return nil
	}
	cmd.Printf("Daemon: unhealthy (%s)\n", resp.Status)
	return nil
}

func runDaemonHealth(cmd *cobra.Command, args []string) error {
	resp, err := og.NewClientFromEnv().Health()
	if err != nil {
		return fmt.Errorf("daemon health: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon health: %s", resp.Status)
	}
	cmd.Println("ok")
	return nil
}
