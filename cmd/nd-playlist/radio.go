package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/navidrome"
)

func newRadioCmd(opts *rootOptions) *cobra.Command {
	radio := &cobra.Command{
		Use:   "radio",
		Short: "Manage Navidrome internet radio stations",
	}
	radio.AddCommand(
		newRadioListCmd(opts),
		newRadioDiffCmd(opts),
		newRadioApplyCmd(opts),
		newRadioExportCmd(opts),
	)
	return radio
}

func newRadioListCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Navidrome internet radio stations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			stations, err := client.GetInternetRadioStations(cmd.Context())
			if err != nil {
				return err
			}
			for _, station := range stations {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", station.Name, station.StreamURL); err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func newRadioDiffCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "diff <stations.yaml>",
		Short: "Compare a radio YAML spec with Navidrome",
		Long:  helpRadioDiff,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			spec, err := readRadioSpecFile(args[0])
			if err != nil {
				return err
			}
			existing, err := client.GetInternetRadioStations(cmd.Context())
			if err != nil {
				return err
			}
			_, err = printRadioDiff(cmd, spec.Stations, existing)
			return err
		},
	}
}

func newRadioApplyCmd(opts *rootOptions) *cobra.Command {
	var dryRun bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "apply <stations.yaml>",
		Short: "Create missing radio stations from YAML",
		Long:  helpRadioApply,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			spec, err := readRadioSpecFile(args[0])
			if err != nil {
				return err
			}
			existing, err := client.GetInternetRadioStations(cmd.Context())
			if err != nil {
				return err
			}
			missing, err := printRadioDiff(cmd, spec.Stations, existing)
			if err != nil || dryRun || len(missing) == 0 {
				return err
			}
			if !yes && !isCI() {
				return errors.New("refusing to create radio stations without --yes")
			}
			for _, station := range missing {
				if err := client.CreateInternetRadioStation(cmd.Context(), station); err != nil {
					return fmt.Errorf("add %s: %w", station.Name, err)
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "added\t%s\n", station.Name); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without mutation")
	cmd.Flags().BoolVar(&yes, "yes", false, "Create missing stations")
	return cmd
}

func newRadioExportCmd(opts *rootOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "export",
		Short: "Export Navidrome internet radio stations as YAML",
		Long:  helpRadioExport,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			stations, err := client.GetInternetRadioStations(cmd.Context())
			if err != nil {
				return err
			}
			return navidrome.WriteRadioSpec(cmd.OutOrStdout(), navidrome.RadioSpec{Stations: stations})
		},
	}
}

func readRadioSpecFile(path string) (navidrome.RadioSpec, error) {
	file, err := os.Open(path)
	if err != nil {
		return navidrome.RadioSpec{}, err
	}
	defer file.Close()
	return navidrome.ReadRadioSpec(file)
}

func printRadioDiff(
	cmd *cobra.Command,
	desired, existing []navidrome.RadioStation,
) ([]navidrome.RadioStation, error) {
	known := make(map[string]struct{}, len(existing))
	for _, station := range existing {
		known[station.StreamURL] = struct{}{}
	}
	missing := make([]navidrome.RadioStation, 0, len(desired))
	for _, station := range desired {
		if _, found := known[station.StreamURL]; found {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "exists\t%s\n", station.Name); err != nil {
				return nil, err
			}
			continue
		}
		missing = append(missing, station)
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "would add\t%s\n", station.Name); err != nil {
			return nil, err
		}
	}
	return missing, nil
}
