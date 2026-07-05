package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/navidrome"
)

type rootOptions struct {
	config     string
	server     string
	username   string
	password   string
	client     string
	apiVersion string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(exitCode(err))
	}
}

func newRootCmd() *cobra.Command {
	var opts rootOptions
	root := &cobra.Command{
		Use:   "nd-playlist [command]",
		Short: "Manage Navidrome playlists from YAML specs",
	}
	root.SilenceUsage = true
	root.PersistentFlags().StringVar(&opts.config, "config", "", "Config file path")
	root.PersistentFlags().StringVar(&opts.server, "server", "", "Navidrome server URL")
	root.PersistentFlags().StringVar(&opts.username, "username", "", "Navidrome username")
	root.PersistentFlags().StringVar(&opts.password, "password", "", "Navidrome password")
	root.PersistentFlags().StringVar(&opts.client, "client", "", "Subsonic client name")
	root.PersistentFlags().StringVar(&opts.apiVersion, "api-version", "", "Subsonic API version")

	root.AddCommand(
		newPingCmd(&opts),
		newListCmd(&opts),
		newShowCmd(&opts),
		newResolveCmd(&opts),
		newDiffCmd(&opts),
		newApplyCmd(&opts),
		newExportCmd(&opts),
		newExportAllCmd(&opts),
	)
	return root
}

func newPingCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Validate Navidrome auth",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			if err := client.Ping(cmd.Context()); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "ok")
			return err
		},
	}
}

func newListCmd(opts *rootOptions) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List server playlists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			playlists, err := client.GetPlaylists(cmd.Context())
			if err != nil {
				return err
			}
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), playlists)
			}
			for _, playlist := range playlists {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", playlist.ID, playlist.Owner, playlist.Name)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	return cmd
}

func newShowCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "show <playlist>",
		Short: "Show a server playlist",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			playlist, songs, err := findServerPlaylist(cmd.Context(), client, cfg.Username, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", playlist.Name, playlist.ID)
			for i, song := range songs {
				fmt.Fprintf(cmd.OutOrStdout(), "%d. %s - %s\n", i+1, song.Artist, song.Title)
			}
			return nil
		},
	}
}

func newResolveCmd(opts *rootOptions) *cobra.Command {
	var jsonOut bool
	var allowFuzzy bool
	cmd := &cobra.Command{
		Use:   "resolve <playlist.yaml>",
		Short: "Resolve a YAML playlist to song IDs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			spec, err := readSpecFile(args[0])
			if err != nil {
				return err
			}
			result, err := navidrome.ResolveTracks(cmd.Context(), client, spec, allowFuzzy)
			if err != nil {
				return err
			}
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), result)
			}
			for _, song := range result.Songs {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", song.ID, song.Artist, song.Title)
			}
			for _, warning := range result.Warnings {
				fmt.Fprintf(cmd.OutOrStderr(), "warning: %s\n", warning)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	cmd.Flags().BoolVar(&allowFuzzy, "allow-fuzzy", false, "Accept one non-exact search result")
	return cmd
}

func newDiffCmd(opts *rootOptions) *cobra.Command {
	var jsonOut bool
	var allowFuzzy bool
	cmd := &cobra.Command{
		Use:   "diff <playlist.yaml>",
		Short: "Compare YAML playlist to server state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			spec, desired, err := resolveSpecFile(cmd.Context(), client, args[0], allowFuzzy)
			if err != nil {
				return err
			}
			playlist, current, err := currentPlaylist(cmd.Context(), client, cfg.Username, spec)
			if err != nil && !errors.Is(err, navidrome.ErrNotFound) {
				return err
			}
			diff := navidrome.DiffTracks(current, desired)
			if jsonOut {
				return writeJSON(cmd.OutOrStdout(), map[string]any{
					"playlist": playlist,
					"diff":     diff,
				})
			}
			printDiff(cmd, playlist, diff, errors.Is(err, navidrome.ErrNotFound))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Print JSON")
	cmd.Flags().BoolVar(&allowFuzzy, "allow-fuzzy", false, "Accept one non-exact search result")
	return cmd
}

func newApplyCmd(opts *rootOptions) *cobra.Command {
	var dryRun bool
	var yes bool
	var allowFuzzy bool
	cmd := &cobra.Command{
		Use:   "apply <playlist.yaml>...",
		Short: "Create or replace server playlists from YAML",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			for _, path := range args {
				if err := applyOne(cmd.Context(), cmd, client, cfg.Username, path, allowFuzzy, dryRun, yes); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without mutation")
	cmd.Flags().BoolVar(&yes, "yes", false, "Apply existing playlist replacement without prompt")
	cmd.Flags().BoolVar(&allowFuzzy, "allow-fuzzy", false, "Accept one non-exact search result")
	return cmd
}

func newExportCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "export <playlist>",
		Short: "Export a server playlist as YAML",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			playlist, songs, err := findServerPlaylist(cmd.Context(), client, cfg.Username, args[0])
			if err != nil {
				return err
			}
			return navidrome.WriteSpec(cmd.OutOrStdout(), specFromPlaylist(playlist, songs))
		},
	}
}

func newExportAllCmd(opts *rootOptions) *cobra.Command {
	var owner string
	var output string
	cmd := &cobra.Command{
		Use:   "export-all --output <dir>",
		Short: "Export server playlists to YAML files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cfg, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			if owner == "" {
				owner = cfg.Username
			}
			if output == "" {
				return fmt.Errorf("--output is required")
			}
			if err := os.MkdirAll(output, 0755); err != nil {
				return err
			}
			playlists, err := client.GetPlaylists(cmd.Context())
			if err != nil {
				return err
			}
			for _, playlist := range playlists {
				if owner != "" && playlist.Owner != "" && playlist.Owner != owner {
					continue
				}
				full, songs, err := client.GetPlaylist(cmd.Context(), playlist.ID)
				if err != nil {
					return err
				}
				path := filepath.Join(output, exportFilename(full)+".yaml")
				file, err := os.Create(path)
				if err != nil {
					return err
				}
				err = navidrome.WriteSpec(file, specFromPlaylist(full, songs))
				closeErr := file.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), path); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "Playlist owner to export")
	cmd.Flags().StringVar(&output, "output", "", "Output directory")
	return cmd
}

func configuredClient(opts rootOptions) (*navidrome.Client, navidrome.Config, error) {
	cfg, err := navidrome.LoadConfig(navidrome.ConfigOptions{
		Path:       opts.config,
		Server:     opts.server,
		Username:   opts.username,
		Password:   opts.password,
		Client:     opts.client,
		APIVersion: opts.apiVersion,
	})
	if err != nil {
		return nil, navidrome.Config{}, err
	}
	return navidrome.NewClient(cfg), cfg, nil
}

func readSpecFile(path string) (navidrome.PlaylistSpec, error) {
	file, err := os.Open(path)
	if err != nil {
		return navidrome.PlaylistSpec{}, err
	}
	defer file.Close()
	return navidrome.ReadSpec(file)
}

func resolveSpecFile(
	ctx context.Context,
	client *navidrome.Client,
	path string,
	allowFuzzy bool,
) (navidrome.PlaylistSpec, []navidrome.Song, error) {
	spec, err := readSpecFile(path)
	if err != nil {
		return navidrome.PlaylistSpec{}, nil, err
	}
	result, err := navidrome.ResolveTracks(ctx, client, spec, allowFuzzy)
	if err != nil {
		return navidrome.PlaylistSpec{}, nil, err
	}
	return spec, result.Songs, nil
}

func currentPlaylist(
	ctx context.Context,
	client *navidrome.Client,
	owner string,
	spec navidrome.PlaylistSpec,
) (navidrome.Playlist, []navidrome.Song, error) {
	playlists, err := client.GetPlaylists(ctx)
	if err != nil {
		return navidrome.Playlist{}, nil, err
	}
	playlist, err := navidrome.ChoosePlaylist(spec, playlists, owner)
	if err != nil {
		return navidrome.Playlist{}, nil, err
	}
	playlist, songs, err := client.GetPlaylist(ctx, playlist.ID)
	if err != nil {
		return navidrome.Playlist{}, nil, err
	}
	return playlist, songs, nil
}

func findServerPlaylist(
	ctx context.Context,
	client *navidrome.Client,
	owner string,
	nameOrID string,
) (navidrome.Playlist, []navidrome.Song, error) {
	spec := navidrome.PlaylistSpec{Name: nameOrID}
	playlists, err := client.GetPlaylists(ctx)
	if err != nil {
		return navidrome.Playlist{}, nil, err
	}
	for _, playlist := range playlists {
		if playlist.ID == nameOrID {
			return client.GetPlaylist(ctx, playlist.ID)
		}
	}
	playlist, err := navidrome.ChoosePlaylist(spec, playlists, owner)
	if err != nil {
		return navidrome.Playlist{}, nil, err
	}
	return client.GetPlaylist(ctx, playlist.ID)
}

func applyOne(
	ctx context.Context,
	cmd *cobra.Command,
	client *navidrome.Client,
	owner string,
	path string,
	allowFuzzy bool,
	dryRun bool,
	yes bool,
) error {
	spec, desired, err := resolveSpecFile(ctx, client, path, allowFuzzy)
	if err != nil {
		return err
	}
	playlist, current, err := currentPlaylist(ctx, client, owner, spec)
	missing := errors.Is(err, navidrome.ErrNotFound)
	if err != nil && !missing {
		return err
	}
	diff := navidrome.DiffTracks(current, desired)
	printDiff(cmd, playlist, diff, missing)
	if dryRun {
		return printDryRun(cmd, spec, playlist, desired, missing)
	}
	if !missing && !yes && !isCI() {
		return fmt.Errorf("refusing to replace existing playlist %q without --yes", playlist.Name)
	}

	if err := applyResolved(ctx, client, owner, spec, playlist, desired, missing); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "applied playlist %q with %d tracks\n", spec.Name, len(desired))
	return nil
}

func applyResolved(
	ctx context.Context,
	client *navidrome.Client,
	owner string,
	spec navidrome.PlaylistSpec,
	playlist navidrome.Playlist,
	desired []navidrome.Song,
	missing bool,
) error {
	ids := make([]string, 0, len(desired))
	for _, song := range desired {
		ids = append(ids, song.ID)
	}
	playlistID := ""
	if !missing {
		playlistID = playlist.ID
	}
	if err := client.CreateOrReplacePlaylist(ctx, playlistID, spec.Name, ids); err != nil {
		return fmt.Errorf("%w: %v", navidrome.ErrMutationRejected, err)
	}
	return updateMetadata(ctx, client, owner, spec, playlistID)
}

func updateMetadata(
	ctx context.Context,
	client *navidrome.Client,
	owner string,
	spec navidrome.PlaylistSpec,
	playlistID string,
) error {
	if spec.Public != nil || spec.Comment != "" {
		targetID := playlistID
		if targetID == "" {
			refreshed, err := client.GetPlaylists(ctx)
			if err != nil {
				return err
			}
			created, err := navidrome.ChoosePlaylist(spec, refreshed, owner)
			if err != nil {
				return err
			}
			targetID = created.ID
		}
		if err := client.UpdatePlaylistMetadata(ctx, targetID, spec.Public, spec.Comment); err != nil {
			return fmt.Errorf("%w: %v", navidrome.ErrMutationRejected, err)
		}
	}
	return nil
}

func printDryRun(
	cmd *cobra.Command,
	spec navidrome.PlaylistSpec,
	playlist navidrome.Playlist,
	desired []navidrome.Song,
	missing bool,
) error {
	if missing {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "would create playlist %q with %d tracks\n", spec.Name, len(desired))
		return err
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "would replace playlist %q with %d tracks\n", playlist.Name, len(desired))
	return err
}

func printDiff(cmd *cobra.Command, playlist navidrome.Playlist, diff navidrome.TrackDiff, missing bool) {
	if missing {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "playlist does not exist; create will replace no existing tracks")
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "playlist %q (%s): replacement semantics\n", playlist.Name, playlist.ID)
	fmt.Fprintf(cmd.OutOrStdout(), "added=%d removed=%d reordered=%d\n", diff.Added, diff.Removed, diff.Reordered)
}

func specFromPlaylist(playlist navidrome.Playlist, songs []navidrome.Song) navidrome.PlaylistSpec {
	tracks := make([]navidrome.TrackSpec, 0, len(songs))
	for _, song := range songs {
		tracks = append(tracks, navidrome.TrackSpec(song))
	}
	public := playlist.Public
	return navidrome.PlaylistSpec{
		Name:        playlist.Name,
		NavidromeID: playlist.ID,
		Comment:     playlist.Comment,
		Public:      &public,
		Tracks:      tracks,
	}
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func slug(name string) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func exportFilename(playlist navidrome.Playlist) string {
	name := slug(playlist.Name)
	if name != "" {
		return name
	}
	return "playlist-" + playlist.ID
}

func isCI() bool {
	return os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("WOODPECKER") != ""
}

func exitCode(err error) int {
	var missing navidrome.MissingTracksError
	if errors.As(err, &missing) {
		return 3
	}
	var ambiguous navidrome.AmbiguousTracksError
	if errors.As(err, &ambiguous) {
		return 4
	}
	if errors.Is(err, navidrome.ErrConfig) {
		return 2
	}
	var apiErr navidrome.APIError
	if errors.As(err, &apiErr) && (apiErr.Code == 40 || apiErr.Code == 41) {
		return 2
	}
	if errors.Is(err, navidrome.ErrMutationRejected) {
		return 5
	}
	return 1
}
