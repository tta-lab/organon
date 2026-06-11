package goal

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Valid status values.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusBlocked  = "blocked"
	StatusComplete = "complete"
)

// Frontmatter is the YAML frontmatter in a goal file.
type Frontmatter struct {
	Status    string `yaml:"status"`
	CreatedAt string `yaml:"created_at"`
	UpdatedAt string `yaml:"updated_at"`
}

// File represents a parsed goal file.
type File struct {
	Path        string
	Frontmatter Frontmatter
	Body        string
	rawPrefix   string // everything before closing --- including the leading ---
}

// validStatuses is the set of allowed status values.
var validStatuses = map[string]bool{
	StatusDraft:    true,
	StatusActive:   true,
	StatusBlocked:  true,
	StatusComplete: true,
}

// IsValidStatus reports whether s is a recognized status.
func IsValidStatus(s string) bool {
	return validStatuses[s]
}

// ValidStatuses returns a sorted list of valid statuses for error messages.
func ValidStatuses() string {
	return StatusDraft + " | " + StatusActive + " | " + StatusBlocked + " | " + StatusComplete
}

// GoalPath returns the absolute path from LENOS_GOAL env or an error.
func GoalPath() (string, error) {
	p := os.Getenv("LENOS_GOAL")
	if p == "" {
		return "", errors.New("LENOS_GOAL is not set; goal CLI only works inside a Lenos goal session")
	}
	return p, nil
}

// Parse reads and parses a goal file at path.
func Parse(path string) (*File, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read goal file: %w", err)
	}
	return parseContent(path, raw)
}

func parseContent(path string, raw []byte) (*File, error) {
	content := string(raw)

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r") {
		return nil, errors.New("invalid goal file: missing YAML frontmatter delimiter")
	}

	// Find closing ---
	after := content[3:] // skip leading ---
	idx := strings.Index(after, "\n---")
	if idx < 0 {
		return nil, errors.New("invalid goal file: missing closing YAML frontmatter delimiter")
	}

	fmText := after[:idx]
	bodyStart := idx + 4 // skip "\n---"

	// Skip leading newline in body
	body := after[bodyStart:]
	body = strings.TrimLeft(body, "\r\n")

	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(fmText), &fm); err != nil {
		return nil, fmt.Errorf("invalid goal frontmatter: %w", err)
	}

	// Validate required fields.
	if fm.Status == "" {
		return nil, errors.New("invalid goal frontmatter: missing status")
	}
	if !IsValidStatus(fm.Status) {
		return nil, fmt.Errorf("invalid goal status %q: must be %s", fm.Status, ValidStatuses())
	}
	if fm.CreatedAt == "" {
		return nil, errors.New("invalid goal frontmatter: missing created_at")
	}

	f := &File{
		Path:        path,
		Frontmatter: fm,
		Body:        body,
		rawPrefix:   content[:bodyStart],
	}

	return f, nil
}

// Add creates a new goal file at path with the given body and optional status.
// If status is empty, StatusDraft is used. Fails if the file already exists.
func Add(path, body, status string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("goal file already exists at %s; use --force to overwrite", path)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create goal directory: %w", err)
	}

	if status == "" {
		status = StatusDraft
	}
	if !IsValidStatus(status) {
		return fmt.Errorf("invalid goal status %q: must be %s", status, ValidStatuses())
	}

	now := time.Now().Format(time.RFC3339)
	fm := Frontmatter{
		Status:    status,
		CreatedAt: now,
		UpdatedAt: now,
	}

	return atomicWrite(path, fm, body)
}

// Update replaces the body of the goal file, preserving the current status.
func Update(path, body string) error {
	f, err := Parse(path)
	if err != nil {
		return err
	}

	f.Frontmatter.UpdatedAt = time.Now().Format(time.RFC3339)
	return atomicWrite(path, f.Frontmatter, body)
}

// Append appends text to the goal file body, separated by a blank line.
func Append(path, text string) error {
	f, err := Parse(path)
	if err != nil {
		return err
	}

	newBody := strings.TrimRight(f.Body, "\n")
	if newBody != "" {
		newBody += "\n\n"
	}
	newBody += text

	f.Frontmatter.UpdatedAt = time.Now().Format(time.RFC3339)
	return atomicWrite(path, f.Frontmatter, newBody)
}

// SetStatus updates only the status and updated_at fields.
func SetStatus(path, status string) error {
	if !IsValidStatus(status) {
		return fmt.Errorf("invalid goal status %q: must be %s", status, ValidStatuses())
	}

	f, err := Parse(path)
	if err != nil {
		return err
	}

	f.Frontmatter.Status = status
	f.Frontmatter.UpdatedAt = time.Now().Format(time.RFC3339)
	return atomicWrite(path, f.Frontmatter, f.Body)
}

// Get returns the parsed goal file. It is the read counterpart to the
// mutating operations.
func Get(path string) (*File, error) {
	return Parse(path)
}

// atomicWrite writes the goal file atomically via temp file + rename.
func atomicWrite(path string, fm Frontmatter, body string) error {
	var buf bytes.Buffer

	buf.WriteString("---\n")

	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return fmt.Errorf("marshal frontmatter: %w", err)
	}
	buf.Write(fmBytes)
	buf.WriteString("---\n")

	if body != "" {
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteString("\n")
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".goal-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("atomic rename: %w", err)
	}
	return nil
}
