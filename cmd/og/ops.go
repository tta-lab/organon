package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/config"
	"github.com/tta-lab/organon/internal/project"
)

const (
	providerGitHub  = "github"
	providerForgejo = "forgejo"
	cmdStatus       = "status"
	branchMain      = "main"
	branchMaster    = "master"
	osDarwin        = "darwin"
	osLinux         = "linux"
	stateAll        = "all"
)

var (
	semverTagRe = regexp.MustCompile(
		`^v\d+\.\d+\.\d+(-[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)
	semverTagBaseRe = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)(\+[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*)?$`)
)

type repoContext struct {
	WorkDir      string
	ProjectAlias string
	Provider     string
	Host         string
	Owner        string
	Repo         string
	RemoteURL    string
	TokenEnv     string
	Token        string
	DefaultBase  string
	Branch       string
}

type pullRequest struct {
	Index   int64  `json:"index"`
	Number  int64  `json:"number,omitempty"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Merged  bool   `json:"merged"`
	URL     string `json:"url"`
	HTMLURL string `json:"html_url,omitempty"`
	Head    string `json:"head"`
	Base    string `json:"base"`
	Body    string `json:"body"`
	SHA     string `json:"head_sha,omitempty"`
}

type daemonRequest struct {
	WorkDir string `json:"work_dir"`
	Force   bool   `json:"force,omitempty"`
	Tag     string `json:"tag,omitempty"`
	Bump    string `json:"bump,omitempty"`
	Title   string `json:"title,omitempty"`
	Body    string `json:"body,omitempty"`
	Index   int64  `json:"index,omitempty"`
	State   string `json:"state,omitempty"`
	Tail    int    `json:"tail,omitempty"`

	// These fields intentionally remain unused by the CLI; tests assert they are not populated there.
	Token    string `json:"token,omitempty"`
	TokenEnv string `json:"token_env,omitempty"`
}

type daemonResponse struct {
	OK      bool         `json:"ok"`
	Error   string       `json:"error,omitempty"`
	Message string       `json:"message,omitempty"`
	PR      *pullRequest `json:"pr,omitempty"`
	Lines   []string     `json:"lines,omitempty"`
}

func runGitPush(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	force, _ := cmd.Flags().GetBool("force")
	resp, err := daemonCall("/git/push", daemonRequest{WorkDir: workDir, Force: force})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runGitPull(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	resp, err := daemonCall("/git/pull", daemonRequest{WorkDir: workDir})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runGitTag(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	bump, _ := cmd.Flags().GetString("bump")
	if bump != "" && len(args) > 0 {
		return fmt.Errorf("--bump and a positional version are mutually exclusive")
	}
	tag := ""
	if len(args) > 0 {
		tag = args[0]
	}
	resp, err := daemonCall("/git/tag", daemonRequest{WorkDir: workDir, Tag: tag, Bump: bump})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRCreate(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	body, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR body: %w", err)
	}
	title := strings.Join(args, " ")
	resp, err := daemonCall("/pr/create", daemonRequest{
		WorkDir: workDir,
		Title:   title,
		Body:    strings.TrimRight(string(body), "\n"),
	})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRView(cmd *cobra.Command, args []string) error {
	return runPRDaemonWithOutput(cmd, "/pr/view", daemonRequest{State: stateAll})
}

func runPRFind(cmd *cobra.Command, args []string) error {
	state, _ := cmd.Flags().GetString("state")
	return runPRDaemonWithOutput(cmd, "/pr/find", daemonRequest{State: state})
}

func runPRGet(cmd *cobra.Command, args []string) error {
	index, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid PR index %q: %w", args[0], err)
	}
	return runPRDaemonWithOutput(cmd, "/pr/get", daemonRequest{Index: index})
}

func runPRModify(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	title, _ := cmd.Flags().GetString("title")
	bodyBytes, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR body: %w", err)
	}
	body := strings.TrimRight(string(bodyBytes), "\n")
	if title == "" && body == "" {
		return fmt.Errorf("nothing to update: provide --title and/or pipe body via stdin")
	}
	prID, _ := cmd.Flags().GetString("pr-id")
	var index int64
	if prID != "" {
		index, err = strconv.ParseInt(prID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid --pr-id %q: %w", prID, err)
		}
	}
	resp, err := daemonCall("/pr/modify", daemonRequest{WorkDir: workDir, Index: index, Title: title, Body: body})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRComment(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	bodyBytes, err := io.ReadAll(cmd.InOrStdin())
	if err != nil {
		return fmt.Errorf("read PR comment: %w", err)
	}
	body := strings.TrimSpace(string(bodyBytes))
	if body == "" {
		return fmt.Errorf("comment body is required on stdin")
	}
	resp, err := daemonCall("/pr/comment", daemonRequest{WorkDir: workDir, Body: body})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRChecks(cmd *cobra.Command, args []string) error {
	return runLinesDaemon(cmd, "/pr/checks", daemonRequest{State: stateAll})
}

func runPRFailures(cmd *cobra.Command, args []string) error {
	tail, _ := cmd.Flags().GetInt("tail")
	return runLinesDaemon(cmd, "/pr/failures", daemonRequest{State: stateAll, Tail: tail})
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	resp, err := daemonCall("/auth/status", daemonRequest{WorkDir: workDir})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPolicyExplain(cmd *cobra.Command, args []string) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	resp, err := daemonCall("/policy/explain", daemonRequest{WorkDir: workDir})
	if err != nil {
		return err
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runPRDaemonWithOutput(cmd *cobra.Command, path string, req daemonRequest) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	req.WorkDir = workDir
	resp, err := daemonCall(path, req)
	if err != nil {
		return err
	}
	if resp.PR != nil {
		return printPR(cmd, resp.PR)
	}
	printDaemonResponse(cmd, resp)
	return nil
}

func runLinesDaemon(cmd *cobra.Command, path string, req daemonRequest) error {
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	req.WorkDir = workDir
	resp, err := daemonCall(path, req)
	if err != nil {
		return err
	}
	if len(resp.Lines) == 0 {
		printDaemonResponse(cmd, resp)
		return nil
	}
	for _, line := range resp.Lines {
		cmd.Println(line)
	}
	return nil
}

func daemonCall(path string, req daemonRequest) (daemonResponse, error) {
	base, client := daemonHTTPClient()
	data, err := json.Marshal(req)
	if err != nil {
		return daemonResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		strings.TrimRight(base, "/")+path,
		bytes.NewReader(data),
	)
	if err != nil {
		return daemonResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return daemonResponse{}, fmt.Errorf("daemon call %s: %w", path, err)
	}
	defer httpResp.Body.Close()
	var resp daemonResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return daemonResponse{}, fmt.Errorf("decode daemon response: %w", err)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 || !resp.OK {
		if resp.Error == "" {
			resp.Error = httpResp.Status
		}
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

func daemonHTTPClient() (string, *http.Client) {
	if base := os.Getenv("OG_DAEMON_URL"); base != "" {
		return strings.TrimRight(base, "/"), &http.Client{Timeout: 60 * time.Second}
	}
	socketPath := daemonSocketPath()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return "http://og", &http.Client{Timeout: 60 * time.Second, Transport: transport}
}

func daemonSocketPath() string {
	if path := os.Getenv("OG_DAEMON_SOCKET"); path != "" {
		return path
	}
	return filepath.Join(config.DefaultConfigDir(), "og.sock")
}

func printDaemonResponse(cmd *cobra.Command, resp daemonResponse) {
	if resp.Message != "" {
		cmd.Println(resp.Message)
		return
	}
	if resp.PR != nil {
		_ = printPR(cmd, resp.PR)
		return
	}
	for _, line := range resp.Lines {
		cmd.Println(line)
	}
}

func resolveRepoContextFor(workDir string) (*repoContext, error) {
	if workDir == "" {
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}
	root, err := gitOutput(workDir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}
	e, err := project.GetByPath(config.ProjectsPath(), root)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("workdir %q is not inside a registered project", root)
	}
	remote, err := gitOutput(root, "remote", "get-url", "origin")
	if err != nil {
		return nil, fmt.Errorf("get origin remote: %w", err)
	}
	provider, host, owner, repo, err := parseRemote(remote)
	if err != nil {
		return nil, err
	}
	branch, err := gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("get current branch: %w", err)
	}
	if branch == "HEAD" || branch == "" {
		return nil, fmt.Errorf("not on a named branch")
	}
	base := defaultBranch(root)
	tokenEnv := tokenEnvFor(provider, e)
	token := ""
	if tokenEnv != "" {
		token = os.Getenv(tokenEnv)
	}
	return &repoContext{
		WorkDir:      root,
		ProjectAlias: e.Alias,
		Provider:     provider,
		Host:         host,
		Owner:        owner,
		Repo:         repo,
		RemoteURL:    remote,
		TokenEnv:     tokenEnv,
		Token:        token,
		DefaultBase:  base,
		Branch:       branch,
	}, nil
}

func parseRemote(remote string) (provider, host, owner, repo string, err error) {
	remote = strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	if strings.HasPrefix(remote, "git@") {
		parts := strings.SplitN(strings.TrimPrefix(remote, "git@"), ":", 2)
		if len(parts) != 2 {
			return "", "", "", "", fmt.Errorf("unsupported git remote %q", remote)
		}
		host = parts[0]
		path := strings.Split(parts[1], "/")
		if len(path) != 2 {
			return "", "", "", "", fmt.Errorf("unsupported git remote %q", remote)
		}
		owner, repo = path[0], path[1]
	} else {
		u, parseErr := url.Parse(remote)
		if parseErr != nil || u.Host == "" {
			return "", "", "", "", fmt.Errorf("unsupported git remote %q", remote)
		}
		host = u.Host
		path := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(path) < 2 {
			return "", "", "", "", fmt.Errorf("unsupported git remote %q", remote)
		}
		owner, repo = path[len(path)-2], path[len(path)-1]
	}
	provider = providerForgejo
	if host == "github.com" {
		provider = providerGitHub
	}
	return provider, host, owner, repo, nil
}

func tokenEnvFor(provider string, e *project.Entry) string {
	if e != nil && e.GitHubTokenEnv != "" {
		return e.GitHubTokenEnv
	}
	if provider == providerGitHub {
		for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
			if os.Getenv(name) != "" {
				return name
			}
		}
		return "GITHUB_TOKEN"
	}
	for _, name := range []string{"FORGEJO_TOKEN", "GITEA_TOKEN"} {
		if os.Getenv(name) != "" {
			return name
		}
	}
	return "FORGEJO_TOKEN"
}

func gitOutput(workDir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func runGit(workDir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", workDir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runGitWithCreds(ctxInfo *repoContext, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", ctxInfo.WorkDir}, args...)...)
	cmd.Env = os.Environ()
	if ctxInfo.Token != "" {
		cmd.Env = append(cmd.Env, gitCredentialEnv(ctxInfo)...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func gitCredentialEnv(ctx *repoContext) []string {
	prefix := "https://" + ctx.Host + "/"
	return []string{
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=url.https://x-access-token:" + ctx.Token + "@" + ctx.Host + "/.insteadOf",
		"GIT_CONFIG_VALUE_0=" + prefix,
		"GIT_CONFIG_KEY_1=url.https://oauth2:" + ctx.Token + "@" + ctx.Host + "/.insteadOf",
		"GIT_CONFIG_VALUE_1=" + prefix,
	}
}

func defaultBranch(workDir string) string {
	out, err := gitOutput(workDir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		if _, branch, ok := strings.Cut(out, "origin/"); ok && branch != "" {
			return branch
		}
	}
	return branchMain
}

func latestTag(workDir string) (string, error) {
	out, err := gitOutput(workDir, "tag", "--sort=-version:refname")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line, nil
		}
	}
	return "", nil
}

func computeBumpedTag(workDir, level string) (string, error) {
	latest, err := latestTag(workDir)
	if err != nil {
		return "", err
	}
	if latest == "" {
		switch level {
		case "major":
			return "v1.0.0", nil
		case "minor":
			return "v0.1.0", nil
		case "patch":
			return "v0.0.1", nil
		default:
			return "", fmt.Errorf("invalid --bump value %q", level)
		}
	}
	m := semverTagBaseRe.FindStringSubmatch(latest)
	if m == nil {
		return "", fmt.Errorf("latest tag %q is not a plain semver tag", latest)
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	pat, _ := strconv.Atoi(m[3])
	suffix := m[4]
	switch level {
	case "major":
		maj++
		min = 0
		pat = 0
	case "minor":
		min++
		pat = 0
	case "patch":
		pat++
	default:
		return "", fmt.Errorf("invalid --bump value %q", level)
	}
	return fmt.Sprintf("v%d.%d.%d%s", maj, min, pat, suffix), nil
}

func localTagExists(workDir, tag string) bool {
	err := runGit(workDir, "show-ref", "--verify", "--quiet", "refs/tags/"+tag)
	return err == nil
}

func daemonGitPush(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	if ctx.Branch == branchMain || ctx.Branch == branchMaster {
		return daemonResponse{}, fmt.Errorf("refusing to push protected branch %q", ctx.Branch)
	}
	gitArgs := []string{"push", "-u", "origin", ctx.Branch}
	if req.Force {
		gitArgs = append(gitArgs, "--force-with-lease")
	}
	if err := runGitWithCreds(ctx, gitArgs...); err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, Message: fmt.Sprintf("Pushed %s -> origin/%s", ctx.Branch, ctx.Branch)}, nil
}

func daemonGitPull(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	target := ctx.Branch
	if ctx.Branch == ctx.DefaultBase {
		target = ctx.DefaultBase
	}
	if err := runGitWithCreds(ctx, "pull", "--ff-only", "origin", target); err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, Message: "Pulled " + target}, nil
}

func daemonGitTag(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	if req.Bump != "" && req.Tag != "" {
		return daemonResponse{}, fmt.Errorf("--bump and a positional version are mutually exclusive")
	}
	tag := req.Tag
	if req.Bump != "" {
		tag, err = computeBumpedTag(ctx.WorkDir, req.Bump)
		if err != nil {
			return daemonResponse{}, err
		}
	}
	if tag == "" {
		return daemonResponse{}, fmt.Errorf("either a version argument or --bump is required")
	}
	if !semverTagRe.MatchString(tag) {
		return daemonResponse{}, fmt.Errorf("invalid semver tag %q", tag)
	}
	if !localTagExists(ctx.WorkDir, tag) {
		if err := runGit(ctx.WorkDir, "tag", "--", tag); err != nil {
			return daemonResponse{}, err
		}
	}
	if err := runGitWithCreds(ctx, "push", "origin", "--", tag); err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, Message: fmt.Sprintf("Tagged %s -> pushed to origin", tag)}, nil
}

func daemonPRCreate(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	pr, err := createPR(ctx, req.Title, req.Body)
	if err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, Message: fmt.Sprintf("PR #%d created: %s", pr.Index, displayPRURL(pr)), PR: pr}, nil
}

func daemonPRView(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	pr, err := findPR(ctx, "all")
	if err != nil {
		return daemonResponse{}, err
	}
	full, err := getPR(ctx, pr.Index)
	if err == nil {
		pr = full
	}
	return daemonResponse{OK: true, PR: pr}, nil
}

func daemonPRFind(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	pr, err := findPR(ctx, req.State)
	if err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, PR: pr}, nil
}

func daemonPRGet(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	pr, err := getPR(ctx, req.Index)
	if err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, PR: pr}, nil
}

func daemonPRModify(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	index := req.Index
	if index == 0 {
		pr, err := findPR(ctx, "all")
		if err != nil {
			return daemonResponse{}, err
		}
		index = pr.Index
	}
	pr, err := updatePR(ctx, index, req.Title, req.Body)
	if err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, Message: fmt.Sprintf("PR #%d updated: %s", pr.Index, displayPRURL(pr)), PR: pr}, nil
}

func daemonPRComment(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	pr, err := findPR(ctx, "all")
	if err != nil {
		return daemonResponse{}, err
	}
	if err := commentPR(ctx, pr.Index, req.Body); err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, Message: fmt.Sprintf("Commented on PR #%d", pr.Index)}, nil
}

func daemonPRChecks(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	pr, err := findPR(ctx, "all")
	if err != nil {
		return daemonResponse{}, err
	}
	lines, err := getChecks(ctx, pr)
	if err != nil {
		return daemonResponse{}, err
	}
	if len(lines) == 0 {
		lines = []string{"No checks found."}
	}
	return daemonResponse{OK: true, Lines: lines}, nil
}

func daemonPRFailures(req daemonRequest) (daemonResponse, error) {
	resp, err := daemonPRChecks(req)
	if err != nil {
		return daemonResponse{}, err
	}
	var failures []string
	for _, line := range resp.Lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "failure") || strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
			failures = append(failures, line)
		}
	}
	if len(failures) == 0 {
		failures = []string{"No failing checks found."}
	}
	return daemonResponse{OK: true, Lines: failures}, nil
}

func daemonAuthStatus(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	status := "unset"
	if ctx.Token != "" {
		status = "set"
	}
	return daemonResponse{OK: true, Message: fmt.Sprintf(
		"provider: %s\nhost: %s\nrepo: %s/%s\nproject: %s\ntoken_env: %s (%s)",
		ctx.Provider, ctx.Host, ctx.Owner, ctx.Repo, ctx.ProjectAlias, ctx.TokenEnv, status,
	)}, nil
}

func daemonPolicyExplain(req daemonRequest) (daemonResponse, error) {
	ctx, err := resolveRepoContextFor(req.WorkDir)
	if err != nil {
		return daemonResponse{}, err
	}
	return daemonResponse{OK: true, Message: fmt.Sprintf(
		"repo: %s/%s\nworkdir: %s\nregistered_project: true\n"+
			"protected_branch: %t\narbitrary_git_args: false\narbitrary_api_paths: false",
		ctx.Owner, ctx.Repo, ctx.WorkDir, ctx.Branch == branchMain || ctx.Branch == branchMaster,
	)}, nil
}

func createPR(ctx *repoContext, title, body string) (*pullRequest, error) {
	if err := requireToken(ctx); err != nil {
		return nil, err
	}
	payload := map[string]string{"title": title, "body": body, "head": ctx.Branch, "base": ctx.DefaultBase}
	var out map[string]any
	if err := forgeJSON(ctx, http.MethodPost, prAPIBase(ctx), payload, &out); err != nil {
		return nil, err
	}
	return mapPR(out), nil
}

func findPR(ctx *repoContext, state string) (*pullRequest, error) {
	if err := requireToken(ctx); err != nil {
		return nil, err
	}
	if state == "" {
		state = "open"
	}
	query := url.Values{}
	query.Set("state", state)
	query.Set("head", prHead(ctx))
	query.Set("base", ctx.DefaultBase)
	endpoint := prAPIBase(ctx) + "?" + query.Encode()
	var list []map[string]any
	if err := forgeJSON(ctx, http.MethodGet, endpoint, nil, &list); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("no PR found for branch %s", ctx.Branch)
	}
	return mapPR(list[0]), nil
}

func getPR(ctx *repoContext, index int64) (*pullRequest, error) {
	if err := requireToken(ctx); err != nil {
		return nil, err
	}
	var out map[string]any
	if err := forgeJSON(ctx, http.MethodGet, prAPIBase(ctx)+"/"+strconv.FormatInt(index, 10), nil, &out); err != nil {
		return nil, err
	}
	return mapPR(out), nil
}

func updatePR(ctx *repoContext, index int64, title, body string) (*pullRequest, error) {
	if err := requireToken(ctx); err != nil {
		return nil, err
	}
	payload := map[string]string{}
	if title != "" {
		payload["title"] = title
	}
	if body != "" {
		payload["body"] = body
	}
	var out map[string]any
	method := http.MethodPatch
	if ctx.Provider == providerForgejo {
		method = http.MethodPatch
	}
	if err := forgeJSON(ctx, method, prAPIBase(ctx)+"/"+strconv.FormatInt(index, 10), payload, &out); err != nil {
		return nil, err
	}
	return mapPR(out), nil
}

func commentPR(ctx *repoContext, index int64, body string) error {
	if err := requireToken(ctx); err != nil {
		return err
	}
	endpoint := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/issues/%d/comments", ctx.Host, ctx.Owner, ctx.Repo, index)
	if ctx.Provider == providerGitHub {
		endpoint = fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", ctx.Owner, ctx.Repo, index)
	}
	var out map[string]any
	return forgeJSON(ctx, http.MethodPost, endpoint, map[string]string{"body": body}, &out)
}

func getChecks(ctx *repoContext, pr *pullRequest) ([]string, error) {
	if err := requireToken(ctx); err != nil {
		return nil, err
	}
	sha := pr.SHA
	if sha == "" {
		sha = "HEAD"
	}
	if ctx.Provider == providerGitHub {
		var out map[string]any
		endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s/status", ctx.Owner, ctx.Repo, sha)
		if err := forgeJSON(ctx, http.MethodGet, endpoint, nil, &out); err != nil {
			return nil, err
		}
		var lines []string
		lines = append(lines, fmt.Sprintf("combined: %v", out["state"]))
		if statuses, ok := out["statuses"].([]any); ok {
			for _, item := range statuses {
				if m, ok := item.(map[string]any); ok {
					lines = append(lines, fmt.Sprintf("%v: %v - %v", m["context"], m["state"], m["description"]))
				}
			}
		}
		return lines, nil
	}
	var statuses []map[string]any
	endpoint := fmt.Sprintf("https://%s/api/v1/repos/%s/%s/statuses/%s", ctx.Host, ctx.Owner, ctx.Repo, sha)
	if err := forgeJSON(ctx, http.MethodGet, endpoint, nil, &statuses); err != nil {
		return nil, err
	}
	lines := make([]string, 0, len(statuses))
	for _, status := range statuses {
		lines = append(lines, fmt.Sprintf("%v: %v - %v", status["context"], status["state"], status["description"]))
	}
	return lines, nil
}

func forgeJSON(ctxInfo *repoContext, method, endpoint string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if ctxInfo.Provider == providerGitHub {
		req.Header.Set("Authorization", "Bearer "+ctxInfo.Token)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	} else {
		req.Header.Set("Authorization", "token "+ctxInfo.Token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s failed: %s: %s", method, endpoint, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func prAPIBase(ctx *repoContext) string {
	if ctx.Provider == providerGitHub {
		return fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", ctx.Owner, ctx.Repo)
	}
	return fmt.Sprintf("https://%s/api/v1/repos/%s/%s/pulls", ctx.Host, ctx.Owner, ctx.Repo)
}

func prHead(ctx *repoContext) string {
	if ctx.Provider == providerGitHub {
		return ctx.Owner + ":" + ctx.Branch
	}
	return ctx.Branch
}

func requireToken(ctx *repoContext) error {
	if ctx.Token == "" {
		return fmt.Errorf("missing token: set %s", ctx.TokenEnv)
	}
	return nil
}

func mapPR(raw map[string]any) *pullRequest {
	pr := &pullRequest{}
	pr.Index = intField(raw, "number", "index")
	pr.Number = pr.Index
	pr.Title = stringField(raw, "title")
	pr.State = stringField(raw, "state")
	pr.Merged = boolField(raw, "merged")
	pr.HTMLURL = stringField(raw, "html_url", "url")
	pr.URL = pr.HTMLURL
	pr.Body = stringField(raw, "body")
	if head, ok := raw["head"].(map[string]any); ok {
		pr.Head = stringField(head, "ref")
		pr.SHA = stringField(head, "sha")
	}
	if base, ok := raw["base"].(map[string]any); ok {
		pr.Base = stringField(base, "ref")
	}
	return pr
}

func printPR(cmd *cobra.Command, pr *pullRequest) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	if jsonOut {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(pr)
	}
	cmd.Printf("PR #%d  %s  [%s]\n", pr.Index, pr.Title, pr.State)
	if displayPRURL(pr) != "" {
		cmd.Printf("  %s\n", displayPRURL(pr))
	}
	if pr.Head != "" || pr.Base != "" {
		cmd.Printf("  %s -> %s\n", pr.Head, pr.Base)
	}
	return nil
}

func displayPRURL(pr *pullRequest) string {
	if pr.HTMLURL != "" {
		return pr.HTMLURL
	}
	return pr.URL
}

func stringField(m map[string]any, names ...string) string {
	for _, name := range names {
		if v, ok := m[name].(string); ok {
			return v
		}
	}
	return ""
}

func intField(m map[string]any, names ...string) int64 {
	for _, name := range names {
		switch v := m[name].(type) {
		case float64:
			return int64(v)
		case int64:
			return v
		}
	}
	return 0
}

func boolField(m map[string]any, name string) bool {
	v, _ := m[name].(bool)
	return v
}

func runDaemonRun(cmd *cobra.Command, args []string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/git/push", daemonHTTPHandler(daemonGitPush))
	mux.HandleFunc("/git/pull", daemonHTTPHandler(daemonGitPull))
	mux.HandleFunc("/git/tag", daemonHTTPHandler(daemonGitTag))
	mux.HandleFunc("/pr/create", daemonHTTPHandler(daemonPRCreate))
	mux.HandleFunc("/pr/view", daemonHTTPHandler(daemonPRView))
	mux.HandleFunc("/pr/find", daemonHTTPHandler(daemonPRFind))
	mux.HandleFunc("/pr/get", daemonHTTPHandler(daemonPRGet))
	mux.HandleFunc("/pr/modify", daemonHTTPHandler(daemonPRModify))
	mux.HandleFunc("/pr/comment", daemonHTTPHandler(daemonPRComment))
	mux.HandleFunc("/pr/checks", daemonHTTPHandler(daemonPRChecks))
	mux.HandleFunc("/pr/failures", daemonHTTPHandler(daemonPRFailures))
	mux.HandleFunc("/auth/status", daemonHTTPHandler(daemonAuthStatus))
	mux.HandleFunc("/policy/explain", daemonHTTPHandler(daemonPolicyExplain))
	socketPath := daemonSocketPath()
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	cmd.Printf("og daemon listening on unix://%s\n", socketPath)
	return http.Serve(listener, mux)
}

func daemonHTTPHandler(fn func(daemonRequest) (daemonResponse, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(daemonResponse{Error: "method not allowed"})
			return
		}
		var req daemonRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(daemonResponse{Error: "decode request: " + err.Error()})
			return
		}
		resp, err := fn(req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(daemonResponse{Error: err.Error()})
			return
		}
		resp.OK = true
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func runDaemonInstall(cmd *cobra.Command, args []string) error {
	switch runtime.GOOS {
	case osDarwin:
		path, err := writeLaunchdPlist()
		if err != nil {
			return err
		}
		cmd.Printf("Installed launchd plist: %s\n", path)
		return nil
	case osLinux:
		path, err := writeSystemdService()
		if err != nil {
			return err
		}
		cmd.Printf("Installed systemd user service: %s\n", path)
		return nil
	default:
		return fmt.Errorf("daemon install is unsupported on %s", runtime.GOOS)
	}
}

func runDaemonUninstall(cmd *cobra.Command, args []string) error {
	switch runtime.GOOS {
	case osDarwin:
		return os.Remove(launchdPlistPath())
	case osLinux:
		return os.Remove(systemdServicePath())
	default:
		return fmt.Errorf("daemon uninstall is unsupported on %s", runtime.GOOS)
	}
}

func runDaemonStart(cmd *cobra.Command, args []string) error {
	return runServiceCommand("start")
}

func runDaemonStop(cmd *cobra.Command, args []string) error {
	return runServiceCommand("stop")
}

func runDaemonRestart(cmd *cobra.Command, args []string) error {
	if err := runServiceCommand("stop"); err != nil {
		return err
	}
	return runServiceCommand("start")
}

func runDaemonStatus(cmd *cobra.Command, args []string) error {
	resp, err := daemonHealth()
	if err != nil {
		cmd.Println("Daemon: not running")
		return nil
	}
	if resp.StatusCode == http.StatusOK {
		cmd.Println("Daemon: running")
		return nil
	}
	cmd.Printf("Daemon: unhealthy (%s)\n", resp.Status)
	return nil
}

func runDaemonHealth(cmd *cobra.Command, args []string) error {
	resp, err := daemonHealth()
	if err != nil {
		return fmt.Errorf("daemon health: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon health: %s", resp.Status)
	}
	cmd.Println("ok")
	return nil
}

func daemonHealth() (*http.Response, error) {
	base, client := daemonHTTPClient()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	_ = resp.Body.Close()
	return resp, nil
}

func writeLaunchdPlist() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := launchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>io.guion.og.daemon</string>
<key>ProgramArguments</key><array><string>%s</string><string>daemon</string><string>run</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><true/>
</dict></plist>
`, exe)
	return path, os.WriteFile(path, []byte(content), 0644)
}

func writeSystemdService() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	path := systemdServicePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	content := fmt.Sprintf(`[Unit]
Description=og daemon

[Service]
ExecStart=%s daemon run
Restart=always

[Install]
WantedBy=default.target
`, exe)
	return path, os.WriteFile(path, []byte(content), 0644)
}

func runServiceCommand(action string) error {
	switch runtime.GOOS {
	case osDarwin:
		verb := map[string]string{"start": "bootstrap", "stop": "bootout"}[action]
		if verb == "" {
			return errors.New("unsupported launchd action")
		}
		target := "gui/" + strconv.Itoa(os.Getuid())
		args := []string{verb, target, launchdPlistPath()}
		return runCommand("launchctl", args...)
	case osLinux:
		return runCommand("systemctl", "--user", action, "og.service")
	default:
		return fmt.Errorf("daemon %s is unsupported on %s", action, runtime.GOOS)
	}
}

func runCommand(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "io.guion.og.daemon.plist")
}

func systemdServicePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "og.service")
}
