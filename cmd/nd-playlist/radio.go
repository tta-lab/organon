package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tta-lab/organon/internal/navidrome"
)

const cliampHomePageURL = "https://www.cliamp.stream/"

var cliampStations = []navidrome.RadioStation{
	{
		Name: "Cliamp Lofi", StreamURL: "http://radio.cliamp.stream/lofi/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp Synthwave", StreamURL: "http://radio.cliamp.stream/synthwave/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp EDM", StreamURL: "http://radio.cliamp.stream/edm/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp NCS", StreamURL: "http://radio.cliamp.stream/ncs/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp NCS House", StreamURL: "http://radio.cliamp.stream/ncs-house/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name:        "Cliamp NCS Dubstep",
		StreamURL:   "http://radio.cliamp.stream/ncs-dubstep/stream",
		HomePageURL: cliampHomePageURL,
	},
	{
		Name:        "Cliamp NCS Drum & Bass",
		StreamURL:   "http://radio.cliamp.stream/ncs-dnb/stream",
		HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp NCS Trap", StreamURL: "http://radio.cliamp.stream/ncs-trap/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp NCS Phonk", StreamURL: "http://radio.cliamp.stream/ncs-phonk/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp NCS Pop", StreamURL: "http://radio.cliamp.stream/ncs-pop/stream", HomePageURL: cliampHomePageURL,
	},
	{
		Name: "Cliamp NCS Chill", StreamURL: "http://radio.cliamp.stream/ncs-chill/stream", HomePageURL: cliampHomePageURL,
	},
}

func newRadioCmd(opts *rootOptions) *cobra.Command {
	radio := &cobra.Command{
		Use:   "radio",
		Short: "Manage Navidrome internet radio stations",
	}
	radio.AddCommand(newImportCliampCmd(opts))
	return radio
}

func newImportCliampCmd(opts *rootOptions) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "import-cliamp",
		Short: "Import Cliamp's built-in radio stations",
		Long:  helpRadioImportCliamp,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, err := configuredClient(*opts)
			if err != nil {
				return err
			}
			existing, err := client.GetInternetRadioStations(cmd.Context())
			if err != nil {
				return err
			}
			byURL := make(map[string]struct{}, len(existing))
			for _, station := range existing {
				byURL[station.StreamURL] = struct{}{}
			}
			for _, station := range cliampStations {
				if _, found := byURL[station.StreamURL]; found {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "exists\t%s\n", station.Name); err != nil {
						return err
					}
					continue
				}
				if !yes {
					if _, err := fmt.Fprintf(cmd.OutOrStdout(), "would add\t%s\n", station.Name); err != nil {
						return err
					}
					continue
				}
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
	cmd.Flags().BoolVar(&yes, "yes", false, "Create missing stations")
	return cmd
}
