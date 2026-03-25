package diff

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Tool priority: delta > diff-so-fancy > colordiff > diff
var (
	toolOnce sync.Once
	toolName string
	toolPath string
)

// detectTool finds the best available diff tool.
// Called once via sync.Once; results cached in toolName/toolPath.
func detectTool() {
	for _, name := range []string{"delta", "diff-so-fancy", "colordiff", "diff"} {
		if p, err := exec.LookPath(name); err == nil {
			toolName = name
			toolPath = p
			return
		}
	}
	// No tool found — Show will be a no-op
}

// Show writes a colored diff between old and new content to w.
// filename is used for temp file extensions (enables syntax highlighting in delta).
// Returns nil without output if old and new are identical, or if no diff tool is available.
func Show(w io.Writer, old, new []byte, filename string) error {
	if bytes.Equal(old, new) {
		return nil
	}

	toolOnce.Do(detectTool)

	if toolName == "" {
		return nil
	}

	ext := filepath.Ext(filename)

	dir, err := os.MkdirTemp("", "src-diff-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	oldFile := filepath.Join(dir, "before"+ext)
	newFile := filepath.Join(dir, "after"+ext)

	if err := os.WriteFile(oldFile, old, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(newFile, new, 0o644); err != nil {
		return err
	}

	switch toolName {
	case "delta":
		return runDelta(w, oldFile, newFile)
	case "diff-so-fancy":
		return runDiffSoFancy(w, oldFile, newFile)
	case "colordiff":
		return runColorDiff(w, oldFile, newFile)
	default:
		return runPlainDiff(w, oldFile, newFile)
	}
}

// runDelta runs: delta --paging=never before after
func runDelta(w io.Writer, oldFile, newFile string) error {
	cmd := exec.Command(toolPath, "--paging=never", oldFile, newFile)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runDiffSoFancy runs: diff -u before after | diff-so-fancy
func runDiffSoFancy(w io.Writer, oldFile, newFile string) error {
	diffCmd := exec.Command("diff", "-u", oldFile, newFile)
	fancyCmd := exec.Command(toolPath)

	pipe, err := diffCmd.StdoutPipe()
	if err != nil {
		return err
	}
	fancyCmd.Stdin = pipe
	fancyCmd.Stdout = w
	fancyCmd.Stderr = os.Stderr

	if err := fancyCmd.Start(); err != nil {
		return err
	}
	// diff returns exit 1 when files differ — not an error
	if err := diffCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			return err
		}
	}
	return fancyCmd.Wait()
}

// runColorDiff runs: colordiff -u before after
func runColorDiff(w io.Writer, oldFile, newFile string) error {
	cmd := exec.Command(toolPath, "-u", oldFile, newFile)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	// colordiff (like diff) returns exit 1 when files differ
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return nil
	}
	return err
}

// runPlainDiff runs: diff -u before after
func runPlainDiff(w io.Writer, oldFile, newFile string) error {
	cmd := exec.Command(toolPath, "-u", oldFile, newFile)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	// diff returns exit 1 when files differ
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return nil
	}
	return err
}
