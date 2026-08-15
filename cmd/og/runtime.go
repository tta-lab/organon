package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/og"
	"github.com/tta-lab/organon/internal/project"
)

type commandRuntime struct {
	executor og.Executor
	projects *project.Store
}

type commandRuntimeKey struct{}

func withCommandRuntime(ctx context.Context, runtime commandRuntime) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, commandRuntimeKey{}, runtime)
}

func runtimeFor(cmd *cobra.Command) (commandRuntime, error) {
	if runtime, ok := cmd.Context().Value(commandRuntimeKey{}).(commandRuntime); ok {
		if runtime.executor == nil || runtime.projects == nil {
			return commandRuntime{}, fmt.Errorf("OG runtime is incomplete")
		}
		return runtime, nil
	}
	if err := config.InjectDotEnvFallback(); err != nil {
		cmd.PrintErrf("warning: could not load .env: %v\n", err)
	}
	service, err := og.LoadService(config.OGConfigPath(), config.DefaultConfigDir())
	if err != nil {
		return commandRuntime{}, err
	}
	runtime := commandRuntime{executor: service, projects: service.ProjectStore()}
	cmd.SetContext(withCommandRuntime(cmd.Context(), runtime))
	return runtime, nil
}

func requestFor(cmd *cobra.Command, req og.Request) og.Request {
	req.Context = cmd.Context()
	return req
}

func resolveOGProject(store *project.Store, reference string) (project.Entry, error) {
	entry, err := store.Resolve(reference)
	if err != nil {
		return project.Entry{}, fmt.Errorf("resolve project: %w", err)
	}
	return entry, nil
}

func normalizeOGProjectRequest(store *project.Store, req *og.Request) (string, error) {
	if req.Project == "" {
		return "", nil
	}
	entry, err := resolveOGProject(store, req.Project)
	if err != nil {
		return "", err
	}
	req.Project = entry.Alias
	return entry.Alias, nil
}

func resolveWorkDir(cmd *cobra.Command, runtime commandRuntime) (workDir, alias string, err error) {
	reference, _ := cmd.Flags().GetString("project")
	if reference == "" {
		if cmd.Flags().Changed("project") {
			return "", "", fmt.Errorf("project reference must not be empty")
		}
		workDir, err = os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("get working directory: %w", err)
		}
		return workDir, "", nil
	}
	entry, err := resolveOGProject(runtime.projects, reference)
	if err != nil {
		return "", "", err
	}
	return entry.Path, entry.Alias, nil
}
