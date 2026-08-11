package skill

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:unparam // path is subPath for future extensibility
func writeSkill(t *testing.T, root, path, name, desc, category, body string) {
	writeSkillFile(t, root, path, name, name, desc, category, body)
}

func writeSkillFile(t *testing.T, root, path, dirName, frontmatterName, desc, category, body string) {
	dir := filepath.Join(root, path, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %q: %v", dir, err)
	}
	content := "---\n"
	if frontmatterName != "" {
		content += "name: " + frontmatterName + "\n"
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

func TestGlobalDiscoveryPaths_Order(t *testing.T) {
	paths := GlobalDiscoveryPaths("/home/user", Config{})
	want := []DiscoveryPath{
		{Dir: "/home/user/.agents/skills", Builtin: true},
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d", len(paths), len(want))
	}
	for i := range paths {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %#v, want %#v", i, paths[i], want[i])
		}
	}
}

func TestGlobalDiscoveryPaths_ConfigExtras(t *testing.T) {
	cfg := Config{
		Global: []string{"/home/user/work/skills", "/srv/shared/skills", "/srv/shared/skills"},
	}
	paths := GlobalDiscoveryPaths("/home/user", cfg)
	want := []DiscoveryPath{
		{Dir: "/home/user/.agents/skills", Builtin: true},
		{Dir: "/home/user/work/skills"},
		{Dir: "/srv/shared/skills"},
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d", len(paths), len(want))
	}
	for i := range paths {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %#v, want %#v", i, paths[i], want[i])
		}
	}
}

func TestGlobalDiscoveryPaths_ExpandsTilde(t *testing.T) {
	cfg := Config{
		Global: []string{"~/work/skills", "~", "/srv/shared/skills", "~/work/skills"},
	}
	paths := GlobalDiscoveryPaths("/home/user", cfg)
	want := []DiscoveryPath{
		{Dir: "/home/user/.agents/skills", Builtin: true},
		{Dir: "/home/user/work/skills"},
		{Dir: "/home/user"},
		{Dir: "/srv/shared/skills"},
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d", len(paths), len(want))
	}
	for i := range paths {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %#v, want %#v", i, paths[i], want[i])
		}
	}
}

func TestGlobalDiscoveryPaths_SkipsTildeWhenHomeEmpty(t *testing.T) {
	cfg := Config{
		Global: []string{"~/work/skills", "/srv/shared/skills", "~"},
	}
	paths := GlobalDiscoveryPaths("", cfg)
	want := []DiscoveryPath{
		{Dir: ".agents/skills", Builtin: true},
		{Dir: "/srv/shared/skills"},
	}
	if len(paths) != len(want) {
		t.Fatalf("got %d paths, want %d", len(paths), len(want))
	}
	for i := range paths {
		if paths[i] != want[i] {
			t.Errorf("paths[%d] = %#v, want %#v", i, paths[i], want[i])
		}
	}
	for _, p := range paths {
		if p.Dir == "." {
			t.Fatalf("tilde entry resolved to current directory: %#v", paths)
		}
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Global) != 0 {
		t.Fatalf("cfg = %#v, want empty", cfg)
	}
}

func TestLoadConfig_TrimsDropsBlanksAndKeepsTilde(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.toml")
	content := `
global = ["~/work/skills", "  ", "/srv/shared/skills", " ~/other/skills "]
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantGlobal := []string{"~/work/skills", "/srv/shared/skills", "~/other/skills"}
	if len(cfg.Global) != len(wantGlobal) {
		t.Fatalf("Global = %v, want %v", cfg.Global, wantGlobal)
	}
	for i := range wantGlobal {
		if cfg.Global[i] != wantGlobal[i] {
			t.Errorf("Global[%d] = %q, want %q", i, cfg.Global[i], wantGlobal[i])
		}
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "skills.toml")
	if err := os.WriteFile(path, []byte("global = [not closed"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestListSkills_AllPathsMissing(t *testing.T) {
	skills, err := ListSkills([]string{"/nonexistent/path/a", "/nonexistent/path/b"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(skills) > 0 {
		t.Errorf("expected empty, got %v", skills)
	}
}

func TestListSkills_FollowsDeployedDirectorySymlink(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".agents", "skills")
	target := filepath.Join(root, "managed", "current", "skill-dir")
	writeSkillFile(t, filepath.Join(root, "managed"), "current", "skill-dir", "shared", "current", "tool", "new body")
	writeSkillFile(t, root, ".agents/skills", "shared.hm-backup", "shared", "stale", "tool", "old body")
	if err := os.Symlink(target, filepath.Join(base, "shared")); err != nil {
		t.Fatal(err)
	}

	skills, err := ListSkills([]string{base})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 || skills[0].Description != "current" || skills[0].Body != "new body" {
		t.Fatalf("skills = %#v, want current symlink target to shadow backup", skills)
	}
}

func TestListSkillsRejectsOversizedSkillBeforeParsing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".agents", "skills", "oversized")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maximumSkillBytes + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ListSkills([]string{filepath.Dir(dir)}); err == nil || !strings.Contains(err.Error(), "1 MiB") {
		t.Fatalf("ListSkills oversized error = %v", err)
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
	// Same skill name in the default dir and a configured extra dir;
	// the earlier path (default) wins.
	defaultDir := filepath.Join(root, ".agents", "skills", "foo")
	extraDir := filepath.Join(root, "work", "skills", "foo")
	if err := os.MkdirAll(defaultDir, 0755); err != nil {
		t.Fatal(err)
	}
	defaultSkillPath := filepath.Join(defaultDir, "SKILL.md")
	defaultContent := "---\nname: foo\ndescription: default version\n---\ndefault body"
	if err := os.WriteFile(defaultSkillPath, []byte(defaultContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extraDir, 0755); err != nil {
		t.Fatal(err)
	}
	extraSkillPath := filepath.Join(extraDir, "SKILL.md")
	extraContent := "---\nname: foo\ndescription: extra version\n---\nextra body"
	if err := os.WriteFile(extraSkillPath, []byte(extraContent), 0644); err != nil {
		t.Fatal(err)
	}

	paths := []string{
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, "work", "skills"),
	}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	s := skills[0]
	if s.Description != "default version" {
		t.Errorf("Description = %q, want %q", s.Description, "default version")
	}
	if s.Source != filepath.Join(root, ".agents", "skills") {
		t.Errorf("Source = %q, want %q", s.Source, filepath.Join(root, ".agents", "skills"))
	}
}

func TestListSkills_UsesFrontmatterNameAsIdentity(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, ".agents/skills", "storage-dir", "bar", "a bar skill", "tools", "bar body")

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

func TestListSkills_SkipsMissingFrontmatterName(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, ".agents/skills", "storage-dir", "", "missing name", "tools", "body")
	writeSkillFile(t, root, ".agents/skills", "valid-dir", "valid", "valid skill", "tools", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "valid" {
		t.Fatalf("Name = %q, want valid", skills[0].Name)
	}
}

func TestListSkills_DedupsByFrontmatterNameFirstWins(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "cwd")
	home := filepath.Join(root, "home")
	writeSkillFile(t, cwd, ".agents/skills", "first-dir", "same-name", "cwd version", "tools", "cwd body")
	writeSkillFile(t, home, ".agents/skills", "second-dir", "same-name", "home version", "tools", "home body")

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
	if skills[0].Description != "cwd version" {
		t.Fatalf("Description = %q, want cwd version", skills[0].Description)
	}
	if skills[0].Path != filepath.Join(cwd, ".agents/skills/first-dir/SKILL.md") {
		t.Fatalf("Path = %q, want first-dir skill", skills[0].Path)
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

	skillDir := filepath.Join(root, ".agents/skills")
	paths := []string{skillDir}
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
	if skill.Source != skillDir {
		t.Errorf("Source = %q, want %q", skill.Source, skillDir)
	}
	if skill.Path != filepath.Join(skillDir, "my-skill/SKILL.md") {
		t.Errorf("Path = %q, want %q", skill.Path, filepath.Join(skillDir, "my-skill/SKILL.md"))
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

	cwdPath := filepath.Join(cwd, ".agents/skills")
	paths := []string{
		cwdPath,
		filepath.Join(home, ".agents/skills"),
	}
	skill, err := GetSkill(paths, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Description != "cwd" {
		t.Errorf("Description = %q, want %q", skill.Description, "cwd")
	}
	if skill.Source != cwdPath {
		t.Errorf("Source = %q, want %q", skill.Source, cwdPath)
	}
	if skill.Path != filepath.Join(cwdPath, "foo/SKILL.md") {
		t.Errorf("Path = %q, want %q", skill.Path, filepath.Join(cwdPath, "foo/SKILL.md"))
	}
}

func TestGetSkill_ResolvesFrontmatterNameNotDirectoryName(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(
		t,
		root,
		".agents/skills",
		"storage-dir",
		"frontmatter-name",
		"a test skill",
		"testing",
		"skill body content",
	)

	skillDir := filepath.Join(root, ".agents/skills")
	paths := []string{skillDir}
	skill, err := GetSkill(paths, "frontmatter-name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Name != "frontmatter-name" {
		t.Fatalf("Name = %q, want frontmatter-name", skill.Name)
	}
	if skill.Path != filepath.Join(skillDir, "storage-dir/SKILL.md") {
		t.Fatalf("Path = %q, want storage-dir skill path", skill.Path)
	}

	_, err = GetSkill(paths, "storage-dir")
	if err == nil {
		t.Fatal("expected directory-name lookup to fail")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("errors.Is(err, fs.ErrNotExist) = false, want true; err = %v", err)
	}
}

func TestGetSkill_SkipsMissingFrontmatterName(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, ".agents/skills", "storage-dir", "", "missing name", "testing", "body")

	_, err := GetSkill([]string{filepath.Join(root, ".agents/skills")}, "storage-dir")
	if err == nil {
		t.Fatal("expected missing-name skill not to resolve")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("errors.Is(err, fs.ErrNotExist) = false, want true; err = %v", err)
	}
}

func searchDiscoveredSkills(t *testing.T, paths []string, query string) []Skill {
	t.Helper()
	skills, err := ListSkills(paths)
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	result, err := SearchSkills(skills, query, DefaultSearchLimit)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	return result
}

func TestSearchDiscoveredSkills_NameMatch(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "git-omz", "git plugin abbreviations", "git", "git body")
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management", "tools", "task body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills := searchDiscoveredSkills(t, paths, "git")
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "git-omz" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "git-omz")
	}
}

func TestSearchDiscoveredSkills_UsesFrontmatterNameOnly(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, ".agents/skills", "storage-dir", "visible-name", "visible description", "tools", "body")
	writeSkillFile(t, root, ".agents/skills", "missing-name-dir", "", "missing name", "tools", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills := searchDiscoveredSkills(t, paths, "visible")
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "visible-name" {
		t.Fatalf("Name = %q, want visible-name", skills[0].Name)
	}

	skills = searchDiscoveredSkills(t, paths, "storage-dir")
	if len(skills) != 0 {
		t.Fatalf("got %d skills for directory-name match, want 0", len(skills))
	}

	skills = searchDiscoveredSkills(t, paths, "missing")
	if len(skills) != 0 {
		t.Fatalf("got %d missing-name skills, want 0", len(skills))
	}
}

func TestSearchDiscoveredSkills_DescriptionMatch(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management using taskwarrior", "tools", "body")
	writeSkill(t, root, ".agents/skills", "git-omz", "git plugin abbreviations", "git", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills := searchDiscoveredSkills(t, paths, "taskwarrior")
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	if skills[0].Name != "taskwarrior" {
		t.Errorf("Name = %q, want %q", skills[0].Name, "taskwarrior")
	}
}

func TestSearchDiscoveredSkills_CaseInsensitive(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "TaskWarrior", "TASK MANAGEMENT", "tools", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills := searchDiscoveredSkills(t, paths, "taskwarrior")
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
}

func TestSearchDiscoveredSkills_MatchesAnyQueryToken(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management", "tools", "body")
	writeSkill(t, root, ".agents/skills", "git-omz", "git plugin", "git", "body")
	writeSkill(t, root, ".agents/skills", "treemd", "read markdown docs", "docs", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills := searchDiscoveredSkills(t, paths, "task git")
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(skills))
	}
}

func TestSearchDiscoveredSkills_NoMatch(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, ".agents/skills", "taskwarrior", "task management", "tools", "body")

	paths := []string{filepath.Join(root, ".agents/skills")}
	skills := searchDiscoveredSkills(t, paths, "nonexistent")
	if len(skills) > 0 {
		t.Errorf("expected empty, got %v", skills)
	}
}

func TestSearchDiscoveredSkills_HigherPriorityDuplicateStillShadowsLowerMatch(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	global := filepath.Join(root, "global")
	writeSkillFile(t, project, ".agents/skills", "project-dir", "shared", "project description", "tools", "project body")
	writeSkillFile(t, global, ".agents/skills", "global-dir", "shared", "matching keyword", "tools", "global body")

	skills := searchDiscoveredSkills(t, []string{
		filepath.Join(project, ".agents/skills"),
		filepath.Join(global, ".agents/skills"),
	}, "matching")
	if len(skills) != 0 {
		t.Fatalf("got %#v, want higher-priority non-match to shadow lower duplicate", skills)
	}
}

func TestSearchSkillsRanksQueryCoverageBeforeSingleTermMatches(t *testing.T) {
	skills := []Skill{
		{Name: "plan-triage", Description: "Triage implementation plans"},
		{Name: "pr-review-loop", Description: "Review pull requests in a repeated loop"},
		{Name: "review-notes", Description: "Review collected notes"},
	}

	got, err := SearchSkills(skills, "review loop triage", 2)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(got) != 2 || got[0].Name != "pr-review-loop" || got[1].Name != "plan-triage" {
		t.Fatalf("SearchSkills() = %#v", got)
	}
}

func TestSearchSkillsSplitsCamelCaseAndPunctuation(t *testing.T) {
	skills := []Skill{
		{Name: "http-client", Description: "Send network requests"},
		{Name: "HTTPServerAudit", Description: "Inspect service configuration"},
	}

	got, err := SearchSkills(skills, "http server", DefaultSearchLimit)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(got) != 2 || got[0].Name != "HTTPServerAudit" {
		t.Fatalf("SearchSkills() = %#v", got)
	}
}

func TestSearchSkillsMatchesDescriptions(t *testing.T) {
	skills := []Skill{{Name: "research", Description: "Collect current evidence"}}

	got, err := SearchSkills(skills, "evidence", DefaultSearchLimit)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(got) != 1 || got[0].Name != "research" {
		t.Fatalf("SearchSkills() = %#v", got)
	}
}

func TestSearchSkillsMatchesCategories(t *testing.T) {
	skills := []Skill{{Name: "research", Category: "methodology"}}

	got, err := SearchSkills(skills, "methodology", DefaultSearchLimit)
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(got) != 1 || got[0].Name != "research" {
		t.Fatalf("SearchSkills() = %#v", got)
	}
}
