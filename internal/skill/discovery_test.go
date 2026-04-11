package skill

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

//nolint:unparam // path is subPath for future extensibility
func writeSkill(t *testing.T, root, path, name, desc, category, body string) {
	dir := filepath.Join(root, path, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	content := "---\n"
	if name != "" {
		content += "name: " + name + "\n"
	}
	if desc != "" {
		content += "description: " + desc + "\n"
	}
	if category != "" {
		content += "category: " + category + "\n"
	}
	content += "---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func TestDiscoveryPaths_Order(t *testing.T) {
	paths := DiscoveryPaths("/my/cwd", "/home/user")
	want := []string{
		"/my/cwd/.agents/skills",
		"/my/cwd/.crush/skills",
		"/my/cwd/.claude/skills",
		"/my/cwd/.cursor/skills",
		"/home/user/.agents/skills",
		"/home/user/.crush/skills",
		"/home/user/.claude/skills",
		"/home/user/.cursor/skills",
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d", len(paths), len(want))
	}
	for i := range paths {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], want[i])
		}
	}
}

func TestListSkills_AllPathsMissing(t *testing.T) {
	skills, err := ListSkills([]string{"/nonexistent/path/a", "/nonexistent/path/b"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil, got %v", skills)
	}
}

func TestListSkills_SinglePath_MultipleSkills(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "zebra", "a zebra skill", "animals", "zebra body")
	writeSkill(t, root, ".agents/skills", "alpha", "an alpha skill", "letters", "alpha body")
	writeSkill(t, root, ".agents/skills", "beta", "a beta skill", "letters", "beta body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 3 {
		t.Fatalf("got %d skills, want 3", len(skills))
	}
	// Should be sorted by name
	wantNames := []string{"alpha", "beta", "zebra"}
	for i, s := range skills {
		if s.Name != wantNames[i] {
			t.Errorf("skills[%d].Name = %q, want %q", i, s.Name, wantNames[i])
		}
	}
}

func TestListSkills_DedupFirstWins(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(cwd, ".agents/skills/foo"), 0755); err != nil {
		t.Fatal(err)
	}
	cwdSkillPath := filepath.Join(cwd, ".agents/skills/foo/SKILL.md")
	cwdContent := "---\nname: foo\ndescription: cwd version\n---\ncwd body"
	if err := os.WriteFile(cwdSkillPath, []byte(cwdContent), 0644); err != nil {
		t.Fatal(err)
	}
	homeSkillPath := filepath.Join(home, ".agents/skills/foo/SKILL.md")
	if err := os.MkdirAll(filepath.Join(home, ".agents/skills/foo"), 0755); err != nil {
		t.Fatal(err)
	}
	homeContent := "---\nname: foo\ndescription: home version\n---\nhome body"
	if err := os.WriteFile(homeSkillPath, []byte(homeContent), 0644); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		filepath.Join(cwd, ".agents/skills"),
		filepath.Join(home, ".agents/skills"),
	}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	s := skills[0]
	if s.Name != "foo" {
		t.Errorf("Name = %q, want %q", s.Name, "foo")
	}
	if s.Description != "cwd version" {
		t.Errorf("Description = %q, want %q", s.Description, "cwd version")
	}
	if s.Source != filepath.Join(cwd, ".agents/skills") {
		t.Errorf("Source = %q, want %q", s.Source, filepath.Join(cwd, ".agents/skills"))
	}
	if s.Path != filepath.Join(cwd, ".agents/skills/foo/SKILL.md") {
		t.Errorf("Path = %q, want %q", s.Path, filepath.Join(cwd, ".agents/skills/foo/SKILL.md"))
	}
}

func TestListSkills_CrossDirDedup(t *testing.T) {
	root := t.TempDir()
	cwdCrush := filepath.Join(root, "cwd/.crush/skills/foo")
	homeAgents := filepath.Join(root, "home/.agents/skills/foo")
	if err := os.MkdirAll(cwdCrush, 0755); err != nil {
		t.Fatal(err)
	}
	crushSkillPath := filepath.Join(cwdCrush, "SKILL.md")
	crushContent := "---\nname: foo\ndescription: crush version\n---\ncrush body"
	if err := os.WriteFile(crushSkillPath, []byte(crushContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(homeAgents, 0755); err != nil {
		t.Fatal(err)
	}
	agentsSkillPath := filepath.Join(homeAgents, "SKILL.md")
	agentsContent := "---\nname: foo\ndescription: agents version\n---\nagents body"
	if err := os.WriteFile(agentsSkillPath, []byte(agentsContent), 0644); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		filepath.Join(root, "cwd/.agents/skills"),
		filepath.Join(root, "cwd/.crush/skills"),
		filepath.Join(root, "cwd/.claude/skills"),
		filepath.Join(root, "cwd/.cursor/skills"),
		filepath.Join(root, "home/.agents/skills"),
		filepath.Join(root, "home/.crush/skills"),
		filepath.Join(root, "home/.claude/skills"),
		filepath.Join(root, "home/.cursor/skills"),
	}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	s := skills[0]
	if s.Description != "crush version" {
		t.Errorf("Description = %q, want %q", s.Description, "crush version")
	}
	if s.Source != filepath.Join(root, "cwd/.crush/skills") {
		t.Errorf("Source = %q, want %q", s.Source, filepath.Join(root, "cwd/.crush/skills"))
	}
}

func TestListSkills_FrontmatterNameOverridesDir(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "bar", "a bar skill", "tools", "bar body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "bar" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "bar")
	}
}

func TestListSkills_DirWithoutSKILLMd_Skipped(t *testing.T) {
	root := t.TempDir()
	// Create a dir without SKILL.md
	dir := filepath.Join(root, ".agents/skills/just-a-dir")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("not a skill"), 0644); err != nil {
		t.Fatal(err)
	}
	// And a real skill
	writeSkill(t, root, ".agents/skills", "real", "real skill", "tools", "real body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "real" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "real")
	}
}

func TestListSkills_NonDirEntriesSkipped(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, ".agents/skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Place a loose file directly in the skills dir
	if err := os.WriteFile(filepath.Join(skillsDir, "README.md"), []byte("not a skill"), 0644); err != nil {
		t.Fatal(err)
	}
	// And a real skill
	writeSkill(t, root, ".agents/skills", "real", "real skill", "tools", "real body")

	paths := []string{skillsDir}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "real" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "real")
	}
}

func TestGetSkill_Found(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "my-skill", "a test skill", "testing", "skill body content")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skill, err := GetSkill(paths, "my-skill")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Name != "my-skill" {
		t.Errorf("Name = %q, want %q", skill.Name, "my-skill")
	}
	if skill.Description != "a test skill" {
		t.Errorf("Description = %q, want %q", skill.Description, "a test skill")
	}
	if skill.Category != "testing" {
		t.Errorf("Category = %q, want %q", skill.Category, "testing")
	}
	if skill.Body != "skill body content" {
		t.Errorf("Body = %q, want %q", skill.Body, "skill body content")
	}
}

func TestGetSkill_NotFound(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, ".agents/skills")}
	_, err := GetSkill(paths, "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is(err, fs.ErrNotExist) = false, want true; err = %v", err)
	}
}

func TestGetSkill_PriorityWins(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(cwd, ".agents/skills/foo"), 0755); err != nil {
		t.Fatal(err)
	}
	cwdSkill := filepath.Join(cwd, ".agents/skills/foo/SKILL.md")
	cwdContent := "---\nname: foo\ndescription: cwd\n---\ncwd body"
	if err := os.WriteFile(cwdSkill, []byte(cwdContent), 0644); err != nil {
		t.Fatal(err)
	}
	homeSkill := filepath.Join(home, ".agents/skills/foo/SKILL.md")
	if err := os.MkdirAll(filepath.Join(home, ".agents/skills/foo"), 0755); err != nil {
		t.Fatal(err)
	}
	homeContent := "---\nname: foo\ndescription: home\n---\nhome body"
	if err := os.WriteFile(homeSkill, []byte(homeContent), 0644); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		filepath.Join(cwd, ".agents/skills"),
		filepath.Join(home, ".agents/skills"),
	}
	skill, err := GetSkill(paths, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Description != "cwd" {
		t.Errorf("Description = %q, want %q", skill.Description, "cwd")
	}
}

func TestFindSkills_NameMatch(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "git-omz", "git plugin abbreviations", "git", "git body")
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management", "tools", "task body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := FindSkills(paths, []string{"git"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "git-omz" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "git-omz")
	}
}

func TestFindSkills_DescriptionMatch(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management using taskwarrior", "tools", "body")
	writeSkill(t, root, ".agents/skills", "git-omz", "git plugin abbreviations", "git", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := FindSkills(paths, []string{"taskwarrior"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "taskwarrior" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "taskwarrior")
	}
}

func TestFindSkills_CaseInsensitive(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "TaskWarrior", "TASK MANAGEMENT", "tools", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := FindSkills(paths, []string{"taskwarrior"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
}

func TestFindSkills_MultipleKeywordsOR(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management", "tools", "body")
	writeSkill(t, root, ".agents/skills", "git-omz", "git plugin", "git", "body")
	writeSkill(t, root, ".agents/skills", "treemd", "read markdown docs", "docs", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := FindSkills(paths, []string{"task", "git"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(skills))
	}
}

func TestFindSkills_NoMatch(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management", "tools", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := FindSkills(paths, []string{"nonexistent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil, got %v", skills)
	}
}
