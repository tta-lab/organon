package goal

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseValid(t *testing.T) {
	content := `---
status: draft
created_at: 2026-06-11T12:00:00+08:00
updated_at: 2026-06-11T12:10:00+08:00
---

# Goal

Implement X
`
	f, err := parseContent("/fake/path.md", []byte(content))
	require.NoError(t, err)
	assert.Equal(t, "draft", f.Frontmatter.Status)
	assert.Equal(t, "2026-06-11T12:00:00+08:00", f.Frontmatter.CreatedAt)
	assert.Equal(t, "2026-06-11T12:10:00+08:00", f.Frontmatter.UpdatedAt)
	assert.Equal(t, "# Goal\n\nImplement X\n", f.Body)
}

func TestParseMissingFrontmatter(t *testing.T) {
	_, err := parseContent("/fake", []byte("# Just markdown"))
	assert.ErrorContains(t, err, "missing YAML frontmatter delimiter")
}

func TestParseMissingClosingDelim(t *testing.T) {
	content := `---
status: draft
`
	_, err := parseContent("/fake", []byte(content))
	assert.ErrorContains(t, err, "missing closing YAML frontmatter delimiter")
}

func TestParseMissingStatus(t *testing.T) {
	content := `---
created_at: 2026-06-11T12:00:00+08:00
---

body
`
	_, err := parseContent("/fake", []byte(content))
	assert.ErrorContains(t, err, "missing status")
}

func TestParseInvalidStatus(t *testing.T) {
	content := `---
status: bogus
created_at: 2026-06-11T12:00:00+08:00
---

body
`
	_, err := parseContent("/fake", []byte(content))
	assert.ErrorContains(t, err, "invalid goal status")
}

func TestParseMissingCreatedAt(t *testing.T) {
	content := `---
status: draft
---

body
`
	_, err := parseContent("/fake", []byte(content))
	assert.ErrorContains(t, err, "missing created_at")
}

func TestInvalidYAML(t *testing.T) {
	content := `---
status: [unclosed
---

body
`
	_, err := parseContent("/fake", []byte(content))
	assert.ErrorContains(t, err, "invalid goal frontmatter")
}

func TestAddDefaultDraft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Test Goal", "", false)
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, StatusDraft, f.Frontmatter.Status)
	assert.NotEmpty(t, f.Frontmatter.CreatedAt)
	assert.NotEmpty(t, f.Frontmatter.UpdatedAt)
	assert.Equal(t, "# Test Goal\n", f.Body)
}

func TestAddWithStatusActive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Goal", StatusActive, false)
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, f.Frontmatter.Status)
}

func TestAddFailsWhenExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# First", StatusDraft, false)
	require.NoError(t, err)

	err = Add(path, "# Second", StatusDraft, false)
	assert.ErrorContains(t, err, "already exists")
}

func TestAddForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# First", StatusDraft, false)
	require.NoError(t, err)

	err = Add(path, "# Second", StatusActive, true)
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "# Second\n", f.Body)
	assert.Equal(t, StatusActive, f.Frontmatter.Status)
}

func TestAddInvalidStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Goal", "bogus", false)
	assert.ErrorContains(t, err, "invalid goal status")
}

func TestAddCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "goal.md")

	err := Add(path, "# Goal", "", false)
	require.NoError(t, err)

	assert.FileExists(t, path)
}

func TestUpdatePreservesStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Original", StatusActive, false)
	require.NoError(t, err)

	err = Update(path, "# Updated")
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, f.Frontmatter.Status)
	assert.Equal(t, "# Updated\n", f.Body)
}

func TestAppendPreservesStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Original", StatusActive, false)
	require.NoError(t, err)

	err = Append(path, "More text")
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, StatusActive, f.Frontmatter.Status)
	assert.Contains(t, f.Body, "# Original")
	assert.Contains(t, f.Body, "More text")
}

func TestAppendBlankLineSeparation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# First", StatusDraft, false)
	require.NoError(t, err)

	err = Append(path, "# Second")
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "# First\n\n# Second\n", f.Body)
}

func TestSetStatusChangesOnlyStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Body", StatusDraft, false)
	require.NoError(t, err)

	original, err := Parse(path)
	require.NoError(t, err)

	// Ensure time-tick so UpdatedAt differs.
	time.Sleep(1 * time.Second)

	err = SetStatus(path, StatusComplete)
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, StatusComplete, f.Frontmatter.Status)
	assert.Equal(t, original.Frontmatter.CreatedAt, f.Frontmatter.CreatedAt)
	assert.NotEqual(t, original.Frontmatter.UpdatedAt, f.Frontmatter.UpdatedAt)
	assert.Equal(t, "# Body\n", f.Body)
}

func TestSetStatusInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Goal", StatusDraft, false)
	require.NoError(t, err)

	err = SetStatus(path, "bogus")
	assert.ErrorContains(t, err, "invalid goal status")
}

func TestGoalPathUnset(t *testing.T) {
	_ = os.Unsetenv("LENOS_GOAL")
	_, err := GoalPath()
	assert.ErrorContains(t, err, "LENOS_GOAL is not set")
}

func TestGoalPathSet(t *testing.T) {
	t.Setenv("LENOS_GOAL", "/tmp/test-goal.md")

	p, err := GoalPath()
	require.NoError(t, err)
	assert.Equal(t, "/tmp/test-goal.md", p)
}

func TestAtomicWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "goal.md")

	err := Add(path, "# Hello\n\nWorld", StatusDraft, false)
	require.NoError(t, err)

	f, err := Parse(path)
	require.NoError(t, err)
	assert.Equal(t, "# Hello\n\nWorld\n", f.Body)
}

func TestGetFailsForMissingFile(t *testing.T) {
	_, err := Get("/nonexistent/path/goal.md")
	assert.ErrorContains(t, err, "read goal file")
}

func TestUpdateFailsForMissingFile(t *testing.T) {
	err := Update("/nonexistent/path/goal.md", "# Body")
	assert.ErrorContains(t, err, "read goal file")
}

func TestAppendFailsForMissingFile(t *testing.T) {
	err := Append("/nonexistent/path/goal.md", "More")
	assert.ErrorContains(t, err, "read goal file")
}

func TestSetStatusFailsForMissingFile(t *testing.T) {
	err := SetStatus("/nonexistent/path/goal.md", StatusComplete)
	assert.ErrorContains(t, err, "read goal file")
}
