package main

import (
	"bytes"
	"strings"
	"testing"
)

func runOG(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()

	var outBuf, errBuf bytes.Buffer
	cmd := newRootCmd(&outBuf, &errBuf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return outBuf.String(), err
}

func TestRootHelpListsV1CommandGroups(t *testing.T) {
	stdout, err := runOG(t, "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"pr",
		"git",
		"auth",
		"policy",
		"daemon",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("root help missing %q:\n%s", want, stdout)
		}
	}
}

func TestPRHelpListsV1CommandsWithoutMerge(t *testing.T) {
	stdout, err := runOG(t, "pr", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"create",
		"view",
		"modify",
		"comment",
		"checks",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("pr help missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "merge") {
		t.Fatalf("pr help should not list merge:\n%s", stdout)
	}
}

func TestPRMergeIsNotAvailableInV1(t *testing.T) {
	_, err := runOG(t, "pr", "merge")
	if err == nil {
		t.Fatal("expected pr merge to fail")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %v, want unknown command", err)
	}
}

func TestGitHelpListsTtalReplacementCommands(t *testing.T) {
	stdout, err := runOG(t, "git", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, want := range []string{
		"push",
		"pull",
		"tag",
		"--force",
		"--bump",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("git help missing %q:\n%s", want, stdout)
		}
	}
}

func TestGitStubsAcceptTtalReplacementShapes(t *testing.T) {
	tests := [][]string{
		{"git", "push", "--force"},
		{"git", "pull"},
		{"git", "tag", "v1.2.3"},
		{"git", "tag", "--bump", "patch"},
	}

	for _, args := range tests {
		_, err := runOG(t, args...)
		if err == nil {
			t.Fatalf("runOG(%v) expected not implemented error", args)
		}
		if !strings.Contains(err.Error(), "not implemented yet") {
			t.Fatalf("runOG(%v) error = %v, want not implemented", args, err)
		}
	}
}
