package diff

import (
	"bytes"
	"fmt"
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
	// No tool found — Show will emit a warning and return nil.
}

// Show writes a colored diff between old and new content to w.
// filename is used for temp file extensions (enables syntax highlighting in delta).
// Returns nil without output if old and new are identical.
// If no diff tool is available, a one-time warning is printed to stderr.
func Show(w io.Writer, old, new []byte, filename string) error {
	if bytes.Equal(old, new) {
		return nil
	}

	toolOnce.Do(detectTool)

	if toolName == "" {
		fmt.Fprintln(os.Stderr, "diff: no diff tool found; install delta, diff-so-fancy, colordiff, or diff")
		return nil
	}

	ext := filepath.Ext(filename)

	dir, err := os.MkdirTemp("", "src-diff-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

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
		// delta follows the POSIX diff exit-code contract when invoked directly
		// as `delta file1 file2`: exit 0 = identical, exit 1 = differ, exit 2 = error.
		// Verified against delta v0.16–v0.18; behavior is inherited from its internal diff call.
		return runSimpleDiff(w, toolPath, "--paging=never", oldFile, newFile)
	case "diff-so-fancy":
		return runDiffSoFancy(w, oldFile, newFile)
	default:
		// colordiff and plain diff both use `diff -u` invocation and exit-code semantics.
		return runSimpleDiff(w, toolPath, "-u", oldFile, newFile)
	}
}

// runSimpleDiff runs a diff command that accepts (flags..., oldFile, newFile) and
// suppresses exit code 1 (files differ), which is the POSIX diff convention.
// Used for delta, colordiff, and plain diff.
func runSimpleDiff(w io.Writer, path string, args ...string) error {
	cmd := exec.Command(path, args...)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return nil
	}
	return err
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
		_ = pipe.Close() // release read-end; write-end owned by diffCmd (not yet started)
		return err
	}

	// diff returns exit 1 when files differ — not an error.
	// On any other error (including signal-killed), still wait for fancyCmd to avoid a zombie.
	diffErr := diffCmd.Run()
	waitErr := fancyCmd.Wait()

	if diffErr != nil {
		if exitErr, ok := diffErr.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
			return diffErr
		}
	}
	return waitErr
}
