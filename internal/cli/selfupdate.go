package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ahmed/odoonoir/internal/selfupdate"
)

func newSelfUpdateCmd() *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "selfupdate",
		Short: "Check for updates and install the latest OdooNoir release",
		Long: `Checks GitHub for the latest OdooNoir release and installs it.

Without flags it shows whether an update is available.
With --install it downloads the .deb and runs dpkg to install it.`,
		Example: `  odoonoir selfupdate              # check for update
  odoonoir selfupdate --install    # download and install latest`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := selfupdate.VersionCheck(Version)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Current : %s\n", result.Current)
			fmt.Fprintf(cmd.OutOrStdout(), "Latest  : %s\n", result.Latest)

			if !result.HasUpdate {
				fmt.Fprintln(cmd.OutOrStdout(), th.Successf("Up to date ✓"))
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), th.Warningf("Update available: %s → %s", result.Current, result.Latest))

			if checkOnly {
				return nil
			}

			// Install
			fmt.Fprintln(cmd.OutOrStdout())
			return selfupdate.DownloadAndInstall(func(msg string) {
				fmt.Fprintln(cmd.OutOrStdout(), msg)
			})
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only check for updates, don't install")
	return cmd
}
