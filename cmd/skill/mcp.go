package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/project"
	"github.com/tta-lab/organon/internal/skill"
)

type skillProjectGetter interface {
	Get(alias string) (project.Entry, error)
}

type skillListInput struct {
	Project string `json:"project,omitempty" jsonschema:"exact project alias; omit for global skills only"`
}

type skillFindInput struct {
	Project  string   `json:"project,omitempty" jsonschema:"exact project alias; omit for global skills only"`
	Keywords []string `json:"keywords" jsonschema:"one or more case-insensitive keywords matched with OR semantics"`
}

type skillGetInput struct {
	Project string `json:"project,omitempty" jsonschema:"exact project alias; omit for global skills only"`
	Name    string `json:"name" jsonschema:"exact case-sensitive skill name from frontmatter"`
}

type skillSummaryOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Scope       string `json:"scope"`
	Source      string `json:"source"`
}

type skillDetailOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Scope       string `json:"scope"`
	Source      string `json:"source"`
	Path        string `json:"path"`
	Body        string `json:"body"`
}

type skillListOutput struct {
	Skills []skillSummaryOutput `json:"skills"`
}

type skillGetOutput struct {
	Skill skillDetailOutput `json:"skill"`
}

type skillSource struct {
	scope string
	name  string
}

type skillMCPService struct {
	home     string
	projects skillProjectGetter
}

func skillBoolPointer(value bool) *bool { return &value }

func skillTool(name, title, description string) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			Title:           title,
			ReadOnlyHint:    true,
			DestructiveHint: skillBoolPointer(false),
			IdempotentHint:  true,
			OpenWorldHint:   skillBoolPointer(false),
		},
	}
}

func (s skillMCPService) discovery(projectAlias string) ([]string, map[string]skillSource, error) {
	paths := make([]string, 0, 8)
	sources := make(map[string]skillSource, 8)
	add := func(scope string, discoveryPaths []string) {
		labels := []string{".agents", ".crush", ".claude", ".cursor"}
		for i, path := range discoveryPaths {
			paths = append(paths, path)
			sources[filepath.Clean(path)] = skillSource{scope: scope, name: scope + ":" + labels[i]}
		}
	}

	projectAlias = strings.TrimSpace(projectAlias)
	if projectAlias != "" {
		entry, err := s.projects.Get(projectAlias)
		if err != nil {
			return nil, nil, fmt.Errorf("get project %q: %w", projectAlias, err)
		}
		add("project", skill.ProjectDiscoveryPaths(entry.Path))
	}
	add("global", skill.GlobalDiscoveryPaths(s.home))
	return paths, sources, nil
}

func (s skillMCPService) list(projectAlias string) ([]skill.Skill, map[string]skillSource, error) {
	paths, sources, err := s.discovery(projectAlias)
	if err != nil {
		return nil, nil, err
	}
	skills, err := skill.ListSkills(paths)
	if err != nil {
		return nil, nil, err
	}
	return skills, sources, nil
}

func skillSummaries(skills []skill.Skill, sources map[string]skillSource) []skillSummaryOutput {
	result := make([]skillSummaryOutput, 0, len(skills))
	for _, candidate := range skills {
		source := sources[filepath.Clean(candidate.Source)]
		result = append(result, skillSummaryOutput{
			Name: candidate.Name, Description: candidate.Description, Category: candidate.Category,
			Scope: source.scope, Source: source.name,
		})
	}
	return result
}

func newSkillMCPServer(home string, projects skillProjectGetter) *mcp.Server {
	service := skillMCPService{home: home, projects: projects}
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-skill", Version: "1.0.0"}, nil)

	mcp.AddTool(server, skillTool(
		"skill_list", "List agent skills", "List deduplicated skill metadata in discovery priority order.",
	), func(
		_ context.Context, _ *mcp.CallToolRequest, input skillListInput,
	) (*mcp.CallToolResult, skillListOutput, error) {
		skills, sources, err := service.list(input.Project)
		if err != nil {
			return nil, skillListOutput{}, fmt.Errorf("list skills: %w", err)
		}
		return nil, skillListOutput{Skills: skillSummaries(skills, sources)}, nil
	})

	mcp.AddTool(server, skillTool(
		"skill_find", "Find agent skills", "Find deduplicated skills by case-insensitive keyword OR match.",
	), func(
		_ context.Context, _ *mcp.CallToolRequest, input skillFindInput,
	) (*mcp.CallToolResult, skillListOutput, error) {
		keywords := make([]string, 0, len(input.Keywords))
		for _, keyword := range input.Keywords {
			if keyword = strings.TrimSpace(keyword); keyword != "" {
				keywords = append(keywords, keyword)
			}
		}
		if len(keywords) == 0 {
			return nil, skillListOutput{}, fmt.Errorf("keywords must contain at least one non-blank value")
		}
		paths, sources, err := service.discovery(input.Project)
		if err != nil {
			return nil, skillListOutput{}, fmt.Errorf("find skills: %w", err)
		}
		skills, err := skill.FindSkills(paths, keywords)
		if err != nil {
			return nil, skillListOutput{}, fmt.Errorf("find skills: %w", err)
		}
		return nil, skillListOutput{Skills: skillSummaries(skills, sources)}, nil
	})

	mcp.AddTool(server, skillTool(
		"skill_get", "Get agent skill", "Read one skill by exact case-sensitive frontmatter name.",
	), func(
		_ context.Context, _ *mcp.CallToolRequest, input skillGetInput,
	) (*mcp.CallToolResult, skillGetOutput, error) {
		name := strings.TrimSpace(input.Name)
		if name == "" {
			return nil, skillGetOutput{}, fmt.Errorf("name must not be blank")
		}
		paths, sources, err := service.discovery(input.Project)
		if err != nil {
			return nil, skillGetOutput{}, fmt.Errorf("get skill: %w", err)
		}
		found, err := skill.GetSkill(paths, name)
		if err != nil {
			return nil, skillGetOutput{}, fmt.Errorf("get skill %q: %w", name, err)
		}
		source := sources[filepath.Clean(found.Source)]
		return nil, skillGetOutput{Skill: skillDetailOutput{
			Name: found.Name, Description: found.Description, Category: found.Category,
			Scope: source.scope, Source: source.name, Path: found.Path, Body: found.Body,
		}}, nil
	})

	return server
}

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve typed skill discovery tools over stdio MCP",
		Long:  helpMCP,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("determine home directory: %w", err)
			}
			return newSkillMCPServer(home, project.NewStore(config.ProjectsPath())).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
