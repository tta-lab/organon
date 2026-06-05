package codexusage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const DefaultConfigPath = "~/.local/share/lenos/config.json"

type CodexConfig struct {
	AccessToken  string
	RefreshToken string
	AccountID    string
}

type WeeklyUsage struct {
	Plan             string
	UsedPercent      int
	RemainingPercent int
	RefreshAt        time.Time
}

type lenosConfig struct {
	Providers map[string]providerConfig `json:"providers"`
}

type providerConfig struct {
	APIKey       string            `json:"api_key"`
	OAuth        oauthToken        `json:"oauth"`
	ExtraHeaders map[string]string `json:"extra_headers"`
}

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type usageResponse struct {
	PlanType  string          `json:"plan_type"`
	RateLimit rateLimitDetail `json:"rate_limit"`
}

type rateLimitDetail struct {
	SecondaryWindow windowSnapshot `json:"secondary_window"`
}

type windowSnapshot struct {
	UsedPercent int   `json:"used_percent"`
	ResetAt     int64 `json:"reset_at"`
}

func LoadConfig(path string) (CodexConfig, error) {
	if path == "" || path == DefaultConfigPath {
		path = defaultConfigPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return CodexConfig{}, fmt.Errorf("read config: %w", err)
	}
	var config lenosConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return CodexConfig{}, fmt.Errorf("parse config: %w", err)
	}
	provider, ok := config.Providers["codex"]
	if !ok {
		return CodexConfig{}, fmt.Errorf("codex provider not found")
	}
	accessToken := provider.OAuth.AccessToken
	if accessToken == "" {
		accessToken = provider.APIKey
	}
	if accessToken == "" {
		return CodexConfig{}, fmt.Errorf("codex access token not found")
	}
	return CodexConfig{
		AccessToken:  accessToken,
		RefreshToken: provider.OAuth.RefreshToken,
		AccountID:    provider.ExtraHeaders["ChatGPT-Account-Id"],
	}, nil
}

func FetchWeeklyUsage(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	config CodexConfig,
) (WeeklyUsage, error) {
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL(baseURL), nil)
	if err != nil {
		return WeeklyUsage{}, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+config.AccessToken)
	request.Header.Set("User-Agent", "organon-codex-usage")
	request.Header.Set("Accept", "application/json")
	if config.AccountID != "" {
		request.Header.Set("ChatGPT-Account-Id", config.AccountID)
	}

	response, err := client.Do(request)
	if err != nil {
		return WeeklyUsage{}, fmt.Errorf("fetch usage: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return WeeklyUsage{}, fmt.Errorf("read response: %w", err)
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return WeeklyUsage{}, fmt.Errorf("codex token expired or invalid")
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return WeeklyUsage{}, fmt.Errorf("codex usage API returned %d: %s", response.StatusCode, string(body))
	}

	var decoded usageResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return WeeklyUsage{}, fmt.Errorf("parse response: %w", err)
	}
	weekly := decoded.RateLimit.SecondaryWindow
	if weekly.ResetAt == 0 {
		return WeeklyUsage{}, fmt.Errorf("weekly usage window not found")
	}

	used := clampPercent(weekly.UsedPercent)
	return WeeklyUsage{
		Plan:             decoded.PlanType,
		UsedPercent:      used,
		RemainingPercent: 100 - used,
		RefreshAt:        time.Unix(weekly.ResetAt, 0),
	}, nil
}

func FormatWeeklyUsage(usage WeeklyUsage) string {
	now := time.Now()
	return FormatWeeklyUsageAt(usage, now, now.Location())
}

func FormatWeeklyUsageAt(usage WeeklyUsage, now time.Time, location *time.Location) string {
	var builder strings.Builder
	if usage.Plan != "" {
		fmt.Fprintf(&builder, "Plan: %s\n", usage.Plan)
	}
	fmt.Fprintf(&builder, "Weekly usage: %d%% used, %d%% left\n", usage.UsedPercent, usage.RemainingPercent)
	refreshTime := usage.RefreshAt.In(location).Format("2006-01-02 15:04:05")
	fmt.Fprintf(&builder, "Refresh date: %s (%s)\n", refreshTime, relativeDuration(now, usage.RefreshAt))
	return builder.String()
}

func relativeDuration(now time.Time, target time.Time) string {
	duration := target.Sub(now)
	if duration < 0 {
		duration = -duration
	}
	minutes := int(duration.Round(time.Minute) / time.Minute)
	if minutes < 60 {
		return fmt.Sprintf("%dmin", minutes)
	}
	hours := minutes / 60
	days := hours / 24
	remainingHours := hours % 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, remainingHours)
	}
	return fmt.Sprintf("%dh", hours)
}

func usageURL(baseURL string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	if trimmed == "" {
		trimmed = "https://chatgpt.com/backend-api"
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && strings.Contains(parsed.Path, "/backend-api") {
		return trimmed + "/wham/usage"
	}
	return trimmed + "/api/codex/usage"
}

func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfigPath
	}
	return home + "/.local/share/lenos/config.json"
}

func clampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
