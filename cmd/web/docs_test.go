package main

import (
	"os"
	"strings"
	"testing"

	"github.com/tta-lab/organon/internal/docs"
)

func TestNormalizeLibraryID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/effect-ts/effect", "/effect-ts/effect"},
		{"effect-ts/effect", "/effect-ts/effect"},
		{"", ""},
		{"/", "/"},
	}

	for _, tt := range tests {
		got := normalizeLibraryID(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeLibraryID(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestFormatLibraries_OK(t *testing.T) {
	libs := []docs.Library{
		{
			ID:            "/reactjs/react.dev",
			Title:         "React",
			Description:   "A JavaScript library for building user interfaces",
			TrustScore:    10.0,
			TotalSnippets: 2779,
			Versions:      []string{"18.0.0", "17.0.2"},
		},
		{
			ID:            "/facebook/react",
			Title:         "React Native",
			Description:   "A framework for building mobile apps",
			TrustScore:    9.0,
			TotalSnippets: 1000,
			Versions:      nil,
		},
	}

	out := formatLibraries(libs)

	// Check first library fields
	if !strings.Contains(out, "React") {
		t.Error("expected output to contain 'React'")
	}
	if !strings.Contains(out, "/reactjs/react.dev") {
		t.Error("expected output to contain '/reactjs/react.dev'")
	}
	if !strings.Contains(out, "Trust:") {
		t.Error("expected output to contain 'Trust:'")
	}
	if !strings.Contains(out, "Snippets:") {
		t.Error("expected output to contain 'Snippets:'")
	}
	if !strings.Contains(out, "A JavaScript library for building user interfaces") {
		t.Error("expected output to contain first library description")
	}
	if !strings.Contains(out, "Versions:") {
		t.Error("expected output to contain 'Versions:' for library with versions")
	}

	// Check second library fields
	if !strings.Contains(out, "React Native") {
		t.Error("expected output to contain 'React Native'")
	}
	if !strings.Contains(out, "/facebook/react") {
		t.Error("expected output to contain '/facebook/react'")
	}

	// Versions: should appear exactly once (only for first library)
	count := strings.Count(out, "Versions:")
	if count != 1 {
		t.Errorf("expected Versions: to appear once, got %d", count)
	}
}

func TestFormatLibraries_Empty(t *testing.T) {
	out := formatLibraries([]docs.Library{})
	if !strings.Contains(out, "Found 0 libraries:") {
		t.Errorf("expected 'Found 0 libraries:', got: %s", out)
	}
}

func TestNewDocsClient_EmptyKeySet(t *testing.T) {
	t.Setenv("CONTEXT7_API_KEY", "")
	_, err := newDocsClient()
	if err == nil {
		t.Error("expected error for empty CONTEXT7_API_KEY")
	}
	if !strings.Contains(err.Error(), "set but empty") {
		t.Errorf("expected 'set but empty' error, got: %s", err.Error())
	}
}

func TestNewDocsClient_KeyUnset(t *testing.T) {
	_ = os.Unsetenv("CONTEXT7_API_KEY")
	_, err := newDocsClient()
	if err != nil {
		t.Errorf("expected no error for unset CONTEXT7_API_KEY, got: %v", err)
	}
}

func TestNewDocsClient_KeyPresent(t *testing.T) {
	t.Setenv("CONTEXT7_API_KEY", "ctx7sk-abc")
	_, err := newDocsClient()
	if err != nil {
		t.Errorf("expected no error for set CONTEXT7_API_KEY, got: %v", err)
	}
}

func TestDocsResolve_RejectsZeroArgs(t *testing.T) {
	cmd := newDocsResolveCmd()
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error for zero args")
	}
}

func TestDocsResolve_RejectsTwoArgs(t *testing.T) {
	cmd := newDocsResolveCmd()
	err := cmd.Args(cmd, []string{"arg1", "arg2"})
	if err == nil {
		t.Error("expected error for two args")
	}
}

func TestDocsFetch_RejectsZeroArgs(t *testing.T) {
	cmd := newDocsFetchCmd()
	err := cmd.Args(cmd, []string{})
	if err == nil {
		t.Error("expected error for zero args")
	}
}

func TestDocsFetch_RejectsThreeArgs(t *testing.T) {
	cmd := newDocsFetchCmd()
	err := cmd.Args(cmd, []string{"id", "topic", "extra"})
	if err == nil {
		t.Error("expected error for three args")
	}
}
