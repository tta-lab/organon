package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/githubapp"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/ogconfig"
	"github.com/tta-lab/organon/internal/project"
)

const cmdStatus = "status"

func runDaemonRun(cmd *cobra.Command, args []string) error {
	if err := config.InjectDotEnvFallback(); err != nil {
		cmd.PrintErrf("warning: could not load .env: %v\n", err)
	}
	service, err := loadDaemonService(config.OGConfigPath(), config.DefaultConfigDir())
	if err != nil {
		return err
	}
	socketPath := og.SocketPath()
	err = og.ListenAndServeUnixReady(socketPath, og.NewMux(service), func() {
		cmd.Printf("og daemon listening on unix://%s\n", socketPath)
	})
	if err != nil {
		return fmt.Errorf("serve daemon unix://%s: %w", socketPath, err)
	}
	return nil
}

func loadDaemonService(configPath, configDir string) (og.Service, error) {
	cfg, err := ogconfig.Load(configPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg = ogconfig.Config{}
	} else if err != nil {
		return og.Service{}, err
	}
	var broker githubapp.CredentialBroker
	if cfg.GitHubApp != nil {
		keySource, err := githubapp.NewKeySource(*cfg.GitHubApp, configDir)
		if err != nil {
			return og.Service{}, err
		}
		broker, err = githubapp.NewBroker(*cfg.GitHubApp, keySource)
		if err != nil {
			return og.Service{}, err
		}
	}
	return og.NewServiceWithConfig(broker, project.NewStore(config.ProjectsPath()), cfg), nil
}

func runDaemonValidate(cmd *cobra.Command, args []string) error {
	service, err := loadDaemonService(config.OGConfigPath(), config.DefaultConfigDir())
	if err != nil {
		return err
	}
	if err := service.Validate(); err != nil {
		return err
	}
	cmd.Println("ok")
	return nil
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
