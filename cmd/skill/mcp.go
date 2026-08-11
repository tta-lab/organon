package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/skill"
)

type skillListInput struct{}

type skillFindInput struct {
	Query string `json:"query" jsonschema:"search query matched against skill names, descriptions, and categories"`
	Limit *int   `json:"limit,omitempty" jsonschema:"maximum results; defaults to 8 and is capped at 32"`
}

type skillGetInput struct {
	Name string `json:"name" jsonschema:"exact case-sensitive skill name from frontmatter"`
}

type skillSummaryOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Source      string `json:"source"` // absolute path of the discovery directory
}

type skillDetailOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Source      string `json:"source"` // absolute path of the discovery directory
	Path        string `json:"path"`   // absolute path to SKILL.md
	Body        string `json:"body"`
}

type skillListOutput struct {
	Skills []skillSummaryOutput `json:"skills"`
}

type skillGetOutput struct {
	Skill skillDetailOutput `json:"skill"`
}

// skillMCPService serves skill tools. Skills are discovered from the global
// ~/.agents/skills directory plus configured extras. loadCfg reloads the
// skills.toml config on every request so edits take effect without a restart.
type skillMCPService struct {
	home    string
	loadCfg func() (skill.Config, error)
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

func (s skillMCPService) catalog() (skill.Catalog, error) {
	cfg, err := s.loadCfg()
	if err != nil {
		return skill.Catalog{}, err
	}
	roots := skill.GlobalDiscoveryPaths(s.home, cfg)
	dirs := make([]string, 0, len(roots))
	for _, root := range roots {
		dirs = append(dirs, root.Dir)
	}
	globalSkills, err := skill.ListSkills(dirs)
	if err != nil {
		return skill.Catalog{}, err
	}
	return skill.NewCatalog(globalSkills), nil
}

func skillSummaries(skills []skill.Skill) []skillSummaryOutput {
	result := make([]skillSummaryOutput, 0, len(skills))
	for _, candidate := range skills {
		result = append(result, skillSummaryOutput{
			Name:        candidate.Name,
			Description: candidate.Description,
			Category:    candidate.Category,
			Source:      candidate.Source,
		})
	}
	return result
}

func newSkillMCPServer(home string, loadCfg func() (skill.Config, error)) *mcp.Server {
	if loadCfg == nil {
		loadCfg = func() (skill.Config, error) { return skill.LoadConfig(config.SkillsConfigPath()) }
	}
	service := skillMCPService{home: home, loadCfg: loadCfg}
	server := mcp.NewServer(&mcp.Implementation{Name: "organon-skill", Version: "1.0.0"}, nil)

	mcp.AddTool(server, skillTool(
		"skill_list", "List agent skills", "List deduplicated skill metadata in discovery priority order.",
	), func(
		_ context.Context, _ *mcp.CallToolRequest, _ skillListInput,
	) (*mcp.CallToolResult, skillListOutput, error) {
		catalog, err := service.catalog()
		if err != nil {
			return nil, skillListOutput{}, fmt.Errorf("list skills: %w", err)
		}
		return nil, skillListOutput{Skills: skillSummaries(catalog.List())}, nil
	})

	mcp.AddTool(server, skillTool(
		"skill_find", "Find agent skills", "Find and rank skills for a natural-language query.",
	), func(
		_ context.Context, _ *mcp.CallToolRequest, input skillFindInput,
	) (*mcp.CallToolResult, skillListOutput, error) {
		limit := skill.DefaultSearchLimit
		if input.Limit != nil {
			limit = *input.Limit
		}
		catalog, err := service.catalog()
		if err != nil {
			return nil, skillListOutput{}, fmt.Errorf("find skills: %w", err)
		}
		skills, err := catalog.Find(input.Query, limit)
		if err != nil {
			return nil, skillListOutput{}, err
		}
		return nil, skillListOutput{Skills: skillSummaries(skills)}, nil
	})

	mcp.AddTool(server, skillTool(
		"skill_get", "Get agent skill", "Read one skill by exact case-sensitive frontmatter name.",
	), func(
		_ context.Context, _ *mcp.CallToolRequest, input skillGetInput,
	) (*mcp.CallToolResult, skillGetOutput, error) {
		catalog, err := service.catalog()
		if err != nil {
			return nil, skillGetOutput{}, fmt.Errorf("get skill: %w", err)
		}
		found, err := catalog.Get(input.Name)
		if err != nil {
			return nil, skillGetOutput{}, err
		}
		return nil, skillGetOutput{Skill: skillDetailOutput{
			Name:        found.Name,
			Description: found.Description,
			Category:    found.Category,
			Source:      found.Source,
			Path:        found.Path,
			Body:        found.Body,
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
			return newSkillMCPServer(home, nil).Run(cmd.Context(), &mcp.StdioTransport{})
		},
	}
}
