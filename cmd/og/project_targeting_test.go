package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/og"
)

func TestExplicitEmptyProjectDoesNotFallBackToWorkingDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	called := false
	ogDaemon(t, func(path string, req og.Request) og.Response {
		called = true
		return og.Response{Auth: &og.AuthStatus{
			Provider: "github", Host: "github.com", Owner: "tta-lab", Repo: "organon",
			AuthMode: "token", Ready: true,
		}}
	})

	_, err := runOGJSON(t, "auth", "status", "--project", "", "--json")
	if err == nil || !strings.Contains(err.Error(), "project alias must not be empty") {
		t.Fatalf("error = %v, want explicit empty project rejection", err)
	}
	if called {
		t.Fatal("daemon called after explicit empty project alias")
	}
}

func TestOmittedProjectRetainsHumanWorkingDirectoryMode(t *testing.T) {
	workDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	currentWorkDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })

	var gotWorkDir string
	ogDaemon(t, func(path string, req og.Request) og.Response {
		gotWorkDir = req.WorkDir
		return og.Response{Auth: &og.AuthStatus{
			Provider: "github", Host: "github.com", Owner: "tta-lab", Repo: "organon",
			AuthMode: "token", Ready: true,
		}}
	})
	cmd := newRootCmd(io.Discard, io.Discard)
	cmd.SetArgs([]string{"auth", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth status without --project: %v", err)
	}
	if gotWorkDir != currentWorkDir {
		t.Fatalf("daemon work dir = %q, want current working directory %q", gotWorkDir, currentWorkDir)
	}
}

func TestTagProjectUsesRegisteredCheckout(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ogWriteProjects(t, home)
	var gotPath string
	var gotRequest og.Request
	ogDaemon(t, func(path string, req og.Request) og.Response {
		gotPath, gotRequest = path, req
		return og.Response{Message: "tagged"}
	})

	_, err := runOGJSON(t, "tag", "v1.2.3", "--project", "ko")
	if err != nil {
		t.Fatalf("tag registered project: %v", err)
	}
	if gotPath != "/git/tag" || gotRequest.WorkDir != "/work/ko" || gotRequest.Tag != "v1.2.3" {
		t.Fatalf("tag daemon request = %s %+v", gotPath, gotRequest)
	}
}
